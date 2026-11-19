package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
)

// gsi is the value of the sign-in cookie that a response set, and whether it was set to be cleared.
func gsi(res netResp) (value string, cleared bool) {
	for _, c := range (&http.Response{Header: res.header}).Cookies() {
		if c.Name == gsiCookieName {
			return c.Value, c.MaxAge < 0
		}
	}

	return "", false
}

// startSignIn is a browser with no session that presses "Sign in with Google".
func (s *googleSite) startSignIn(t *testing.T, headers map[string]string) (authorize, string) {
	t.Helper()

	res := s.send(t, http.MethodPost, "/auth/google/start", "", "", headers)
	a := authorizeOf(t, res)

	value, _ := gsi(res)
	if value == "" {
		t.Fatalf("start set no sign-in cookie: %v", res.header["Set-Cookie"])
	}

	return a, value
}

// linkedUser is a user of the server who linked the Google account sub and email.
func (s *googleSite) linkedUser(t *testing.T, email, sub string) {
	t.Helper()

	s.user(t, email, service.RoleMember)
	session := s.session(t, email)

	res := s.send(t, http.MethodPost, "/auth/google/link", `{"password":"`+goodPass+`"}`, session, nil)
	a := authorizeOf(t, res)

	res = s.complete(t, s.back(a, sub, email), cookies(cookieName, session))
	if res.status != http.StatusOK {
		t.Fatalf("linking %s = %d %s", email, res.status, res.body)
	}
}

func TestGoogleSignInOpensTheSameSessionAsAPassword(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.linkedUser(t, "ana@example.com", "sub-ana")

	password := s.login(t, "ana@example.com", goodPass)

	a, binding := s.startSignIn(t, nil)
	if !strings.HasPrefix(a.state, "l.") || a.redirect != netOrigin+"/login/google/callback" {
		t.Fatalf("authorize address = %+v", a)
	}

	res := s.complete(t, s.back(a, "sub-ana", "ana@example.com"), cookies(gsiCookieName, binding))
	if res.status != http.StatusOK || res.cookie == nil {
		t.Fatalf("complete = %d %s", res.status, res.body)
	}

	// The same cookie, with the same flags, and the same answer as POST /auth/login.
	c, p := res.cookie, password.cookie
	if c.Name != p.Name || c.Secure != p.Secure || c.HttpOnly != p.HttpOnly || c.SameSite != p.SameSite ||
		c.Path != p.Path || c.MaxAge != p.MaxAge || c.Value == p.Value {
		t.Errorf("session cookie = %+v, after a password login it is %+v", c, p)
	}

	for _, want := range []string{`"email":"ana@example.com"`, `"role":"member"`, `"scopes":[`} {
		if !strings.Contains(string(res.body), want) {
			t.Errorf("answer %s lacks %s", res.body, want)
		}
	}

	if res.header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", res.header.Get("Cache-Control"))
	}

	// It is a session of the server: it reads the API, and it is the session of Ana.
	if got := s.send(t, http.MethodGet, "/api/v1/projects", "", c.Value, nil); got.status != http.StatusOK {
		t.Errorf("GET /projects with the Google session = %d %s", got.status, got.body)
	}

	if got := s.send(
		t,
		http.MethodGet,
		"/auth/google",
		"",
		c.Value,
		nil,
	); !strings.Contains(
		string(got.body),
		`"linked":{"email":"ana@example.com"`,
	) {
		t.Errorf("GET /auth/google with the Google session = %s", got.body)
	}

	// The sign-in cookie served its flow and is cleared.
	if _, cleared := gsi(res); !cleared {
		t.Error("the sign-in cookie was not cleared")
	}

	// And the password login still works.
	if got := s.login(t, "ana@example.com", goodPass); got.status != http.StatusOK {
		t.Errorf("password login after Google sign-in = %d", got.status)
	}
}

func TestGoogleSignInRefusesAnIdentityThatWasNeverLinked(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.user(t, "ana@example.com", service.RoleAdmin)

	// A Google account whose verified e-mail is Ana's, which nobody linked, and one that is nothing to us.
	for _, c := range []struct{ sub, email string }{
		{"sub-stranger", "ana@example.com"}, {"sub-stranger", "nobody@example.net"},
	} {
		a, binding := s.startSignIn(t, nil)

		res := s.complete(t, s.back(a, c.sub, c.email), cookies(gsiCookieName, binding))
		if res.status != http.StatusUnauthorized || res.code() != service.CodeUnauthorized ||
			!strings.Contains(res.message(), "not linked") {
			t.Errorf("%s: complete = %d %s, want E_UNAUTHORIZED not linked", c.email, res.status, res.body)
		}

		if res.cookie != nil {
			t.Errorf("%s: a session cookie was set for an identity that is not linked", c.email)
		}
	}

	// Nothing was created: still one user.
	if n, err := s.auth.Users.Count(t.Context()); err != nil || n != 1 {
		t.Errorf("users = %d, %v, want the one that existed", n, err)
	}
}

