package httpapi_test

import (
	"io"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/web/app/auth/googletest"
)

// raw is a request exactly as given: no Origin and no JSON content type unless the headers say so, which is
// what a navigation from another site looks like.
func (s *googleSite) raw(t *testing.T, method, host, path string, headers map[string]string) int {
	t.Helper()

	hr, err := http.NewRequest(method, s.srv.URL+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	hr.Host = host

	for k, v := range headers {
		hr.Header.Set(k, v)
	}

	client := *s.srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	res, err := client.Do(hr)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	_, _ = io.Copy(io.Discard, res.Body)

	return res.StatusCode
}

// The callback page of the Google sign-in is the second cross-site navigation, as narrow as the first
// (SDD-007 section 3.6 item 4): a GET that loads a document, to that one path, in network mode only.
func TestSecurity_T82_CallbackIsTheOnlyCrossSiteNavigation(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)

	navigate := map[string]string{
		"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document",
	}

	with := func(extra map[string]string) map[string]string {
		out := map[string]string{}
		maps.Copy(out, navigate)
		maps.Copy(out, extra)

		return out
	}

	const callback = "/login/google/callback?state=l.x&code=y"

	// The navigation Google makes is let through; so is the same-site one of a fake on another port.
	for _, site := range []string{"cross-site", "same-site"} {
		if got := s.raw(
			t,
			http.MethodGet,
			netHost,
			callback,
			with(map[string]string{"Sec-Fetch-Site": site}),
		); got != http.StatusOK {
			t.Errorf("a %s navigation to the callback page = %d, want 200", site, got)
		}
	}

	refused := map[string]struct {
		method, path string
		headers      map[string]string
	}{
		"a fetch of the page": {
			http.MethodGet,
			callback,
			with(map[string]string{"Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty"}),
		},
		"the page in a frame":      {http.MethodGet, callback, with(map[string]string{"Sec-Fetch-Dest": "iframe"})},
		"a form post to the page":  {http.MethodPost, callback, navigate},
		"the finish by navigation": {http.MethodPost, "/auth/google/complete", navigate},
		"the start by navigation":  {http.MethodPost, "/auth/google/start", navigate},
		"the state of the feature": {http.MethodGet, "/auth/google", navigate},
		// The first exception, /social/callback, is epic 18's and is not widened: a fetch of it is refused.
		"the social callback by a fetch": {
			http.MethodGet,
			"/social/callback?state=x",
			with(map[string]string{"Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty"}),
		},
		"a project page": {http.MethodGet, "/projects", navigate},
		"the login page": {http.MethodGet, "/login", navigate},
		"another origin on it": {
			http.MethodGet,
			callback,
			with(map[string]string{"Origin": "https://evil.example.com"}),
		},
		"another host": {http.MethodGet, callback, navigate},
	}

	for name, c := range refused {
		host := netHost
		if name == "another host" {
			host = "evil.example.com"
		}

		if got := s.raw(
			t,
			c.method,
			host,
			c.path,
			c.headers,
		); got != http.StatusForbidden &&
			got != http.StatusNotFound &&
			got != http.StatusMethodNotAllowed {
			t.Errorf("%s = %d, want it refused", name, got)
		}
	}

	// The cross-site exceptions that are refused must be refused by the guard (403), not merely unrouted.
	for _, name := range []string{"a fetch of the page", "the page in a frame", "the finish by navigation", "a project page"} {
		c := refused[name]
		if got := s.raw(t, c.method, netHost, c.path, c.headers); got != http.StatusForbidden {
			t.Errorf("%s = %d, want 403 from the guard", name, got)
		}
	}

	// In local mode the exception does not exist at all.
	local := newHarness(t)

	res := local.do(req{path: "/login/google/callback", noAuth: true, headers: navigate})
	if res.status != http.StatusForbidden {
		t.Errorf("the callback page cross-site in local mode = %d, want 403", res.status)
	}
}

// An account takeover by e-mail match never works: neither signing in nor linking looks a user up by e-mail
// (SDD-007 section 2, T-81).
func TestSecurity_T81_EmailMatchNeverLinksOrSignsIn(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.linkedUser(t, "ana@example.com", "sub-ana")
	s.user(t, "bob@example.com", service.RoleMember)

	// The attacker owns a Google account whose verified e-mail is Ana's (and Bob's): a sign-in is refused.
	for _, victim := range []string{"ana@example.com", "bob@example.com"} {
		a, binding := s.startSignIn(t, nil)

		res := s.complete(t, s.back(a, "sub-attacker", victim), cookies(gsiCookieName, binding))
		if res.status != http.StatusUnauthorized || res.cookie != nil {
			t.Errorf("sign-in with the e-mail of %s = %d %s, want it refused", victim, res.status, res.body)
		}
	}

	// Linking that account to the attacker's own user does not touch the users it names by e-mail.
	s.user(t, "mallory@example.com", service.RoleMember)
	session := s.session(t, "mallory@example.com")
	link := authorizeOf(t, s.send(t, http.MethodPost, "/auth/google/link", `{"password":"`+goodPass+`"}`, session, nil))

	if res := s.complete(
		t,
		s.back(link, "sub-attacker", "bob@example.com"),
		cookies(cookieName, session),
	); res.status != http.StatusOK {
		t.Fatalf("mallory linking her own Google account = %d %s", res.status, res.body)
	}

	bob, err := s.auth.Users.ByEmail(t.Context(), "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if _, has, _ := s.google.Identity(t.Context(), bob.ID); has {
		t.Error("an identity with Bob's e-mail was attached to Bob")
	}

	// And the attacker's account now signs in as Mallory, never as Bob.
	a, binding := s.startSignIn(t, nil)

	res := s.complete(t, s.back(a, "sub-attacker", "bob@example.com"), cookies(gsiCookieName, binding))
	if res.status != http.StatusOK || !strings.Contains(string(res.body), `"email":"mallory@example.com"`) {
		t.Errorf("the attacker's account signed in as %s %s, want Mallory", res.body, res.header)
	}
}

