package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
)

func rateHarness(t *testing.T) (*networkHarness, string) {
	t.Helper()

	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")
	key := h.makeKey(t, ana, `{"name":"agent","scopes":["read"]}`)

	return h, key.Secret
}

// SDD-007 section 4.4: a key has a burst of 60 and 300 a minute, so the 361st
// request within a minute is 429 with Retry-After and E_RATE_LIMITED (feeds
// TestSecurity_T18_RateLimitPerKey). The limiter refills while the test runs, so
// the first refusal may come a few requests after the 361st, never before.
func TestRateLimitPerKey(t *testing.T) {
	h, secret := rateHarness(t)

	first := 0

	for i := 1; i <= 400 && first == 0; i++ {
		res := h.asKey(t, secret, http.MethodGet, "/api/v1/projects", "")

		switch res.status {
		case http.StatusOK:
		case http.StatusTooManyRequests:
			first = i

			if res.code() != service.CodeRateLimited || res.header.Get("Retry-After") == "" ||
				!strings.Contains(string(res.body), "retry_after") {
				t.Errorf("the refusal = %s, Retry-After %q", res.body, res.header.Get("Retry-After"))
			}
		default:
			t.Fatalf("request %d = %d %s", i, res.status, res.body)
		}
	}

	if first < 361 || first > 380 {
		t.Errorf("the first refusal was request %d, want from the 361st", first)
	}

	// Another key is not slowed by it.
	other := h.makeKey(t, h.session(t, "ana@example.com"), `{"name":"other","scopes":["read"]}`)
	if res := h.asKey(t, other.Secret, http.MethodGet, "/api/v1/projects", ""); res.status != http.StatusOK {
		t.Errorf("another key = %d, want 200", res.status)
	}
}

// The limits of the browser session and of the credential are separate and
// follow the variables.
func TestRateLimitPerSessionFollowsTheVariable(t *testing.T) {
	h := newNetworkHarnessRates(t, resolved(), true, nil, httpapi.RateLimits{PerSession: 5})
	h.user(t, "ana@example.com", service.RoleMember)

	id := h.session(t, "ana@example.com")
	refused := false

	// The bucket of the session limiter holds 5 plus 300.
	for range 330 {
		if res := h.send(t, http.MethodGet, "/api/v1/projects", "", id, nil); res.status == http.StatusTooManyRequests {
			refused = true
			break
		}
	}

	if !refused {
		t.Error("a session was never limited")
	}
}

// 21 failed key checks from one address within a minute are throttled (feeds
// TestSecurity_T32_FailedKeyAttemptsThrottled), and the address is refused
// until the limiter lets it through, even with a valid key.
func TestRateLimitFailedAuthenticationByAddress(t *testing.T) {
	h, secret := rateHarness(t)

	for i := 1; i <= 20; i++ {
		if res := h.asKey(
			t,
			"vvk_wrong.guess",
			http.MethodGet,
			"/api/v1/projects",
			"",
		); res.status != http.StatusUnauthorized {
			t.Fatalf("failed check %d = %d, want 401", i, res.status)
		}
	}

	res := h.asKey(t, "vvk_wrong.guess", http.MethodGet, "/api/v1/projects", "")
	if res.status != http.StatusTooManyRequests || res.code() != service.CodeRateLimited ||
		res.header.Get("Retry-After") == "" {
		t.Errorf("the 21st failed check = %d %s", res.status, res.body)
	}

	if res := h.asKey(t, secret, http.MethodGet, "/api/v1/projects", ""); res.status != http.StatusTooManyRequests {
		t.Errorf("a valid key from a refused address = %d, want 429", res.status)
	}
}

// The event stream, the byte-serving routes and the plan windows are not
// counted: 1,000 requests to the proxy route leave the budget whole.
func TestRateLimitDoesNotCountTheProxyRoute(t *testing.T) {
	h, secret := rateHarness(t)

	for i := range 1000 {
		res := h.asKey(t, secret, http.MethodGet, "/api/v1/projects/prj_0000000000/media/ast_0000000000/proxy", "")
		if res.status == http.StatusTooManyRequests {
			t.Fatalf("request %d to the proxy route was counted", i+1)
		}
	}

	if res := h.asKey(t, secret, http.MethodGet, "/api/v1/projects", ""); res.status != http.StatusOK {
		t.Errorf("a counted request after 1,000 uncounted = %d, want 200", res.status)
	}
}

// Local mode: only failed authentication is throttled by address; the
// per-token limiter runs with the same defaults.
func TestRateLimitLocalMode(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for i := 1; i <= 20; i++ {
		if res := h.do(req{path: "/api/v1/projects", noAuth: true}); res.status != http.StatusUnauthorized {
			t.Fatalf("unauthenticated request %d = %d, want 401", i, res.status)
		}
	}

	if res := h.do(req{path: "/api/v1/projects", noAuth: true}); res.status != http.StatusTooManyRequests {
		t.Errorf("the 21st unauthenticated request = %d, want 429", res.status)
	}
}
