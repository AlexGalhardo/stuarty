package contract

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"
	"github.com/AlexGalhardo/stuarty/web/bootstrap"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/auth/hash"
	"github.com/AlexGalhardo/vukem/ratelimit"
	"github.com/AlexGalhardo/vukem/vukemtest"
)

// network is a server in network mode over the Service of a testenv: the HTTP
// API and /mcp behind the key guard, on a loopback address (APP_URL of the
// container-on-its-own-machine case, SDD-007 section 4.3), with its log
// captured.
type network struct {
	env  *testenv.Env
	auth *auth.Network
	srv  *httptest.Server
	logs *bytes.Buffer
	keys atomic.Int64
	user auth.User

	mu       sync.Mutex
	requests []string
}

// pass is the password of every user of the tests, built at run time (the fake
// hasher takes any).
var pass = strings.Repeat("pw-", 5)

func newNetwork(t *testing.T) *network {
	t.Helper()

	env := testenv.New(t, testenv.Options{})
	logs := &bytes.Buffer{}

	// The listener exists before the server starts, so APP_URL can name its port.
	srv := httptest.NewUnstartedServer(nil)
	res := config.Resolved{
		Mode: config.ModeNetwork, AppURL: "http://" + srv.Listener.Addr().String(), LoopbackHTTP: true,
	}

	netw, err := httpapi.NewNetwork(res, slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}

	now := func() time.Time { return fixedNow }

	netAuth, err := auth.NewNetwork(env.Store.DB(), auth.NetworkOptions{Now: now, Hasher: hash.Fake(), PlainHTTP: true})
	if err != nil {
		t.Fatal(err)
	}

	mcpHandler, err := mcp.Handler(env.Service, mcp.Options{})
	if err != nil {
		t.Fatal(err)
	}

	loose := ratelimit.NewMemory(1_000_000, time.Minute)
	deps := &httpapi.Deps{
		Service:     env.Service,
		Registry:    command.Default(),
		Workspace:   env.Dir,
		Network:     netw,
		Auth:        netAuth,
		MCP:         mcpHandler,
		LoginLimits: &httpapi.LoginLimits{PerEmailAndAddress: loose, PerEmail: loose, PerAddress: loose},
		Sessions:    auth.NewSessions(now),
		Launch:      auth.NewLaunch(now),
		Now:         now,
	}

	app := vukemtest.New(t, func(opts ...vukem.Option) (*vukem.App, error) { return bootstrap.NewWith(deps, opts...) })

	n := &network{env: env, auth: netAuth, srv: srv, logs: logs}

	// Every request line the server saw, to check that a key is never in a URL.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.mu.Lock()
		n.requests = append(n.requests, r.Method+" "+r.RequestURI)
		n.mu.Unlock()

		app.Handler().ServeHTTP(w, r)
	})
	srv.Start()
	t.Cleanup(srv.Close)

	return n
}

// key makes the user, once, and an API key with the scopes, limited to a project when
// project is not empty, and returns the secret and the id of the key.
func (n *network) key(t *testing.T, scopes []string, project string) (secret, id string) {
	t.Helper()

	// A user owns the projects, so every key of the tests is one person's.
	if n.user.ID == "" {
		var err error

		if n.user, err = n.auth.Users.Create(t.Context(), "ana@example.com", pass, service.RoleMember); err != nil {
			t.Fatal(err)
		}
	}

	u := n.user

	id = fmt.Sprintf("key_%010d", n.keys.Add(1))

	var err error

	secret, _, err = n.auth.Keys().CreateKey(t.Context(), service.NewKey{
		KeyID: id, UserID: u.ID, Name: "agent", Scopes: scopes, ProjectID: project, Now: fixedNow,
		MaxActive: 10, MaxRecent: 10, Window: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	return secret, id
}

// bearerTransport sends a key the way a client does.
type bearerTransport struct {
	secret string
	next   http.RoundTripper
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.secret)

	return b.next.RoundTrip(r)
}

// mcpSession connects an MCP client over HTTP at /mcp with the key.
func (n *network) mcpSession(t *testing.T, secret string) *sdk.ClientSession {
	t.Helper()

	own := *n.srv.Client()
	own.Transport = bearerTransport{secret: secret, next: n.srv.Client().Transport}
	hc := &own
	client := sdk.NewClient(&sdk.Implementation{Name: "claude-code", Version: "1"}, nil)

	cs, err := client.Connect(
		t.Context(),
		&sdk.StreamableClientTransport{Endpoint: n.srv.URL + "/mcp", HTTPClient: hc},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = cs.Close() })

	return cs
}

