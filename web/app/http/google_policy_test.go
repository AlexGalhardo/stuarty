package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"

	"github.com/AlexGalhardo/vukem/ratelimit"
)

func TestGoogleSignInPolicyIsSetByAnAdministratorSession(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.user(t, "root@example.com", service.RoleAdmin)
	s.user(t, "ana@example.com", service.RoleMember)
	root, ana := s.session(t, "root@example.com"), s.session(t, "ana@example.com")
	key := s.makeKey(t, root, `{"name":"ops","scopes":["read","admin"],"expires_in_days":30}`)

	// Off by default, with the address to register in the Google project.
	res := s.send(t, http.MethodGet, "/auth/google/settings", "", root, nil)
	if res.status != http.StatusOK || !strings.Contains(string(res.body), `"enabled":false`) ||
		!strings.Contains(string(res.body), `"redirect_url":"`+netOrigin+`/login/google/callback"`) {
		t.Fatalf("GET settings = %d %s", res.status, res.body)
	}

	// Only an administrator session reads or sets them: not a member, not a key with the admin scope.
	body := `{"enabled":true,"connection_id":"` + connectionstest.ID + `"}`

	for _, c := range []struct{ method, body string }{{http.MethodGet, ""}, {http.MethodPut, body}} {
		if got := s.send(t, c.method, "/auth/google/settings", c.body, ana, nil); got.status != http.StatusForbidden {
			t.Errorf("a member: %s settings = %d %s, want 403", c.method, got.status, got.body)
		}

		if got := s.asKey(
			t,
			key.Secret,
			c.method,
			"/auth/google/settings",
			c.body,
		); got.status != http.StatusForbidden {
			t.Errorf("a key: %s settings = %d %s, want 403", c.method, got.status, got.body)
		}

		if got := s.send(t, c.method, "/auth/google/settings", c.body, "", nil); got.status != http.StatusUnauthorized {
			t.Errorf("no credential: %s settings = %d, want 401", c.method, got.status)
		}
	}

	// Refused changes say which member is wrong, and keep it off.
	for _, bad := range []string{
		`{"enabled":true}`,
		`{"enabled":true,"connection_id":"con_nothingnothing"}`,
		`{"enabled":false,"allowed_emails":["not an address"]}`,
	} {
		if got := s.send(
			t,
			http.MethodPut,
			"/auth/google/settings",
			bad,
			root,
			nil,
		); got.status != http.StatusBadRequest {
			t.Errorf("PUT %s = %d %s, want 400", bad, got.status, got.body)
		}
	}

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
		t.Errorf("a refused change turned it on: %s", got.body)
	}

	// On, with a list: anyone can see it is on, nobody learns the list.
	res = s.send(t, http.MethodPut, "/auth/google/settings",
		`{"enabled":true,"connection_id":"`+connectionstest.ID+`","allowed_emails":["Ana@Gmail.test"]}`, root, nil)
	if res.status != http.StatusOK || !strings.Contains(string(res.body), `"allowed_emails":["ana@gmail.test"]`) {
		t.Fatalf("PUT settings = %d %s", res.status, res.body)
	}

	got := s.send(t, http.MethodGet, "/auth/google", "", "", nil)
	if !strings.Contains(string(got.body), `"enabled":true`) || strings.Contains(string(got.body), "gmail") ||
		strings.Contains(string(got.body), connectionstest.ID) {
		t.Errorf("GET /auth/google = %s, want only whether it is on", got.body)
	}

	// Off again.
	if got := s.send(
		t,
		http.MethodPut,
		"/auth/google/settings",
		`{"enabled":false}`,
		root,
		nil,
	); got.status != http.StatusOK {
		t.Fatalf("PUT off = %d %s", got.status, got.body)
	}

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
		t.Errorf("GET /auth/google after turning it off = %s", got.body)
	}
}

func TestGoogleSignInPolicyListHoldsOverHTTP(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)
	s.linkedUser(t, "ana@example.com", "sub-ana")
	s.linkedUser(t, "bob@example.com", "sub-bob")

	// The owner restricts sign-in to Ana's address; Bob is linked and is refused.
	s.enable(t, "ana@example.com")

	for _, c := range []struct {
		sub, email string
		want       int
	}{
		{"sub-ana", "ana@example.com", http.StatusOK},
		{"sub-bob", "bob@example.com", http.StatusUnauthorized},
	} {
		a, binding := s.startSignIn(t, nil)

		res := s.complete(t, s.back(a, c.sub, c.email), cookies(gsiCookieName, binding))
		if res.status != c.want || (c.want == http.StatusOK) != (res.cookie != nil) {
			t.Errorf("%s: complete = %d %s, want %d", c.email, res.status, res.body, c.want)
		}
	}
}

func TestSecurity_T82_GoogleRoutesAreRateLimited(t *testing.T) {
	const burst = 5

	s := newGoogleSite(t, googleSiteOptions{limits: &httpapi.LoginLimits{
		PerEmailAndAddress: ratelimit.NewMemory(1000, time.Minute),
		PerEmail:           ratelimit.NewMemory(1000, time.Minute),
		PerAddress:         ratelimit.NewMemory(burst, time.Minute),
	}})
	s.enable(t)

	for i := range burst {
		if res := s.send(t, http.MethodPost, "/auth/google/start", "", "", nil); res.status != http.StatusOK {
			t.Fatalf("start %d = %d %s", i+1, res.status, res.body)
		}
	}

	res := s.send(t, http.MethodPost, "/auth/google/start", "", "", nil)
	if res.status != http.StatusTooManyRequests || res.code() != service.CodeRateLimited ||
		res.header.Get("Retry-After") == "" {
		t.Errorf("start beyond the limit = %d %s, want E_RATE_LIMITED with Retry-After", res.status, res.body)
	}

	// The finish is counted against the same address: a guesser cannot try states for free.
	res = s.complete(t, `{"state":"l.guess","code":"x"}`, nil)
	if res.status != http.StatusTooManyRequests {
		t.Errorf("complete beyond the limit = %d %s, want 429", res.status, res.body)
	}
}

func TestSecurity_T82_RefusalsSpendTheFailedAuthenticationsOfTheAddress(t *testing.T) {
	s := newGoogleSite(t, googleSiteOptions{})
	s.enable(t)

	// An address that keeps presenting identities nobody linked ends up blocked, like one that guesses passwords
	// (20 a minute, SDD-007 section 4.4).
	blocked := false

	for range 40 {
		a, binding := s.startSignIn(t, nil)

		res := s.complete(t, s.back(a, "sub-stranger", "stranger@example.net"), cookies(gsiCookieName, binding))
		if res.status == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}

	if !blocked {
		t.Error("forty refused Google sign-ins from one address were never rate limited")
	}
}
