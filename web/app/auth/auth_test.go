package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.t = c.t.Add(d)
}

func newClock() *fakeClock { return &fakeClock{t: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)} }

func redeemed(l *Launch, code string) bool {
	_, ok := l.Redeem(code)

	return ok
}

func live(s *Sessions, id string) bool {
	valid, _ := s.Lookup(id)

	return valid
}

func TestPrivilegeFollowsTheLauncherSecret(t *testing.T) {
	t.Parallel()

	l, s := NewLaunch(nil), NewSessions(nil)

	for _, privileged := range []bool{false, true} {
		code, _ := l.Issue(privileged)

		got, ok := l.Redeem(code)
		if !ok || got != privileged {
			t.Errorf("Redeem(issued privileged=%v) = %v, %v", privileged, got, ok)
		}

		id, _ := s.Create(got)
		if valid, p := s.Lookup(id); !valid || p != privileged {
			t.Errorf("Lookup(created privileged=%v) = %v, %v", privileged, valid, p)
		}
	}

	if IsLauncher("", "") || IsLauncher("", "x") || IsLauncher("secret", "") || IsLauncher("secret", "secreT") ||
		!IsLauncher("secret", "secret") {
		t.Error("IsLauncher: an empty secret must match nothing and the rest must match exactly")
	}
}

func TestLaunchCodeWorksOnceAndExpires(t *testing.T) {
	t.Parallel()

	clock := newClock()
	l := NewLaunch(clock.now)

	code, err := l.Issue(false)
	if err != nil || len(code) != 64 {
		t.Fatalf("Issue = %q, %v; want 64 hex characters", code, err)
	}

	if !redeemed(l, code) {
		t.Error("a fresh code was refused")
	}

	if redeemed(l, code) {
		t.Error("a code worked twice")
	}

	late, _ := l.Issue(false)

	clock.advance(121 * time.Second)

	if redeemed(l, late) {
		t.Error("a code worked after 120 seconds")
	}

	edge, _ := l.Issue(false)

	clock.advance(120 * time.Second)

	if !redeemed(l, edge) {
		t.Error("a code was refused at exactly 120 seconds")
	}
}

func TestLaunchKeepsEightCodesAndLocksAfterFiveFailures(t *testing.T) {
	t.Parallel()

	clock := newClock()
	l := NewLaunch(clock.now)

	first, _ := l.Issue(false)
	for range 8 {
		_, _ = l.Issue(false)
	}

	if redeemed(l, first) {
		t.Error("the ninth code did not push the oldest out")
	}

	if l.Locked() {
		t.Fatal("locked after one failure")
	}

	for range 4 {
		redeemed(l, strings.Repeat("0", 64))
	}

	if !l.Locked() {
		t.Error("not locked after five failures within 60 seconds")
	}

	clock.advance(61 * time.Second)

	if l.Locked() {
		t.Error("still locked 61 seconds later")
	}
}

func TestSessionLifetimes(t *testing.T) {
	t.Parallel()

	clock := newClock()
	s := NewSessions(clock.now)

	id, err := s.Create(false)
	if err != nil || !live(s, id) || live(s, "nope") {
		t.Fatalf("Create = %q, %v", id, err)
	}

	clock.advance(23 * time.Hour)

	if !live(s, id) {
		t.Error("the session ended before 24 hours without use")
	}

	clock.advance(23 * time.Hour) // used 23 hours ago, 46 hours old: the idle time renews

	if !live(s, id) {
		t.Error("use did not renew the idle time")
	}

	for range 6 {
		clock.advance(23 * time.Hour)
		live(s, id)
	}

	if live(s, id) {
		t.Error("the session outlived 7 days")
	}

	other, _ := s.Create(false)

	s.Clear()

	if live(s, other) {
		t.Error("Clear left a session")
	}
}