func callTool(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) *sdk.CallToolResult {
	t.Helper()

	res, err := cs.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}

	return res
}

func resultText(res *sdk.CallToolResult) string {
	var b strings.Builder

	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}

	return b.String()
}

var _ = cli.ExitOK

// cliRun runs one command of the CLI in client mode against the server, with
// the key in the environment, as a person or an agent would.
func (n *network) cliRun(t *testing.T, secret string, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	env := map[string]string{"STUARTY_URL": n.srv.URL, "STUARTY_KEY": secret}

	var out, errw bytes.Buffer

	rt := &cli.Runtime{
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errw,
		Getenv:   func(k string) string { return env[k] },
		Getwd:    func() (string, error) { return t.TempDir(), nil },
		Resolver: cli.LocalResolver{Getenv: func(k string) string { return env[k] }},
	}

	code = rt.Execute(t.Context(), args[0], args[1:])

	return code, out.String(), errw.String()
}

func structuredOf(t *testing.T, res *sdk.CallToolResult) map[string]any {
	t.Helper()

	m, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("no structured content: %s", resultText(res))
	}

	return m
}

func addTrack(t *testing.T, cs *sdk.ClientSession, project string, base int, id string) *sdk.CallToolResult {
	t.Helper()

	return callTool(t, cs, "edit", map[string]any{
		"project_id": project, "base_revision": base,
		"operations": []any{map[string]any{"op": "track.add", "args": map[string]any{"kind": "visual", "id": id}}},
	})
}

// TestMCPPrincipalPropagation (SDD-003 section 4.5 rule 4, section 9 check 24):
// /mcp accepts an API key, the key reaches the Service as the actor of the call,
// so two keys are two actors in the history of one project.
func TestMCPPrincipalPropagation(t *testing.T) {
	t.Parallel()

	n := newNetwork(t)
	anaSecret, anaID := n.key(t, []string{"read", "edit"}, "")
	bobSecret, bobID := n.key(t, []string{"read", "edit"}, "")
	ana, bob := n.mcpSession(t, anaSecret), n.mcpSession(t, bobSecret)

	project, _ := structuredOf(t, callTool(t, ana, "project_create", map[string]any{"name": "Shared"}))["project_id"].(string)

	first := addTrack(t, ana, project, 0, "trk_f6g7h8j9k0")
	second := addTrack(t, bob, project, 1, "trk_m1n2p3q4r5")

	for _, r := range []*sdk.CallToolResult{first, second} {
		if r.IsError {
			t.Fatalf("an edit by a key failed: %s", resultText(r))
		}
	}

	a, b := structuredOf(t, first)["actor"], structuredOf(t, second)["actor"]
	if a == b || !strings.Contains(a.(string), anaID) || !strings.Contains(b.(string), bobID) {
		t.Errorf("two keys are attributed to %v and %v, want one actor each naming %s and %s", a, b, anaID, bobID)
	}

	history := resultText(callTool(t, ana, "history", map[string]any{"project_id": project, "action": "list"}))
	if !strings.Contains(history, a.(string)) || !strings.Contains(history, b.(string)) {
		t.Errorf("the history does not name both actors:\n%s", history)
	}

	// A call with no key never reaches the Service.
	anon := n.srv.Client()
	client := sdk.NewClient(&sdk.Implementation{Name: "claude-code", Version: "1"}, nil)

	if _, err := client.Connect(
		t.Context(),
		&sdk.StreamableClientTransport{Endpoint: n.srv.URL + "/mcp", HTTPClient: anon},
		nil,
	); err == nil {
		t.Error("an MCP client with no key connected to /mcp")
	}
}

