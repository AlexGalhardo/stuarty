package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/events"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
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
// request within a minute is 429 with Retry-After and E_RATE_LIMITED. The
// limiter refills while the test runs, so the first refusal may come a few requests after the 361st, never before.
func TestSecurity_T18_RateLimitPerKey(t *testing.T) {
	h, secret := rateHarness(t)

	first := 0
	start := time.Now()

	for i := 1; i <= 1000 && first == 0; i++ {
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

	// 300 a minute refills 5 tokens a second, so a slow run (the race detector)
	// is refused later by about that much, never earlier.
	late := 361 + int(time.Since(start).Seconds()*5) + 1
	if first < 361 || first > late {
		t.Errorf("the first refusal was request %d, want from the 361st to the %dth", first, late)
	}

	// Another key is not slowed by it.
	other := h.makeKey(t, h.session(t, "ana@example.com"), `{"name":"other","scopes":["read"]}`)
	if res := h.asKey(t, other.Secret, http.MethodGet, "/api/v1/projects", ""); res.status != http.StatusOK {
		t.Errorf("another key = %d, want 200", res.status)
	}
}

// P4-SRV-003, SDD-007 section 4.4: in network mode an address has 600 requests a minute before
// authentication; the 601st is E_RATE_LIMITED naming the limit, whatever the route answers.
func TestRateLimitPreAuthenticationPerAddress(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	session := h.session(t, "ana@example.com")

	first := 0
	start := time.Now()

	// A session is no failed authentication, so only the limiter by address can refuse.
	for i := 1; i <= 800 && first == 0; i++ {
		res := h.send(t, http.MethodGet, "/api/v1/projects", "", session, nil)
		if res.status == http.StatusTooManyRequests {
			first = i

			var body struct {
				Details map[string]any `json:"details"`
			}

			_ = json.Unmarshal(res.body, &body)

			d := body.Details
			if res.code() != service.CodeRateLimited || d["limit"] != "requests" || d["ceiling"] != 600.0 {
				t.Errorf("the refusal = %s", res.body)
			}
		}
	}

	// 600 a minute refills 10 tokens a second, so a slow run is refused later by about that much.
	// The login that made the session spent a few of the 600.
	late := 601 + int(time.Since(start).Seconds()*10) + 1
	if first < 590 || first > late {
		t.Errorf("the first refusal was request %d, want from the 590th to the %dth", first, late)
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

// 21 failed key checks from one address within a minute are throttled,
// and the address is refused until the limiter lets it through, even with a valid key.
func TestSecurity_T32_FailedKeyAttemptsThrottled(t *testing.T) {
	h, secret := rateHarness(t)
	session := h.session(t, "ana@example.com")

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

	// A blocked address is refused before the key is verified: a valid key
	// gets the very answer of a bad one, so the block is no guessing oracle.
	good := h.asKey(t, secret, http.MethodGet, "/api/v1/projects", "")
	bad := h.asKey(t, "vvk_wrong.guess", http.MethodGet, "/api/v1/projects", "")

	if good.status != http.StatusTooManyRequests || bad.status != good.status ||
		good.code() != bad.code() || good.message() != bad.message() {
		t.Errorf("valid key %d %s, bad key %d %s: want the same refusal", good.status, good.body, bad.status, bad.body)
	}

	// A session that is already established still works, and a login is refused
	// unverified.
	if res := h.send(t, http.MethodGet, "/api/v1/projects", "", session, nil); res.status != http.StatusOK {
		t.Errorf("an established session from a refused address = %d, want 200", res.status)
	}

	if res := h.login(t, "ana@example.com", goodPass); res.status != http.StatusTooManyRequests || res.cookie != nil {
		t.Errorf("a login from a refused address = %d, want 429 and no cookie", res.status)
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

// Local mode: every client is the loopback address, so bad tokens never lock
// out the good one.
func TestRateLimitLocalMode(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for i := 1; i <= 40; i++ {
		if res := h.do(req{path: "/api/v1/projects", noAuth: true}); res.status != http.StatusUnauthorized {
			t.Fatalf("unauthenticated request %d = %d, want 401", i, res.status)
		}
	}

	if res := h.get("/api/v1/projects"); res.status != http.StatusOK {
		t.Errorf("the local token after bad requests = %d, want 200", res.status)
	}
}

// slowFrames holds every ViewFrame until release is closed.
type slowFrames struct {
	service.Service
	started chan struct{}
	release chan struct{}
}

func (s slowFrames) ViewFrame(_ context.Context, _ service.ViewFrameRequest) (*service.ViewResult, error) {
	s.started <- struct{}{}
	<-s.release

	return nil, &service.Error{Code: "E_NOT_FOUND", Message: "no frame in this test."}
}

// ViewFrame is exempt from the request counter because the editor scrubs, but it
// renders on a cache miss: one caller has at most 2 in flight, then E_QUEUE_FULL
// (SDD-007 section 4.4 names the cached frames among the uncounted routes).
func TestRateLimitFrameConcurrencyCap(t *testing.T) {
	slow := slowFrames{started: make(chan struct{}, 8), release: make(chan struct{})}
	h := newHarnessWrapped(t, events.Options{}, func(s service.Service) service.Service {
		slow.Service = s
		return slow
	})

	const path = "/api/v1/projects/prj_0000000001/frame?at=0s"

	var wg sync.WaitGroup

	for range 2 {
		wg.Go(func() { h.get(path) })
		<-slow.started
	}

	res := h.get(path)
	if res.status != http.StatusTooManyRequests || res.json(t)["code"] != service.CodeQueueFull {
		t.Errorf("the third frame in flight = %d %s, want 429 E_QUEUE_FULL", res.status, res.body)
	}

	if d, _ := res.json(t)["details"].(map[string]any); d["limit"] != "frames" || d["value"] != 2.0 ||
		d["ceiling"] != 2.0 {
		t.Errorf("details = %v, want limit frames, value 2, ceiling 2", d)
	}

	// Another route of the same caller is not affected.
	if res := h.get("/api/v1/projects"); res.status != http.StatusOK {
		t.Errorf("another route during the frames = %d", res.status)
	}

	close(slow.release)
	wg.Wait()

	// The places came back.
	go func() { <-slow.started }()

	if res := h.get(path); res.status == http.StatusTooManyRequests {
		t.Errorf("a frame after the others finished = %d", res.status)
	}
}

// SR1-03 (T-18): the cap belongs to the credential, not to the label that the client chooses: rotating
// Stuarty-Client labels with one token does not lift it.
func TestRateLimitFrameCapIsPerTokenNotPerLabel(t *testing.T) {
	slow := slowFrames{started: make(chan struct{}, 8), release: make(chan struct{})}
	h := newHarnessWrapped(t, events.Options{}, func(s service.Service) service.Service {
		slow.Service = s
		return slow
	})

	const path = "/api/v1/projects/prj_0000000001/frame?at=0s"

	var wg sync.WaitGroup

	for _, label := range []string{"agent-a", "agent-b"} {
		wg.Go(func() { h.do(req{path: path, headers: map[string]string{"Stuarty-Client": label}}) })
		<-slow.started
	}

	res := h.do(req{path: path, headers: map[string]string{"Stuarty-Client": "agent-c"}})
	if res.status != http.StatusTooManyRequests || res.json(t)["code"] != service.CodeQueueFull {
		t.Errorf("a third label on the same token = %d %s, want 429 E_QUEUE_FULL", res.status, res.body)
	}

	close(slow.release)
	wg.Wait()
}