func TestActorLabel(t *testing.T) {
	t.Parallel()

	for label, want := range map[string]string{"": "local:api", "curl-demo": "local:curl-demo", "a1": "local:a1"} {
		if got, err := Actor(label); err != nil || got != want {
			t.Errorf("Actor(%q) = %q, %v; want %q", label, got, err, want)
		}
	}

	for _, bad := range []string{"ui", "UPPER", "a b", "a_b", strings.Repeat("a", 33), "local:x"} {
		if _, err := Actor(bad); err == nil {
			t.Errorf("Actor(%q) was accepted", bad)
		}
	}
}

func TestBearerParsing(t *testing.T) {
	t.Parallel()

	if tok, ok := Bearer("Bearer vvl_x"); !ok || tok != "vvl_x" {
		t.Errorf("Bearer = %q, %v", tok, ok)
	}

	for _, h := range []string{"", "Basic abc", "Bearer", "Bearer  "} {
		if _, ok := Bearer(h); ok {
			t.Errorf("Bearer(%q) accepted", h)
		}
	}
}

// proofServer answers the handshake as a server that knows the token would.
func proofServer(token string, seen *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.Header.Get("Authorization"))

		if r.URL.Path != HandshakePath {
			w.WriteHeader(http.StatusOK)
			return
		}

		_, _ = w.Write([]byte(`{"proof":"` + Proof(token, r.URL.Query().Get("nonce")) + `"}`))
	}))
}

func newGet(t *testing.T, url string) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	return req
}

func TestVerifyAcceptsTheRealServer(t *testing.T) {
	t.Parallel()

	var seen []string

	srv := proofServer("vvl_secret", &seen)
	defer srv.Close()

	if err := Verify(context.Background(), srv.Client(), srv.URL, "vvl_secret"); err != nil {
		t.Errorf("Verify against a server that knows the token: %v", err)
	}

	if err := Verify(context.Background(), srv.Client(), srv.URL, "vvl_other"); err == nil {
		t.Error("Verify accepted a proof made with another token")
	}
}

// An impostor on the port cannot make the proof, so the client never sends the
// token to it (SDD-007 section 3.8).
func TestSecurity_T30_ImpostorServerGetsNoToken(t *testing.T) {
	t.Parallel()

	for name, handler := range map[string]http.HandlerFunc{
		"garbage proof": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"proof":"00"}`)) },
		"not json":      func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`hello`)) },
		"error status":  func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var (
				mu   sync.Mutex
				auth []string
			)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				auth = append(auth, r.Header.Get("Authorization")+"|"+r.URL.RawQuery)
				mu.Unlock()
				handler(w, r)
			}))
			defer srv.Close()

			client := &Client{Base: srv.URL, Token: "vvl_secret", Next: srv.Client()}

			if res, err := client.Do(newGet(t, srv.URL+"/api/v1/projects")); err == nil {
				_ = res.Body.Close()
				t.Fatal("the request went through to an impostor")
			}

			mu.Lock()
			defer mu.Unlock()

			for _, a := range auth {
				if strings.Contains(a, "vvl_secret") || strings.Contains(a, "Bearer") {
					t.Errorf("the impostor saw the token: %q", a)
				}
			}
		})
	}
}

func TestTransportSendsTheTokenAfterAValidProof(t *testing.T) {
	t.Parallel()

	var seen []string

	srv := proofServer("vvl_secret", &seen)
	defer srv.Close()

	client := &Client{Base: srv.URL, Token: "vvl_secret", Next: srv.Client()}

	res, err := client.Do(newGet(t, srv.URL+"/api/v1/projects"))
	if err != nil {
		t.Fatal(err)
	}

	_ = res.Body.Close()

	if got := seen[len(seen)-1]; got != "Bearer vvl_secret" {
		t.Errorf("the real request carried %q", got)
	}

	if seen[0] != "" {
		t.Errorf("the handshake carried %q: it must not hold a credential", seen[0])
	}
}