// TestAuthNetwork (SDD-007 section 15 check 19): a key without the edit scope
// calling edit, and a key limited to project A calling project B, both get
// E_FORBIDDEN through MCP as an execution error with the tool list unchanged,
// and through the CLI as exit 1 with the error document.
func TestAuthNetwork(t *testing.T) {
	t.Parallel()

	n := newNetwork(t)
	full, _ := n.key(t, []string{"read", "edit"}, "")
	owner := n.mcpSession(t, full)

	a, _ := structuredOf(t, callTool(t, owner, "project_create", map[string]any{"name": "A"}))["project_id"].(string)
	b, _ := structuredOf(t, callTool(t, owner, "project_create", map[string]any{"name": "B"}))["project_id"].(string)

	reader, _ := n.key(t, []string{"read"}, "")
	limited, _ := n.key(t, []string{"read", "edit"}, a)

	toolNames := func(cs *sdk.ClientSession) string {
		list, err := cs.ListTools(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}

		names := make([]string, 0, len(list.Tools))
		for _, tool := range list.Tools {
			names = append(names, tool.Name)
		}

		return strings.Join(names, ",")
	}

	forbidden := func(res *sdk.CallToolResult, what string) {
		t.Helper()

		if !res.IsError || !strings.Contains(resultText(res), "E_FORBIDDEN") {
			t.Errorf("%s through MCP: %q, want an execution error E_FORBIDDEN", what, resultText(res))
		}
	}

	readerSession := n.mcpSession(t, reader)
	limitedSession := n.mcpSession(t, limited)

	forbidden(addTrack(t, readerSession, a, 0, "trk_f6g7h8j9k0"), "edit without the edit scope")
	forbidden(callTool(t, limitedSession, "project_get", map[string]any{"project_id": b}), "a read of another project")
	forbidden(addTrack(t, limitedSession, b, 0, "trk_f6g7h8j9k0"), "an edit of another project")

	if got, want := toolNames(readerSession), toolNames(owner); got != want || got != toolNames(limitedSession) {
		t.Errorf("the tool list depends on the key:\n%s\n%s", got, want)
	}

	if res := addTrack(t, limitedSession, a, 0, "trk_f6g7h8j9k0"); res.IsError {
		t.Errorf("the key limited to A cannot edit A: %s", resultText(res))
	}

	// The same through the CLI in client mode.
	cliCode, out, _ := n.cliRun(t, reader, "project:show", a, "--json")
	if cliCode != cli.ExitOK {
		t.Errorf("a read with the read scope: exit %d %s", cliCode, out)
	}

	cliCode, out, errs := n.cliRun(t, limited, "project:show", b, "--json")
	if cliCode != cli.ExitFailure || !strings.Contains(out, "E_FORBIDDEN") || errs != "" {
		t.Errorf("another project through the CLI: exit %d, out %s, err %q", cliCode, out, errs)
	}
}

// TestNoTokenLeak: a key never appears in a URL the server saw, in its log, in
// what the CLI printed or in what an MCP tool returned, for a good call and for
// refused ones (SDD-007 section 4.2, SDD-003 section 9 check 19).
func TestNoTokenLeak(t *testing.T) {
	t.Parallel()

	n := newNetwork(t)
	full, _ := n.key(t, []string{"read", "edit"}, "")
	reader, _ := n.key(t, []string{"read"}, "")

	var seen strings.Builder

	record := func(parts ...string) { seen.WriteString(strings.Join(parts, "\n") + "\n") }

	owner := n.mcpSession(t, full)
	project, _ := structuredOf(t, callTool(t, owner, "project_create", map[string]any{"name": "Leaky"}))["project_id"].(string)

	record(resultText(addTrack(t, owner, project, 0, "trk_f6g7h8j9k0")))
	record(resultText(addTrack(t, n.mcpSession(t, reader), project, 1, "trk_m1n2p3q4r5")))
	record(resultText(callTool(t, owner, "project_get", map[string]any{"project_id": "prj_0000000000"})))

	for _, secret := range []string{full, reader, full[:len(full)-1] + "0"} {
		for _, args := range [][]string{
			{"project:show", project, "--json"},
			{"project:show", project},
			{"edit", "--project", project, "track.add", "kind=visual", "--json"},
			{"project:show", "prj_0000000000", "--json"},
		} {
			code, out, errs := n.cliRun(t, secret, args...)
			record(strconv.Itoa(code), out, errs)
		}
	}

	record(n.logs.String())

	for _, line := range n.requests {
		record(line)
	}

	for _, secret := range []string{full, reader} {
		// The prefix and the whole secret: a truncated copy in a log is a leak too.
		for _, needle := range []string{secret, secret[:len(auth.KeyPrefix)+12]} {
			if strings.Contains(seen.String(), needle) {
				t.Fatalf(
					"a key appears in the output of the server, the CLI or the tools: %q",
					needle[:len(auth.KeyPrefix)]+"...",
				)
			}
		}
	}

	if len(n.requests) == 0 {
		t.Fatal("the server saw no request")
	}
}
