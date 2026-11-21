package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
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

var (
	fakeOnce sync.Once
	fakeDir  string
	fakePath string
	errFake  error
)

func TestMain(m *testing.M) {
	code := m.Run()

	if fakeDir != "" {
		_ = os.RemoveAll(fakeDir)
	}

	os.Exit(code)
}

func fakeAgentBinary(t *testing.T) string {
	t.Helper()

	fakeOnce.Do(func() {
		fakeDir, errFake = os.MkdirTemp("", "stuarty-fakeagent-") //nolint:usetesting // built once for the package
		if errFake != nil {
			return
		}

		name := "fakeagent"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}

		fakePath = filepath.Join(fakeDir, name)

		out, err := exec.CommandContext(context.Background(), "go", "build", "-o", fakePath,
			"github.com/AlexGalhardo/stuarty/service/agenthost/fakeagent").CombinedOutput()
		if err != nil {
			errFake = &buildError{err: err, out: string(out)}
		}
	})

	if errFake != nil {
		t.Fatalf("building the fake agent: %v", errFake)
	}

	return fakePath
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

// chatHarness is the whole product around a hosted agent: the real store, the service with a host,
// the registry of session tokens, the MCP endpoint and the HTTP adapter, behind a test server.
type chatHarness struct {
	*harness
	host   *agenthost.Host
	tokens *agenthost.Tokens
}

func newChatHarness(t *testing.T) *chatHarness {
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

	hub := events.New(st, events.Options{})
	t.Cleanup(hub.Close)

	c := &chatHarness{tokens: agenthost.NewTokens(nil)}
	h := &harness{
		t: t, events: hub, dir: dir, store: st,
		clock: &clock{t: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)},
	}
	c.harness = h

	c.host = agenthost.New(agenthost.Options{
		Store: st.Chat(), Tokens: c.tokens, Workspace: string(dir), Version: "test",
		MCPURL: func() string { return h.srv.URL + "/mcp" },
		Limits: agenthost.Limits{InputClose: 200 * time.Millisecond},
	})
	// The host stops before the store closes.
	t.Cleanup(c.host.Shutdown)

	// MinFreeBytes -1: not a test of the disk guard, so no dependence on the free space of the machine.
	svc, err := service.New(service.Options{
		Store:     st,
		Publish:   hub.Publish,
		Workspace: string(dir),
		Agents:    c.host,
		Limits:    service.Limits{MinFreeBytes: -1},
	})
	if err != nil {
		t.Fatal(err)
	}

	h.svc = svc

	mcpHandler, err := mcp.Handler(svc, mcp.Options{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}

	deps := &httpapi.Deps{
		MCP: mcpHandler, Events: hub, Service: svc, Registry: command.Default(), Workspace: dir, Token: canary,
		Sessions: auth.NewSessions(h.clock.now), Launch: auth.NewLaunch(h.clock.now), LauncherSecret: launcherSecret,
		Now: h.clock.now, AgentTokens: c.tokens, Chat: c.host,
	}

	h.app = vukemtest.New(t, func(opts ...vukem.Option) (*vukem.App, error) {
		app, err := bootstrap.NewWith(deps, opts...)
		h.vapp = app

		return app, err
	})

	h.srv = httptest.NewServer(h.app.Handler())
	t.Cleanup(h.srv.Close)

	addr := h.srv.Listener.Addr().String()
	h.port = addr[strings.LastIndex(addr, ":")+1:]

	return c
}

// asPerson is the headers of a request of the person in the app: the session cookie of a privileged
// launch.
func (c *chatHarness) asPerson(t *testing.T) map[string]string {
	t.Helper()

	return map[string]string{"Cookie": c.session(t), "Sec-Fetch-Site": "same-origin", "Origin": c.srv.URL}
}

func (c *chatHarness) person(t *testing.T, headers map[string]string, method, path, body string) resp {
	t.Helper()

	return c.do(req{method: method, path: path, body: body, noAuth: true, headers: headers})
}

func (c *chatHarness) createProject(t *testing.T, name string) string {
	t.Helper()

	res := c.post("/api/v1/projects", projectJSON(name))
	if res.status != http.StatusCreated {
		t.Fatalf("create project = %d %s", res.status, res.body)
	}

	id, _ := res.json(t)["project_id"].(string)

	if added := c.post(
		"/api/v1/projects/"+id+"/items",
		`{"kind":"timeline","title":`+quoted(name)+`}`,
	); added.status != http.StatusCreated {
		t.Fatalf("add timeline = %d %s", added.status, added.body)
	}

	return id
}

