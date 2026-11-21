package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/account"
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

const (
	accPassword = "Correct-Horse-9"
	accOther    = "Another-Pass-7!"
)

var accountParams = account.Params{Time: 1, MemoryKiB: 64, Threads: 1}

// accountHarness is a local server that has an account manager, as the owner builds it.
type accountHarness struct {
	*harness
	acc *account.Manager
}

func newAccountHarness(tb testing.TB, extra ...vukem.Option) *accountHarness {
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

	clk := &clock{t: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}

	acc := account.NewManager(account.Options{Store: st.Accounts(), Now: clk.now, Params: accountParams})
	if err := acc.Load(context.Background()); err != nil {
		tb.Fatal(err)
	}

	hub := events.New(st, events.Options{})
	tb.Cleanup(hub.Close)

	svc, err := service.New(service.Options{Store: st, Publish: hub.Publish, Gate: acc})
	if err != nil {
		tb.Fatal(err)
	}

	h := &harness{t: tb, svc: svc, events: hub, dir: dir, store: st, clock: clk}

	mcpHandler, err := mcp.Handler(svc, mcp.Options{Version: "test"})
	if err != nil {
		tb.Fatal(err)
	}

	deps := &httpapi.Deps{
		MCP: mcpHandler, Events: hub, Service: svc, Registry: command.Default(), Workspace: dir, Token: canary,
		Sessions: auth.NewSessions(clk.now), Launch: auth.NewLaunch(clk.now), LauncherSecret: launcherSecret,
		Now: clk.now, Account: acc,
	}

	h.app = vukemtest.New(tb, func(opts ...vukem.Option) (*vukem.App, error) {
		app, err := bootstrap.NewWith(deps, append(opts, extra...)...)
		h.vapp = app

		return app, err
	})

	h.srv = httptest.NewServer(h.app.Handler())
	tb.Cleanup(h.srv.Close)

	addr := h.srv.Listener.Addr().String()
	h.port = addr[strings.LastIndex(addr, ":")+1:]

	return &accountHarness{harness: h, acc: acc}
}

const createBody = `{"username":"stuart","email":"stuart@example.com","password":"` + accPassword +
	`","confirmation":"` + accPassword + `"}`

// create makes the account with the local token and returns the recovery key.
func (a *accountHarness) create() string {
	a.t.Helper()

	res := a.post("/auth/account/create", createBody)
	if res.status != http.StatusOK {
		a.t.Fatalf("create = %d %s", res.status, res.body)
	}

	key, _ := res.json(a.t)["recovery_key"].(string)

	return key
}

// browser is a request from a browser that holds the given cookie.
func (a *accountHarness) browser(method, path, body, cookie string) resp {
	a.t.Helper()

	headers := map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": a.srv.URL}
	if cookie != "" {
		headers["Cookie"] = cookie
	}

	return a.do(req{method: method, path: path, body: body, noAuth: true, headers: headers})
}

// signIn is the password sign-in of a browser; it returns the cookie.
func (a *accountHarness) signIn(password string) (string, resp) {
	a.t.Helper()

	res := a.browser(http.MethodPost, "/auth/account/login", `{"identity":"stuart","password":"`+password+`"}`, "")
	cookie, _, _ := strings.Cut(res.header.Get("Set-Cookie"), ";")

	return cookie, res
}

