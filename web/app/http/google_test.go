package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/auth/googletest"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
	"github.com/AlexGalhardo/stuarty/web/app/store"
	"github.com/AlexGalhardo/stuarty/web/bootstrap"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/auth/hash"
	"github.com/AlexGalhardo/vukem/ratelimit"
	"github.com/AlexGalhardo/vukem/vukemtest"
)

const (
	googleClientID = "1234-owner-project.apps.example.test"
	gsiCookieName  = "__Host-stuarty_gsi"
)

// googleSite is a server in network mode with the Google sign-in built over a fake Google on loopback and a
// connection of the owner's own project, which is what a deployment has (SDD-014 section 12).
type googleSite struct {
	*networkHarness

	fake   *googletest.Server
	google *auth.Google
}

type googleSiteOptions struct {
	// limits replaces the login limits; nil is loose, as the other tests of network mode.
	limits *httpapi.LoginLimits
}

func newGoogleSite(t *testing.T, o googleSiteOptions) *googleSite {
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

	logs := &bytes.Buffer{}

	netw, err := httpapi.NewNetwork(resolved(), slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}

	clk := &clock{t: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)}

	netAuth, err := auth.NewNetwork(st.DB(), auth.NetworkOptions{Now: clk.now, Hasher: hash.Fake()})
	if err != nil {
		t.Fatal(err)
	}

	svc, err := service.New(service.Options{Store: st, Now: clk.now, Keys: netAuth.Keys()})
	if err != nil {
		t.Fatal(err)
	}

	fake := googletest.New(t)
	conns := connectionstest.New(t, "youtube", fake.URL,
		`{"client_id":"`+googleClientID+`","client_secret":"the-client-secret-of-the-owner"}`)

	google, err := auth.NewGoogle(auth.GoogleOptions{
		DB: st.DB(), Users: netAuth.Users, Connections: conns.Manager, AppURL: netOrigin,
		// The real clock: the fake Google signs its tokens with it, not with the stub of the harness.
		Endpoints: auth.TestEndpoints(fake.URL),
	})
	if err != nil {
		t.Fatal(err)
	}

	limits := o.limits
	if limits == nil {
		loose := ratelimit.NewMemory(1_000_000, time.Minute)
		limits = &httpapi.LoginLimits{PerEmailAndAddress: loose, PerEmail: loose, PerAddress: loose}
	}

	deps := &httpapi.Deps{
		Service: svc, Registry: command.Default(), Workspace: dir, Network: netw, Auth: netAuth, Google: google,
		RuntimeSecret: testRuntimeSecret, LoginLimits: limits, Sessions: auth.NewSessions(clk.now),
		Launch: auth.NewLaunch(clk.now), Now: clk.now,
	}

	app := vukemtest.New(t, func(opts ...vukem.Option) (*vukem.App, error) { return bootstrap.NewWith(deps, opts...) })

	h := &networkHarness{log: logs, auth: netAuth, clk: clk, st: st, srv: httptest.NewTLSServer(app.Handler())}
	t.Cleanup(h.srv.Close)

	return &googleSite{networkHarness: h, fake: fake, google: google}
}

// enable is what the administrator does in Settings, written straight to the settings.
func (s *googleSite) enable(t *testing.T, allowed ...string) {
	t.Helper()

	_, err := s.google.SetSettings(t.Context(), auth.GoogleSettings{
		Enabled: true, ConnectionID: connectionstest.ID, AllowedEmails: allowed,
	})
	if err != nil {
		t.Fatal(err)
	}
}

// cookies is the Cookie header of a browser that holds these values.
func cookies(pairs ...string) map[string]string {
	var parts []string

	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			parts = append(parts, pairs[i]+"="+pairs[i+1])
		}
	}

	return map[string]string{"Cookie": strings.Join(parts, "; ")}
}

// authorize is the address Google's consent screen is opened at, from the answer of start or link.
type authorize struct{ state, nonce, clientID, redirect, challenge, scope string }