// A token that Google did not vouch for for this flow is refused whatever it claims (T-84).
func TestSecurity_T84_ForgedIDTokenIsRefused(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.linkedUser(t, "ana@example.com", "sub-ana")

	cases := map[string]func(g *googletest.Grant){
		"another audience":    func(g *googletest.Grant) { g.Audience = "another-app.apps.example.test" },
		"an expired token":    func(g *googletest.Grant) { g.Expired = true },
		"a bad signature":     func(g *googletest.Grant) { g.BadSignature = true },
		"the nonce of a flow": func(g *googletest.Grant) { g.Nonce = "the-nonce-of-another-flow" },
		"another issuer":      func(g *googletest.Grant) { g.Issuer = "https://evil.example.com" },
		"an unverified mail":  func(g *googletest.Grant) { g.Unverified = true },
	}

	for name, mutate := range cases {
		a, binding := s.startSignIn(t, nil)

		grant := googletest.Grant{Subject: "sub-ana", Email: "ana@example.com", Nonce: a.nonce}
		mutate(&grant)

		code := s.fake.Grant(grant)
		res := s.complete(t, `{"state":"`+a.state+`","code":"`+code+`"}`, cookies(gsiCookieName, binding))

		if res.status != http.StatusUnauthorized || res.cookie != nil {
			t.Errorf("%s: complete = %d %s, want 401 and no session", name, res.status, res.body)
		}

		// The answer names no claim and no token.
		if strings.Contains(string(res.body), "eyJ") || strings.Contains(res.message(), "audience") ||
			strings.Contains(res.message(), "nonce") {
			t.Errorf("%s: the answer says too much: %s", name, res.body)
		}
	}

	// The same flow with a good token still works, so the refusals above were for the token.
	a, binding := s.startSignIn(t, nil)
	if res := s.complete(
		t,
		s.back(a, "sub-ana", "ana@example.com"),
		cookies(gsiCookieName, binding),
	); res.status != http.StatusOK {
		t.Errorf("a good token = %d %s", res.status, res.body)
	}
}

// Link and unlink need the password in the same request: a stolen session alone changes nothing (T-85).
func TestSecurity_T85_WrongPasswordBeginsNoFlow(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.user(t, "ana@example.com", service.RoleMember)
	session := s.session(t, "ana@example.com")

	for _, body := range []string{`{"password":"wrong password 123"}`, `{"password":""}`, `{}`} {
		res := s.send(t, http.MethodPost, "/auth/google/link", body, session, nil)
		if res.status < 400 || strings.Contains(string(res.body), "authorize_url") {
			t.Errorf("link with %s = %d %s, want a refusal and no address", body, res.status, res.body)
		}
	}

	if _, has, _ := s.google.Identity(t.Context(), mustUser(t, s, "ana@example.com")); has {
		t.Error("an identity was linked without the password")
	}
}

func mustUser(t *testing.T, s *googleSite, email string) string {
	t.Helper()

	u, err := s.auth.Users.ByEmail(t.Context(), email)
	if err != nil {
		t.Fatal(err)
	}

	return u.ID
}

func TestSecurity_T85_SettingsNeedAnAdministratorSession(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.user(t, "root@example.com", service.RoleAdmin)
	s.user(t, "ana@example.com", service.RoleMember)
	root, ana := s.session(t, "root@example.com"), s.session(t, "ana@example.com")
	key := s.makeKey(t, root, `{"name":"ops","scopes":["read","admin"],"expires_in_days":30}`)

	body := `{"enabled":true,"connection_id":"` + connectionstest.ID + `"}`

	for _, c := range []struct{ method, body string }{{http.MethodGet, ""}, {http.MethodPut, body}} {
		if got := s.send(t, c.method, "/auth/google/settings", c.body, ana, nil); got.status != http.StatusForbidden {
			t.Errorf("a member: %s = %d %s, want 403", c.method, got.status, got.body)
		}

		if got := s.asKey(
			t,
			key.Secret,
			c.method,
			"/auth/google/settings",
			c.body,
		); got.status != http.StatusForbidden {
			t.Errorf("a key with admin: %s = %d %s, want 403", c.method, got.status, got.body)
		}

		if got := s.send(t, c.method, "/auth/google/settings", c.body, "", nil); got.status != http.StatusUnauthorized {
			t.Errorf("no credential: %s = %d, want 401", c.method, got.status)
		}
	}

	// A change that a cookie request makes without showing where it comes from is refused.
	if got := s.send(t, http.MethodPut, "/auth/google/settings", body, root, map[string]string{
		"Origin": "https://evil.example.com", "Sec-Fetch-Site": "cross-site",
	}); got.status != http.StatusForbidden {
		t.Errorf("a cross-site PUT = %d %s, want 403", got.status, got.body)
	}

	// None of it turned the feature on.
	if got := s.send(
		t,
		http.MethodGet,
		"/auth/google",
		"",
		"",
		nil,
	); !strings.Contains(
		string(got.body),
		`"enabled":false`,
	) {
		t.Errorf("the refusals changed the policy: %s", got.body)
	}
}