func TestAccountFirstRunCreateAndSignIn(t *testing.T) {
	t.Parallel()

	a := newAccountHarness(t)

	// Nothing to sign in to yet, and the state needs no credential.
	state := a.do(req{path: "/auth/account", noAuth: true})
	if state.status != http.StatusOK || state.json(t)["state"] != "none" {
		t.Fatalf("state = %d %s", state.status, state.body)
	}

	// A workspace with no account works as before.
	if res := a.get("/api/v1/projects"); res.status != http.StatusOK {
		t.Fatalf("a workspace with no account refused a read: %d %s", res.status, res.body)
	}

	// Creating the account needs a caller: a stranger on the machine with no token and no session cannot.
	if res := a.browser(
		http.MethodPost,
		"/auth/account/create",
		createBody,
		"",
	); res.status != http.StatusUnauthorized {
		t.Fatalf("an anonymous create = %d %s", res.status, res.body)
	}

	// A refused field is E_VALIDATION with the field and the reason, never the value.
	bad := a.post("/auth/account/create",
		`{"username":"AB","email":"x","password":"abc","confirmation":"abd"}`)
	if bad.status != http.StatusBadRequest || bad.json(t)["code"] != "E_VALIDATION" ||
		!strings.Contains(
			string(bad.body),
			`"reason":"too_short"`,
		) || !strings.Contains(string(bad.body), `"mismatch"`) {
		t.Fatalf("bad create = %d %s", bad.status, bad.body)
	}

	if strings.Contains(string(bad.body), "abc") && strings.Contains(string(bad.body), `"abd"`) {
		t.Fatalf("the refusal echoes a value: %s", bad.body)
	}

	key := a.create()
	if len(strings.Split(key, "-")) != 13 {
		t.Fatalf("recovery key %q", key)
	}

	if again := a.post("/auth/account/create", createBody); again.status != http.StatusConflict ||
		again.json(t)["code"] != "E_INVALID_STATE" {
		t.Fatalf("a second create = %d %s", again.status, again.body)
	}

	if state := a.do(req{path: "/auth/account", noAuth: true}); state.json(t)["state"] != "locked" {
		t.Fatalf("after create: %s", state.body)
	}

	// Locked: the token and a launch session reach nothing; the health check, the handshake and the state stay.
	for _, r := range []req{
		{path: "/api/v1/projects"},
		{method: http.MethodPost, path: "/api/v1/projects", body: `{"name":"x"}`},
		{path: "/api/v1/projects/prj_0000000001/events"},
		{method: http.MethodPost, path: "/mcp", body: `{}`},
	} {
		res := a.do(r)
		if res.status != http.StatusLocked || res.json(t)["code"] != "E_LOCKED" {
			t.Errorf("%s %s while locked = %d %s", r.method, r.path, res.status, res.body)
		}
	}

	if res := a.do(req{path: "/up", noAuth: true}); res.status != http.StatusOK {
		t.Errorf("the health check while locked = %d", res.status)
	}

	// A wrong password and an unknown name are the same answer.
	for _, body := range []string{
		`{"identity":"stuart","password":"` + accOther + `"}`, `{"identity":"nobody","password":"` + accPassword + `"}`,
	} {
		res := a.browser(http.MethodPost, "/auth/account/login", body, "")
		if res.status != http.StatusUnauthorized || res.json(t)["code"] != "E_UNAUTHORIZED" ||
			res.header.Get("Set-Cookie") != "" {
			t.Errorf("a refused sign-in = %d %s", res.status, res.body)
		}
	}

	cookie, res := a.signIn(accPassword)
	if res.status != http.StatusOK || cookie == "" {
		t.Fatalf("sign-in = %d %s", res.status, res.body)
	}

	for _, secret := range []string{accPassword, key} {
		if strings.Contains(string(res.body), secret) || strings.Contains(res.header.Get("Set-Cookie"), secret) {
			t.Fatalf("a sign-in answer holds a secret: %s", res.body)
		}
	}

	if !strings.Contains(res.header.Get("Set-Cookie"), "HttpOnly") ||
		!strings.Contains(res.header.Get("Set-Cookie"), "SameSite=Strict") {
		t.Errorf("cookie flags: %s", res.header.Get("Set-Cookie"))
	}

	if res := a.browser(http.MethodGet, "/api/v1/projects", "", cookie); res.status != http.StatusOK {
		t.Fatalf("a read with the session = %d %s", res.status, res.body)
	}

	// Unlocked, the token works too (an agent works while a person has the studio open).
	if res := a.get("/api/v1/projects"); res.status != http.StatusOK {
		t.Fatalf("the token while unlocked = %d %s", res.status, res.body)
	}
}

func TestAccountLogoutLocksAndEndsEverySession(t *testing.T) {
	t.Parallel()

	a := newAccountHarness(t)
	a.create()

	cookie, _ := a.signIn(accPassword)
	other, _ := a.signIn(accPassword)

	// The token is not a person: it cannot lock the studio.
	if res := a.post("/auth/account/logout", "{}"); res.status != http.StatusForbidden ||
		res.json(t)["code"] != "E_PRIVILEGED_SESSION" {
		t.Fatalf("logout with the token = %d %s", res.status, res.body)
	}

	if res := a.browser(http.MethodPost, "/auth/account/logout", "{}", cookie); res.status != http.StatusOK {
		t.Fatalf("logout = %d %s", res.status, res.body)
	}

	if a.acc.State() != account.StateLocked {
		t.Fatal("the studio is not locked after Logout")
	}

	for _, c := range []string{cookie, other} {
		if res := a.browser(http.MethodGet, "/api/v1/projects", "", c); res.status != http.StatusUnauthorized &&
			res.status != http.StatusLocked {
			t.Errorf("a session survived the logout: %d %s", res.status, res.body)
		}
	}

	if res := a.get("/api/v1/projects"); res.status != http.StatusLocked {
		t.Errorf("the token after the logout = %d", res.status)
	}
}