func authorizeOf(t *testing.T, res netResp) authorize {
	t.Helper()

	var body struct {
		AuthorizeURL string `json:"authorize_url"`
	}

	if res.status != http.StatusOK || json.Unmarshal(res.body, &body) != nil || body.AuthorizeURL == "" {
		t.Fatalf("no authorize address: %d %s", res.status, res.body)
	}

	u, err := url.Parse(body.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}

	q := u.Query()

	return authorize{
		state: q.Get("state"), nonce: q.Get("nonce"), clientID: q.Get("client_id"), redirect: q.Get("redirect_uri"),
		challenge: q.Get("code_challenge"), scope: q.Get("scope"),
	}
}

// back is the redirect of Google for the flow: the person has the Google account sub and email.
func (s *googleSite) back(a authorize, sub, email string) string {
	code := s.fake.Grant(googletest.Grant{Subject: sub, Email: email, Nonce: a.nonce})

	return `{"state":"` + a.state + `","code":"` + code + `"}`
}

func (s *googleSite) complete(t *testing.T, body string, headers map[string]string) netResp {
	t.Helper()

	return s.send(t, http.MethodPost, "/auth/google/complete", body, "", headers)
}

func TestLinkIdentityOverHTTP(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.user(t, "ana@example.com", service.RoleMember)
	session := s.session(t, "ana@example.com")
	withSession := cookies(cookieName, session)

	// The page that decides what to show: on, and nothing linked yet.
	res := s.send(t, http.MethodGet, "/auth/google", "", session, nil)
	if res.status != http.StatusOK || !strings.Contains(string(res.body), `"enabled":true`) ||
		!strings.Contains(string(res.body), `"linked":null`) {
		t.Fatalf("GET /auth/google = %d %s", res.status, res.body)
	}

	// Linking asks for the password again.
	res = s.send(t, http.MethodPost, "/auth/google/link", `{"password":"not the password"}`, session, nil)
	if res.status != http.StatusBadRequest || res.code() != command.CodeValidation {
		t.Fatalf("link with a wrong password = %d %s, want E_VALIDATION", res.status, res.body)
	}

	res = s.send(t, http.MethodPost, "/auth/google/link", `{"password":"`+goodPass+`"}`, session, nil)
	a := authorizeOf(t, res)

	if a.clientID != googleClientID || a.redirect != netOrigin+"/login/google/callback" || a.challenge == "" ||
		a.scope != "openid email" || !strings.HasPrefix(a.state, "k.") || a.nonce == "" {
		t.Fatalf("authorize address = %+v", a)
	}

	// Google sends the browser back; the page posts the state and the code with the session.
	res = s.complete(t, s.back(a, "sub-ana", "Ana@Gmail.test"), withSession)
	if res.status != http.StatusOK || !strings.Contains(string(res.body), `"email":"ana@gmail.test"`) {
		t.Fatalf("complete = %d %s", res.status, res.body)
	}

	res = s.send(t, http.MethodGet, "/auth/google", "", session, nil)
	if !strings.Contains(string(res.body), `"email":"ana@gmail.test"`) {
		t.Fatalf("GET /auth/google after linking = %s", res.body)
	}

	// No token, no secret in anything the server said.
	for _, leak := range []string{"fake-access-token", "eyJ", "the-client-secret-of-the-owner"} {
		if strings.Contains(string(res.body), leak) || strings.Contains(s.log.String(), leak) {
			t.Errorf("%q is in an answer or in the log", leak)
		}
	}

	// A second link is refused while one exists.
	res = s.send(t, http.MethodPost, "/auth/google/link", `{"password":"`+goodPass+`"}`, session, nil)
	a = authorizeOf(t, res)

	res = s.complete(t, s.back(a, "sub-other", "other@gmail.test"), withSession)
	if res.code() != command.CodeInvalidState {
		t.Errorf("a second link = %d %s, want E_INVALID_STATE", res.status, res.body)
	}

	// Unlinking asks for the password too; afterwards the password login still works.
	res = s.send(t, http.MethodPost, "/auth/google/unlink", `{"password":"wrong password 123"}`, session, nil)
	if res.status != http.StatusBadRequest {
		t.Errorf("unlink with a wrong password = %d %s", res.status, res.body)
	}

	res = s.send(t, http.MethodPost, "/auth/google/unlink", `{"password":"`+goodPass+`"}`, session, nil)
	if res.status != http.StatusOK || !strings.Contains(string(res.body), `"enabled":true`) {
		t.Fatalf("unlink = %d %s", res.status, res.body)
	}

	if res := s.send(
		t,
		http.MethodGet,
		"/auth/google",
		"",
		session,
		nil,
	); !strings.Contains(
		string(res.body),
		`"linked":null`,
	) {
		t.Errorf("GET /auth/google after unlinking = %s", res.body)
	}

	if res := s.login(t, "ana@example.com", goodPass); res.status != http.StatusOK {
		t.Errorf("the password login after unlinking = %d %s", res.status, res.body)
	}

	if res := s.send(
		t,
		http.MethodPost,
		"/auth/google/unlink",
		`{"password":"`+goodPass+`"}`,
		session,
		nil,
	); res.status != http.StatusNotFound {
		t.Errorf("unlinking twice = %d %s, want 404", res.status, res.body)
	}
}

