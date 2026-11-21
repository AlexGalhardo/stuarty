package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/events"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"
	"github.com/AlexGalhardo/stuarty/web/app/store"
	"github.com/AlexGalhardo/stuarty/web/bootstrap"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/vukemtest"
)

// canary is the local token of every test: it must never appear in a log, a
// response or a URL (SDD-007 section 15 item 7).
const canary = "vvl_c4n4ry0000000000000000000000000000000000000000000000000000000000"

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.t = c.t.Add(d)
}

type harness struct {
	t      testing.TB
	app    *vukemtest.App
	vapp   *vukem.App
	srv    *httptest.Server
	svc    service.Service
	track  string
	events *events.Events
	dir    workspace.Dir
	store  *store.Store
	clock  *clock
	port   string
}

func newHarness(tb testing.TB) *harness {
	tb.Helper()

	return newHarnessWith(tb, events.Options{})
}

func newHarnessWith(tb testing.TB, eo events.Options, extra ...vukem.Option) *harness {
	tb.Helper()

	return newHarnessWrapped(tb, eo, nil, extra...)
}

// newHarnessWrapped serves the API over wrap(service), for tests that need a
// method the plain service cannot answer, such as a renderer.
func newHarnessWrapped(
	tb testing.TB,
	eo events.Options,
	wrap func(service.Service) service.Service,
	extra ...vukem.Option,
) *harness {
	tb.Helper()

	return newHarnessLimited(tb, eo, wrap, httpapi.RateLimits{}, extra...)
}

// newHarnessLimited is newHarnessWrapped with the rate limits of the API, for a
// benchmark that sends more requests a minute than the defaults allow.
func newHarnessLimited(
	tb testing.TB,
	eo events.Options,
	wrap func(service.Service) service.Service,
	rate httpapi.RateLimits,
	extra ...vukem.Option,
) *harness {
	tb.Helper()

	return newHarnessBuilt(tb, eo, wrap, rate, nil, extra...)
}

// newHarnessBuilt is newHarnessLimited with the chance to complete the options of the service (the media
// machinery of the image import) before it is built; the workspace folder is given.
func newHarnessBuilt(
	tb testing.TB,
	eo events.Options,
	wrap func(service.Service) service.Service,
	rate httpapi.RateLimits,
	mutate func(dir workspace.Dir, o *service.Options),
	extra ...vukem.Option,
) *harness {
	tb.Helper()

	dir, err := workspace.Resolve(func(string) string { return filepath.Join(tb.TempDir(), "ws") })
	if err != nil {
		tb.Fatal(err)
	}

	if err := dir.EnsureLayout(); err != nil {
		tb.Fatal(err)
	}

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = st.Close() })

	hub := events.New(st, eo)
	tb.Cleanup(hub.Close)

	opts := service.Options{Store: st, Publish: hub.Publish}
	if mutate != nil {
		mutate(dir, &opts)
	}

	svc, err := service.New(opts)
	if err != nil {
		tb.Fatal(err)
	}

	if wrap != nil {
		svc = wrap(svc)
	}

	h := &harness{
		t:      tb,
		svc:    svc,
		events: hub,
		dir:    dir,
		store:  st,
		clock:  &clock{t: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)},
	}

	mcpHandler, err := mcp.Handler(svc, mcp.Options{Version: "test"})
	if err != nil {
		tb.Fatal(err)
	}

	deps := &httpapi.Deps{
		MCP:       mcpHandler,
		Events:    hub,
		Service:   svc,
		Registry:  command.Default(),
		Workspace: dir,
		Token:     canary,
		Sessions: auth.NewSessions(
			h.clock.now,
		),
		Launch: auth.NewLaunch(h.clock.now),
		// The desktop shell's secret: a launch request that sends it gets a browser session.
		LauncherSecret: launcherSecret,
		Now:            h.clock.now,
		RateLimits:     rate,
	}

	h.app = vukemtest.New(tb, func(opts ...vukem.Option) (*vukem.App, error) {
		app, err := bootstrap.NewWith(deps, append(opts, extra...)...)
		h.vapp = app

		return app, err
	})

	h.srv = httptest.NewServer(h.app.Handler())
	tb.Cleanup(h.srv.Close)

	h.port = h.srv.Listener.Addr().String()[strings.LastIndex(h.srv.Listener.Addr().String(), ":")+1:]

	return h
}

// req is one request of a test; zero fields mean the defaults of a bearer client.
type req struct {
	method  string
	path    string
	body    string
	headers map[string]string
	noAuth  bool
	host    string
}

type resp struct {
	status int
	header http.Header
	body   []byte
}

func (r resp) json(tb testing.TB) map[string]any {
	tb.Helper()

	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		tb.Fatalf("body is not a JSON object: %v\n%s", err, r.body)
	}

	return m
}

func (h *harness) do(r req) resp {
	h.t.Helper()

	if r.method == "" {
		r.method = http.MethodGet
	}

	var body io.Reader
	if r.body != "" {
		body = strings.NewReader(r.body)
	}

	hr, err := http.NewRequest(r.method, h.srv.URL+r.path, body)
	if err != nil {
		h.t.Fatal(err)
	}

	if !r.noAuth {
		hr.Header.Set("Authorization", "Bearer "+canary)
	}

	if r.body != "" {
		hr.Header.Set("Content-Type", "application/json")
	}

	for k, v := range r.headers {
		hr.Header.Set(k, v)
	}

	if r.host != "" {
		hr.Host = r.host
	}

	client := *h.srv.Client() // a copy: the redirect policy is per request, and tests run in parallel
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	res, err := client.Do(hr)
	if err != nil {
		h.t.Fatal(err)
	}

	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		h.t.Fatal(err)
	}

	return resp{status: res.StatusCode, header: res.Header, body: b}
}

func (h *harness) get(path string) resp { return h.do(req{path: path}) }

func (h *harness) post(path, body string) resp {
	return h.do(req{method: http.MethodPost, path: path, body: body})
}

// newProject creates a project over HTTP and returns its id.
func (h *harness) newProject(name string) string {
	h.t.Helper()

	res := h.post("/api/v1/projects", projectJSON(name))
	if res.status != http.StatusCreated {
		h.t.Fatalf("create = %d %s", res.status, res.body)
	}

	id, _ := res.json(h.t)["project_id"].(string)

	// The editor tests work on the timeline that a video makes; the first one has the id of the project.
	if added := h.post(
		"/api/v1/projects/"+id+"/items",
		`{"kind":"timeline","title":`+quoted(name)+`}`,
	); added.status != http.StatusCreated {
		h.t.Fatalf("add timeline = %d %s", added.status, added.body)
	}

	return id
}

// projectJSON is the body of a valid POST /projects for name: the description and the goal are required (epic 56).
func projectJSON(name string) string {
	raw, err := json.Marshal(map[string]any{
		"name": testproject.ValidName(name), "description": testproject.Description, "goal": testproject.Goal,
	})
	if err != nil {
		panic(err)
	}

	return string(raw)
}

func quoted(s string) string {
	raw, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}

	return string(raw)
}

// addFirstTimeline adds the first timeline of a project, which has the id of the project. A project has no timeline
// until one is added (epic 56), and a key can be limited only to a project that has one.
func addFirstTimeline(t *testing.T, h *networkHarness, session, projectID string) {
	t.Helper()

	if res := h.send(
		t,
		http.MethodPost,
		"/api/v1/projects/"+projectID+"/items",
		`{"kind":"timeline","title":"First"}`,
		session,
		nil,
	); res.status != http.StatusCreated {
		t.Fatalf("add the first timeline = %d %s", res.status, res.body)
	}
}