func TestAccountRecoveryPasswordAndSettings(t *testing.T) {
	t.Parallel()

	a := newAccountHarness(t)
	key := a.create()

	// A wrong recovery key.
	res := a.browser(http.MethodPost, "/auth/account/recover",
		`{"recovery_key":"AAAA-BBBB","password":"`+accOther+`","confirmation":"`+accOther+`"}`, "")
	if res.status != http.StatusUnauthorized {
		t.Fatalf("a wrong recovery key = %d %s", res.status, res.body)
	}

	// A weak new password.
	res = a.browser(http.MethodPost, "/auth/account/recover",
		`{"recovery_key":"`+key+`","password":"weak","confirmation":"weak"}`, "")
	if res.status != http.StatusBadRequest || res.json(t)["code"] != "E_VALIDATION" {
		t.Fatalf("a weak password = %d %s", res.status, res.body)
	}

	res = a.browser(http.MethodPost, "/auth/account/recover",
		`{"recovery_key":"`+key+`","password":"`+accOther+`","confirmation":"`+accOther+`"}`, "")
	cookie, _, _ := strings.Cut(res.header.Get("Set-Cookie"), ";")

	if res.status != http.StatusOK || cookie == "" || a.acc.State() != account.StateUnlocked {
		t.Fatalf("recover = %d %s", res.status, res.body)
	}

	// Settings and the password need a person; the token is refused.
	if res := a.do(
		req{method: http.MethodPut, path: "/auth/account/settings", body: `{"idle_lock_minutes":5}`},
	); res.status !=
		http.StatusForbidden {
		t.Fatalf("settings with the token = %d %s", res.status, res.body)
	}

	if res := a.browser(http.MethodPut, "/auth/account/settings", `{"idle_lock_minutes":2000}`, cookie); res.status !=
		http.StatusBadRequest {
		t.Fatalf("an idle lock out of range = %d %s", res.status, res.body)
	}

	if res := a.browser(http.MethodPut, "/auth/account/settings", `{"idle_lock_minutes":5}`, cookie); res.status !=
		http.StatusOK || a.acc.IdleLockMinutes() != 5 {
		t.Fatalf("settings = %d %s", res.status, res.body)
	}

	if got := a.browser(http.MethodGet, "/auth/account/settings", "", cookie).json(t)["idle_lock_minutes"]; got != float64(
		5,
	) {
		t.Fatalf("the idle lock reads back as %v", got)
	}

	// The idle lock locks, and the sessions go with the keys.
	a.clock.advance(6 * time.Minute)
	a.acc.Tick()

	if a.acc.State() != account.StateLocked {
		t.Fatal("the idle lock did not lock")
	}

	if res := a.browser(http.MethodGet, "/api/v1/projects", "", cookie); res.status == http.StatusOK {
		t.Fatal("a session read after the idle lock")
	}

	cookie, _ = a.signIn(accOther)

	// The password changes with the current one; the old session ends and a new one is given.
	change := a.browser(http.MethodPost, "/auth/account/password",
		`{"current":"`+accPassword+`","password":"`+accPassword+`x1!","confirmation":"`+accPassword+`x1!"}`, cookie)
	if change.status != http.StatusUnauthorized {
		t.Fatalf("a wrong current password = %d %s", change.status, change.body)
	}

	change = a.browser(http.MethodPost, "/auth/account/password",
		`{"current":"`+accOther+`","password":"`+accPassword+`","confirmation":"`+accPassword+`"}`, cookie)
	renewed, _, _ := strings.Cut(change.header.Get("Set-Cookie"), ";")

	if change.status != http.StatusOK || renewed == "" || renewed == cookie {
		t.Fatalf("change = %d %s", change.status, change.body)
	}

	if res := a.browser(http.MethodGet, "/api/v1/projects", "", cookie); res.status == http.StatusOK {
		t.Error("the old session survived a password change")
	}

	if res := a.browser(http.MethodGet, "/api/v1/projects", "", renewed); res.status != http.StatusOK {
		t.Errorf("the renewed session = %d %s", res.status, res.body)
	}
}

func TestAccountRoutesAreNotInNetworkMode(t *testing.T) {
	t.Parallel()

	h := newHarness(t) // no manager: the routes answer as a route that is not there

	for _, r := range []req{
		{path: "/auth/account", noAuth: true},
		{method: http.MethodPost, path: "/auth/account/create", body: createBody},
	} {
		res := h.do(r)
		if res.status == http.StatusOK {
			t.Errorf("%s %s answered 200 with no manager", r.method, r.path)
		}
	}
}

