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

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/vukemtest"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
	"github.com/AlexGalhardo/vukem-video/web/app/events"
	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
	"github.com/AlexGalhardo/vukem-video/web/app/mcp"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
	"github.com/AlexGalhardo/vukem-video/web/bootstrap"
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
	t      *testing.T
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

func newHarness(t *testing.T) *harness {
	t.Helper()

	return newHarnessWith(t, events.Options{})
}

func newHarnessWith(t *testing.T, eo events.Options, extra ...vukem.Option) *harness {
	t.Helper()

	return newHarnessWrapped(t, eo, nil, extra...)
}

// newHarnessWrapped serves the API over wrap(service), for tests that need a
// method the plain service cannot answer, such as a renderer.
func newHarnessWrapped(
	t *testing.T,
	eo events.Options,
	wrap func(service.Service) service.Service,
	extra ...vukem.Option,
) *harness {
	t.Helper()

	dir, err := workspace.Resolve(func(string) string { return filepath.Join(t.TempDir(), "ws") })
	if err != nil {
		t.Fatal(err)
	}

	if err := dir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	hub := events.New(st, eo)
	t.Cleanup(hub.Close)

	svc, err := service.New(service.Options{Store: st, Publish: hub.Publish})
	if err != nil {
		t.Fatal(err)
	}

	if wrap != nil {
		svc = wrap(svc)
	}

	h := &harness{
		t:      t,
		svc:    svc,
		events: hub,
		dir:    dir,
		store:  st,
		clock:  &clock{t: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)},
	}

	mcpHandler, err := mcp.Handler(svc, mcp.Options{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}

	deps := &httpapi.Deps{
		MCP: mcpHandler, Events: hub,
		Service: svc, Registry: command.Default(), Jobs: st.Jobs(), Workspace: dir, Token: canary,
		Sessions: auth.NewSessions(h.clock.now), Launch: auth.NewLaunch(h.clock.now), Now: h.clock.now,
	}

	h.app = vukemtest.New(t, func(opts ...vukem.Option) (*vukem.App, error) {
		app, err := bootstrap.NewWith(deps, append(opts, extra...)...)
		h.vapp = app

		return app, err
	})

	h.srv = httptest.NewServer(h.app.Handler())
	t.Cleanup(h.srv.Close)

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

func (r resp) json(t *testing.T) map[string]any {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("body is not a JSON object: %v\n%s", err, r.body)
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

	res := h.post("/api/v1/projects", `{"name":"`+name+`"}`)
	if res.status != http.StatusCreated {
		h.t.Fatalf("create = %d %s", res.status, res.body)
	}

	id, _ := res.json(h.t)["project_id"].(string)

	return id
}