// readStream reads the frames of an event stream until a frame satisfies done, and returns the
// events it saw.
func readStream(t *testing.T, url string, headers map[string]string, done func(chat.Event) bool) []chat.Event {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	for k, v := range headers {
		r.Header.Set(k, v)
	}

	client := &http.Client{} //nolint:forbidigo // loopback only: the test server
	res, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("the stream answered %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}

	var (
		seen []chat.Event
		name string
	)

	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)

	for sc.Scan() {
		line := sc.Text()

		switch {
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: ") && name == events.ChatEventName:
			var ev chat.Event
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
				t.Fatalf("a frame that is not an event: %v", err)
			}

			seen = append(seen, ev)

			if done(ev) {
				return seen
			}
		case line == "":
			name = ""
		}
	}

	t.Fatalf("the stream ended before the event came; %d events seen", len(seen))

	return nil
}

// Epic 14 acceptance criteria 1 to 4, end to end: the person starts a session over HTTP, the agent
// edits through the MCP endpoint with its own token, the edit is in the operation log with the
// agent as its actor, and neither the agent nor the local token can write a profile or drive a chat.
func TestChatHostedAgentEditsThroughMCP(t *testing.T) {
	t.Parallel()

	c := newChatHarness(t)
	person := c.asPerson(t)
	mine, other := c.createProject(t, "mine"), c.createProject(t, "other")

	// A profile is written by a person; the local token and a bearer client are refused.
	body := `{"command":` + jsonString(fakeAgentBinary(t)) + `,"args":["--mode=mcp"]}`
	for name, r := range map[string]resp{
		"the local token": c.do(req{method: http.MethodPut, path: "/api/v1/agents/profiles/fake", body: body}),
		"no credential":   c.do(req{method: http.MethodPut, path: "/api/v1/agents/profiles/fake", body: body, noAuth: true}),
	} {
		if r.status != http.StatusForbidden && r.status != http.StatusUnauthorized {
			t.Errorf("%s writing a profile = %d %s", name, r.status, r.body)
		}
	}

	if res := c.person(t, person, http.MethodPut, "/api/v1/agents/profiles/fake", body); res.status != http.StatusOK {
		t.Fatalf("the person writing a profile = %d %s", res.status, res.body)
	}

	// The local token reads the profiles but cannot start a session.
	if res := c.get("/api/v1/agents/profiles"); res.status != http.StatusOK ||
		!strings.Contains(string(res.body), `"name":"fake"`) {
		t.Errorf("the profiles = %d %s", res.status, res.body)
	}

	if res := c.post(
		"/api/v1/projects/"+mine+"/chat/sessions",
		`{"profile":"fake"}`,
	); res.status != http.StatusForbidden ||
		res.json(t)["code"] != "E_PRIVILEGED_SESSION" {
		t.Errorf("the local token starting a session = %d %s", res.status, res.body)
	}

	started := c.person(t, person, http.MethodPost, "/api/v1/projects/"+mine+"/chat/sessions", `{"profile":"fake"}`)
	if started.status != http.StatusCreated {
		t.Fatalf("starting a session = %d %s", started.status, started.body)
	}

	session, _ := started.json(t)["id"].(string)

	// The stream opens with what was said and follows the live events.
	editCall := `mcp edit {"project_id":"` + mine + `","base_revision":0,"operations":[{"op":"track.add","args":{"kind":"visual"}}]}`
	otherCall := `mcp edit {"project_id":"` + other + `","base_revision":0,"operations":[{"op":"track.add","args":{"kind":"visual"}}]}`

	streamed := make(chan []chat.Event, 1)

	go func() {
		streamed <- readStream(t, c.srv.URL+"/api/v1/chat/sessions/"+session+"/events", person, func(e chat.Event) bool {
			return e.Kind == chat.KindState && e.State.State == chat.StateIdle && e.Seq > 3
		})
	}()

	for _, text := range []string{editCall, otherCall} {
		if res := c.person(t, person, http.MethodPost, "/api/v1/chat/sessions/"+session+"/messages",
			`{"text":`+jsonString(text)+`}`); res.status != http.StatusOK {
			t.Fatalf("sending = %d %s", res.status, res.body)
		}

		// Wait for the turn to end before the next message.
		c.waitIdle(t, session)
	}

	var seen []chat.Event
	select {
	case seen = <-streamed:
	case <-time.After(30 * time.Second):
		t.Fatal("the stream saw no end of turn")
	}

	if len(seen) == 0 || seen[0].Seq != 1 {
		t.Errorf("the stream did not start at the first event: %+v", seen)
	}

	// The edit of the agent is in the log with the agent as the actor.
	hist := c.get("/api/v1/projects/" + mine + "/history").json(t)
	var names []string

	for _, e := range hist["entries"].([]any) {
		names = append(names, e.(map[string]any)["actor"].(string))
	}

	actors := strings.Join(names, ",")

	if !strings.Contains(actors, "local:chat-"+strings.TrimPrefix(session, "cht_")) {
		t.Errorf("the log has the actors %q, want the agent of session %s", actors, session)
	}

	// The other project was not touched: the agent was refused.
	if rev := c.get("/api/v1/projects/" + other).json(t)["revision"]; rev != float64(0) {
		t.Errorf("the other project is at revision %v", rev)
	}

	transcript := c.get("/api/v1/chat/sessions/" + session + "/transcript?format=json").json(t)
	text, err := json.Marshal(transcript)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(text), "mcp 200") || !strings.Contains(string(text), "E_FORBIDDEN") {
		t.Errorf("the transcript does not show the agent's two calls: %s", text)
	}

	// The token of the agent, used by hand, reaches its project and cannot drive a chat or write a profile.
	secret, revoke, err := c.tokens.Issue(agenthost.Grant{
		SessionID: "cht_0000000777", ProjectID: mine, Scopes: agenthost.GrantScopes(),
	}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	defer revoke()

	bearer := map[string]string{"Authorization": "Bearer " + secret}

	for name, r := range map[string]req{
		"a profile":    {method: http.MethodPut, path: "/api/v1/agents/profiles/evil", body: body},
		"a session":    {method: http.MethodPost, path: "/api/v1/projects/" + mine + "/chat/sessions", body: `{"profile":"fake"}`},
		"a message":    {method: http.MethodPost, path: "/api/v1/chat/sessions/" + session + "/messages", body: `{"text":"x"}`},
		"an answer":    {method: http.MethodPost, path: "/api/v1/chat/sessions/" + session + "/permissions/cpr_0000000001", body: `{"answer":"allow"}`},
		"a delete":     {method: http.MethodDelete, path: "/api/v1/chat/sessions/" + session},
		"a decision":   {method: http.MethodPost, path: "/api/v1/approvals/apr_0000000001/decide", body: `{"decision":"approve"}`},
		"another read": {method: http.MethodGet, path: "/api/v1/projects/" + other},
	} {
		r.noAuth, r.headers = true, bearer

		if res := c.do(r); res.status != http.StatusForbidden {
			t.Errorf("the agent token on %s = %d %s, want 403", name, res.status, res.body)
		}
	}

	if res := c.do(req{path: "/api/v1/projects/" + mine, noAuth: true, headers: bearer}); res.status != http.StatusOK {
		t.Errorf("the agent token on its own project = %d %s", res.status, res.body)
	}

	// After the session ends the token is dead: nothing authenticates with it.
	if res := c.person(
		t,
		person,
		http.MethodPost,
		"/api/v1/chat/sessions/"+session+"/cancel",
		`{"end":true}`,
	); res.status != http.StatusOK {
		t.Fatalf("ending = %d %s", res.status, res.body)
	}

	if c.host.LiveCount() != 0 {
		t.Errorf("%d agents are still running", c.host.LiveCount())
	}
}

func (c *chatHarness) waitIdle(t *testing.T, session string) {
	t.Helper()

	sub, err := c.host.Subscribe(context.Background(), session, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	// The last state event says whether the turn ended.
	last := func(evs []chat.Event) chat.State {
		var st chat.State

		for _, e := range evs {
			if e.Kind == chat.KindState {
				st = e.State.State
			}
		}

		return st
	}

	seen := sub.Snapshot
	if last(seen) == chat.StateIdle {
		return
	}

	deadline := time.After(30 * time.Second)

	for {
		select {
		case ev, ok := <-sub.Events:
			if !ok {
				t.Fatal("the stream closed before the turn ended")
			}

			if ev.Kind == chat.KindState && ev.State.State == chat.StateIdle {
				return
			}
		case <-deadline:
			t.Fatal("the turn did not end")
		}
	}
}

func jsonString(s string) string {
	return strconv.Quote(s)
}