// TestSecurity_T117_OnlyThePasswordMakesAPerson: a session from a launch code (`stuarty open`, a launch that
// proves the launcher secret, the desktop shell), the local token and an agent are never a person, and the
// operations of a person answer E_PRIVILEGED_SESSION to them; the session of the password is one
// (SDD-007 section 3.9 item 5).
func TestSecurity_T117_OnlyThePasswordMakesAPerson(t *testing.T) {
	t.Parallel()

	a := newAccountHarness(t)
	a.create()

	person, _ := a.signIn(accPassword)

	persons := []req{
		{method: http.MethodPost, path: "/api/v1/approval-rules", body: `{"kind":"publish.scheduled"}`},
		{method: http.MethodPut, path: "/api/v1/cost/ceilings", body: `{"currency":"USD","per_day_micros":1}`},
		{method: http.MethodPost, path: "/auth/account/logout", body: `{}`},
	}

	for name, secret := range map[string]string{"a plain launch": "", "a launch with the launcher secret": launcherSecret} {
		launched := a.sessionWith(t, secret)

		// The launch session works as the editor.
		if res := a.browser(http.MethodGet, "/api/v1/projects", "", launched); res.status != http.StatusOK {
			t.Fatalf("%s: the editor read = %d %s", name, res.status, res.body)
		}

		for _, r := range persons {
			res := a.browser(r.method, r.path, r.body, launched)
			if res.status != http.StatusForbidden || res.json(t)["code"] != "E_PRIVILEGED_SESSION" {
				t.Errorf("%s: %s %s = %d %s, want E_PRIVILEGED_SESSION", name, r.method, r.path, res.status, res.body)
			}
		}
	}

	for _, r := range persons[:2] {
		if res := a.do(req{method: r.method, path: r.path, body: r.body}); res.status != http.StatusForbidden ||
			res.json(t)["code"] != "E_PRIVILEGED_SESSION" {
			t.Errorf("the local token: %s %s = %d %s", r.method, r.path, res.status, res.body)
		}

		if res := a.browser(r.method, r.path, r.body, person); res.status != http.StatusOK {
			t.Errorf("the password session: %s %s = %d %s, want 200", r.method, r.path, res.status, res.body)
		}
	}

	// The studio is still open for the person after the refusals above.
	if a.acc.State() != account.StateUnlocked {
		t.Fatal("a refused logout locked the studio")
	}
}

// The pages follow the account (SDD-012 section 2.1.1): a new workspace sends the browser to the page that
// creates the account, a locked studio to the sign-in, and an unlocked one draws the dashboard.
func TestAccountPagesFollowTheStateOfTheAccount(t *testing.T) {
	t.Parallel()

	built := fstest.MapFS{"build/.vite/manifest.json": {Data: []byte(manifest)}}
	a := newAccountHarness(t, vukem.WithVite(built))

	page := func(path, cookie string) resp {
		headers := map[string]string{"Accept": "text/html"}
		if cookie != "" {
			headers["Cookie"] = cookie
		}

		return a.do(req{path: path, noAuth: true, headers: headers})
	}

	location := func(r resp) string { return r.header.Get("Location") }

	launched := a.sessionWith(t, "")

	// No account yet: the dashboard is the create page, the create page is drawn, the sign-in has nothing to open.
	if res := page("/", launched); res.status != http.StatusSeeOther || location(res) != "/account/create" {
		t.Fatalf("/ with no account = %d %s", res.status, location(res))
	}

	if res := page(
		"/account/create",
		"",
	); res.status != http.StatusOK ||
		!strings.Contains(unescaped(res.body), "Account/Create") {
		t.Fatalf("/account/create = %d", res.status)
	}

	if res := page("/account/signin", ""); res.status != http.StatusSeeOther || location(res) != "/account/create" {
		t.Fatalf("/account/signin with no account = %d %s", res.status, location(res))
	}

	a.create()

	// Locked: every page but the account ones is the sign-in, with a session or without one.
	for _, c := range []string{launched, ""} {
		if res := page("/projects", c); res.status != http.StatusSeeOther || location(res) != "/account/signin" {
			t.Fatalf("/projects while locked (cookie %q) = %d %s", c, res.status, location(res))
		}
	}

	if res := page(
		"/account/signin",
		"",
	); res.status != http.StatusOK ||
		!strings.Contains(unescaped(res.body), "Account/SignIn") {
		t.Fatalf("/account/signin = %d", res.status)
	}

	if res := page("/account/create", ""); res.status != http.StatusSeeOther || location(res) != "/" {
		t.Fatalf("/account/create with an account = %d %s", res.status, location(res))
	}

	cookie, _ := a.signIn(accPassword)

	if res := page("/projects", cookie); res.status != http.StatusOK {
		t.Fatalf("/projects when unlocked = %d", res.status)
	}

	// Unlocked but no session in this browser: the sign-in page, not the instructions of `stuarty open`.
	if res := page("/projects", ""); res.status != http.StatusSeeOther || location(res) != "/account/signin" {
		t.Fatalf("/projects with no session = %d %s", res.status, location(res))
	}
}