func TestGoogleSignInIsBoundToTheBrowserThatBeganIt(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.linkedUser(t, "ana@example.com", "sub-ana")

	// An attacker begins a flow in his browser and gets the redirect for his own account; a page makes the
	// victim's browser finish it (login CSRF). The victim holds no cookie of that flow.
	a, attackerBinding := s.startSignIn(t, nil)
	_, victimBinding := s.startSignIn(t, nil)
	redirect := s.back(a, "sub-ana", "ana@example.com")

	for name, headers := range map[string]map[string]string{
		"no cookie":              nil,
		"another browser cookie": cookies(gsiCookieName, victimBinding),
	} {
		res := s.complete(t, redirect, headers)
		if res.status == http.StatusOK || res.cookie != nil {
			t.Fatalf("%s: complete = %d %s, a session was opened", name, res.status, res.body)
		}
	}

	// The state is spent by the refusals: the right browser cannot use it either.
	if res := s.complete(t, redirect, cookies(gsiCookieName, attackerBinding)); res.status == http.StatusOK {
		t.Errorf("a state that was refused once was accepted later: %s", res.body)
	}
}

func TestGoogleSignInRotatesTheSession(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.linkedUser(t, "ana@example.com", "sub-ana")
	s.user(t, "bob@example.com", service.RoleMember)

	// A session planted in the browser before the sign-in (fixation).
	planted := s.session(t, "bob@example.com")

	a, binding := s.startSignIn(t, cookies(cookieName, planted))
	res := s.complete(t, s.back(a, "sub-ana", "ana@example.com"), cookies(gsiCookieName, binding, cookieName, planted))

	if res.status != http.StatusOK || res.cookie == nil || res.cookie.Value == planted {
		t.Fatalf("complete = %d, cookie %+v: the session id did not change", res.status, res.cookie)
	}

	if got := s.send(t, http.MethodGet, "/api/v1/projects", "", planted, nil); got.status != http.StatusUnauthorized {
		t.Errorf("the planted session after the sign-in = %d, want 401", got.status)
	}
}

func TestGoogleSignInIsOffUntilTheOwnerTurnsItOn(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.linkedUser(t, "ana@example.com", "sub-ana")

	// Linking needed it on: the owner turns it off now.
	if _, err := s.google.SetSettings(t.Context(), auth.GoogleSettings{ConnectionID: connectionstest.ID}); err != nil {
		t.Fatal(err)
	}

	before := s.fake.TotalHits()

	for _, c := range []struct{ path, body string }{
		{"/auth/google/start", ""},
		{"/auth/google/complete", `{"state":"l.abc","code":"x"}`},
	} {
		if res := s.send(t, http.MethodPost, c.path, c.body, "", nil); res.status != http.StatusNotFound {
			t.Errorf("%s while off = %d %s, want 404", c.path, res.status, res.body)
		}
	}

	if got := s.fake.TotalHits() - before; got != 0 {
		t.Errorf("a server with Google sign-in off reached Google %d times", got)
	}
}

func TestGoogleSignInRefusesADisabledUser(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.linkedUser(t, "ana@example.com", "sub-ana")
	s.user(t, "root@example.com", service.RoleAdmin)

	ana, err := s.auth.Users.ByEmail(t.Context(), "ana@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.auth.Users.SetDisabled(t.Context(), ana.ID, true); err != nil {
		t.Fatal(err)
	}

	a, binding := s.startSignIn(t, nil)

	res := s.complete(t, s.back(a, "sub-ana", "ana@example.com"), cookies(gsiCookieName, binding))
	if res.status != http.StatusUnauthorized || res.cookie != nil {
		t.Errorf("a disabled user signed in with Google: %d %s", res.status, res.body)
	}
}

func TestGoogleSignInRoutesDoNotExistInLocalMode(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/auth/google"},
		{http.MethodPost, "/auth/google/start"},
		{http.MethodPost, "/auth/google/complete"},
		{http.MethodPost, "/auth/google/link"},
		{http.MethodPost, "/auth/google/unlink"},
		{http.MethodGet, "/auth/google/settings"},
		{http.MethodPut, "/auth/google/settings"},
		{http.MethodGet, "/login/google/callback"},
		{http.MethodGet, "/settings/google"},
	} {
		body := ""
		if c.method != http.MethodGet {
			body = `{"password":"` + goodPass + `","state":"l.x"}`
		}

		if res := h.do(req{method: c.method, path: c.path, body: body}); res.status != http.StatusNotFound {
			t.Errorf("%s %s in local mode = %d, want 404", c.method, c.path, res.status)
		}
	}
}