func TestLinkIdentityIsForASessionWithThePassword(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.user(t, "ana@example.com", service.RoleAdmin)
	session := s.session(t, "ana@example.com")
	key := s.makeKey(t, session, `{"name":"ops","scopes":["read","admin"],"expires_in_days":30}`)

	for _, path := range []string{"/auth/google/link", "/auth/google/unlink"} {
		body := `{"password":"` + goodPass + `"}`

		// A key never links, whatever its scopes: it is not a person at a browser.
		if res := s.asKey(t, key.Secret, http.MethodPost, path, body); res.status != http.StatusForbidden {
			t.Errorf("a key calling %s = %d %s, want 403", path, res.status, res.body)
		}

		if res := s.send(t, http.MethodPost, path, body, "", nil); res.status != http.StatusUnauthorized {
			t.Errorf("no credential calling %s = %d, want 401", path, res.status)
		}

		// A cookie request that shows no origin and is not JSON is refused before it is read.
		if res := s.send(t, http.MethodPost, path, body, session, map[string]string{
			"Origin": "", "Content-Type": "application/x-www-form-urlencoded",
		}); res.status != http.StatusForbidden {
			t.Errorf("a form post to %s = %d %s, want 403", path, res.status, res.body)
		}

		// Another site is refused by the guard.
		if res := s.send(t, http.MethodPost, path, body, session, map[string]string{
			"Origin": "https://evil.example.com", "Sec-Fetch-Site": "cross-site",
		}); res.status != http.StatusForbidden {
			t.Errorf("a cross-site post to %s = %d %s, want 403", path, res.status, res.body)
		}
	}
}

func TestLinkIdentityIsOffUntilTheOwnerTurnsItOn(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.user(t, "ana@example.com", service.RoleMember)
	session := s.session(t, "ana@example.com")

	res := s.send(t, http.MethodGet, "/auth/google", "", "", nil)
	if res.status != http.StatusOK || !strings.Contains(string(res.body), `"enabled":false`) {
		t.Fatalf("GET /auth/google = %d %s", res.status, res.body)
	}

	res = s.send(t, http.MethodPost, "/auth/google/link", `{"password":"`+goodPass+`"}`, session, nil)
	if res.status != http.StatusNotFound {
		t.Errorf("link while off = %d %s, want 404", res.status, res.body)
	}

	if n := s.fake.TotalHits(); n != 0 {
		t.Errorf("a server with Google sign-in off made %d requests to Google", n)
	}
}
