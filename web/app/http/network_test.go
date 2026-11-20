package httpapi_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/events"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
	"github.com/AlexGalhardo/stuarty/web/app/store"
	"github.com/AlexGalhardo/stuarty/web/bootstrap"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/auth/hash"
	"github.com/AlexGalhardo/vukem/ratelimit"
	"github.com/AlexGalhardo/vukem/vukemtest"
)

type networkHarness struct {
	srv  *httptest.Server
	log  *bytes.Buffer
	auth *auth.Network
	st   *store.Store
	clk  *clock
}

// newNetworkHarness serves the API in network mode over a plain or a TLS
// listener (the TLS one makes r.TLS non-nil, which is what a process that
// terminates TLS sees).
func newNetworkHarness(t *testing.T, res config.Resolved, useTLS bool) *networkHarness {
	t.Helper()

	// Tests log in many times in a few real seconds, which the clock they stub
	// does not slow down; the throttle tests ask for the defaults.
	loose := ratelimit.NewMemory(1_000_000, time.Minute)

	return newNetworkHarnessLimits(t, res, useTLS, &httpapi.LoginLimits{
		PerEmailAndAddress: loose, PerEmail: loose, PerAddress: loose,
	})
}

func newNetworkHarnessLimits(
	t *testing.T, res config.Resolved, useTLS bool, limits *httpapi.LoginLimits,
) *networkHarness {
	t.Helper()

	return newNetworkHarnessRates(t, res, useTLS, limits, httpapi.RateLimits{})
}

func newNetworkHarnessRates(
	t *testing.T, res config.Resolved, useTLS bool, limits *httpapi.LoginLimits, rates httpapi.RateLimits,
	extra ...vukem.Option,
) *networkHarness {
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

	netw, err := httpapi.NewNetwork(res, slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}

	clk := &clock{t: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)}
	now := clk.now

	// The fake hasher keeps a test from paying for argon2id on every login.
	netAuth, err := auth.NewNetwork(st.DB(), auth.NetworkOptions{
		Now: now, Hasher: hash.Fake(), PlainHTTP: res.LoopbackHTTP,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The streams are served as in a real server, so a test can check who may open one in network mode.
	hub := events.New(st, events.Options{})
	t.Cleanup(hub.Close)

	svc, err := service.New(service.Options{Store: st, Now: now, Keys: netAuth.Keys(), Publish: hub.Publish})
	if err != nil {
		t.Fatal(err)
	}

	deps := &httpapi.Deps{
		Events:        hub,
		Service:       svc,
		Registry:      command.Default(),
		Workspace:     dir,
		Network:       netw,
		Auth:          netAuth,
		RuntimeSecret: testRuntimeSecret,
		LoginLimits:   limits,
		RateLimits:    rates,
		Sessions:      auth.NewSessions(now),
		Launch:        auth.NewLaunch(now),
		Now:           now,
	}

	app := vukemtest.New(t, func(opts ...vukem.Option) (*vukem.App, error) {
		return bootstrap.NewWith(deps, append(opts, extra...)...)
	})

	h := &networkHarness{log: logs, auth: netAuth, clk: clk, st: st}
	if useTLS {
		h.srv = httptest.NewTLSServer(app.Handler())
	} else {
		h.srv = httptest.NewServer(app.Handler())
	}

	t.Cleanup(h.srv.Close)

	return h
}

// answer is what a test reads of a response.
type answer struct {
	status int
	www    string
}

func (h *networkHarness) do(t *testing.T, path, host string, headers map[string]string) answer {
	t.Helper()

	hr, err := http.NewRequest(http.MethodGet, h.srv.URL+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	hr.Host = host

	for k, v := range headers {
		hr.Header.Set(k, v)
	}

	res, err := h.srv.Client().Do(hr)
	if err != nil {
		t.Fatal(err)
	}

	_ = res.Body.Close()

	return answer{status: res.StatusCode, www: res.Header.Get("WWW-Authenticate")}
}

const testRuntimeSecret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func resolved() config.Resolved {
	return config.Resolved{Mode: config.ModeNetwork, AppURL: "https://video.example.com"}
}

// SDD-007 section 4.3: Host and Origin are exactly those of APP_URL, the
// effective scheme must be https, and with zero users the answer is 401 (never
// a 403 once the guard has passed).
func TestNetworkHostAndOriginRules(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)

	const api = "/api/v1/projects"

	for _, c := range []struct {
		name    string
		host    string
		headers map[string]string
		want    int
	}{
		{"exact host", "video.example.com", nil, http.StatusUnauthorized},
		{"host in capitals", "VIDEO.example.com", nil, http.StatusUnauthorized},
		{"other host", "evil.example.com", nil, http.StatusForbidden},
		{"loopback host is not accepted in network mode", "127.0.0.1", nil, http.StatusForbidden},
		{"host with the default port", "video.example.com:443", nil, http.StatusForbidden},
		{
			"same-origin", "video.example.com",
			map[string]string{"Origin": "https://video.example.com"},
			http.StatusUnauthorized,
		},
		{
			"other origin", "video.example.com",
			map[string]string{"Origin": "https://evil.example.com"},
			http.StatusForbidden,
		},
		{"null origin", "video.example.com", map[string]string{"Origin": "null"}, http.StatusForbidden},
		{
			"http origin", "video.example.com",
			map[string]string{"Origin": "http://video.example.com"},
			http.StatusForbidden,
		},
		{
			"cross-site fetch", "video.example.com",
			map[string]string{"Sec-Fetch-Site": "cross-site"},
			http.StatusForbidden,
		},
	} {
		res := h.do(t, api, c.host, c.headers)
		if res.status != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, res.status, c.want)
		}
	}
}

func TestNetworkSchemeRules(t *testing.T) {
	const api = "/api/v1/projects"

	t.Run("plain http without a trusted proxy is refused", func(t *testing.T) {
		h := newNetworkHarness(t, resolved(), false)

		for _, headers := range []map[string]string{nil, {"X-Forwarded-Proto": "http"}, {"X-Forwarded-Proto": "https"}} {
			if res := h.do(t, api, "video.example.com", headers); res.status != http.StatusForbidden {
				t.Errorf("headers %v: status = %d, want 403", headers, res.status)
			}
		}
	})

	t.Run("a trusted proxy that says https is accepted, one that says http is not", func(t *testing.T) {
		res := resolved()
		res.TrustForwardedProto = true
		res.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}

		h := newNetworkHarness(t, res, false)

		if r := h.do(
			t,
			api,
			"video.example.com",
			map[string]string{"X-Forwarded-Proto": "https"},
		); r.status != http.StatusUnauthorized {
			t.Errorf("https through a proxy: status = %d, want 401", r.status)
		}

		if r := h.do(
			t,
			api,
			"video.example.com",
			map[string]string{"X-Forwarded-Proto": "http"},
		); r.status != http.StatusForbidden {
			t.Errorf("http through a proxy: status = %d, want 403", r.status)
		}
	})

	t.Run("a proxy that is not trusted cannot vouch for https", func(t *testing.T) {
		res := resolved()
		res.TrustForwardedProto = true
		res.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

		h := newNetworkHarness(t, res, false)

		if r := h.do(
			t,
			api,
			"video.example.com",
			map[string]string{"X-Forwarded-Proto": "https"},
		); r.status != http.StatusForbidden {
			t.Errorf("status = %d, want 403", r.status)
		}
	})
}

// SDD-007 section 4.3 item 4: a server that binds a non-loopback address and takes plain HTTP
// warns that the published port decides who can read a credential; one bound to loopback does not.
func TestPlainHTTPOnANonLoopbackBindWarns(t *testing.T) {
	for _, c := range []struct {
		addr string
		warn bool
	}{
		{"0.0.0.0:7878", true},
		{"[::]:7878", true},
		{"127.0.0.1:7878", false},
	} {
		h := newNetworkHarness(t, config.Resolved{
			Mode: config.ModeNetwork, Addr: c.addr, AppURL: "http://localhost:7878", LoopbackHTTP: true,
		}, false)

		out := h.log.String()
		got := strings.Contains(out, "level=WARN") && strings.Contains(out, "publish the port on the loopback")
		if got != c.warn {
			t.Errorf("bind %s: warning = %v, want %v\n%s", c.addr, got, c.warn, out)
		}
	}
}

// SDD-007 section 4.3 item 4: plain HTTP only as a loopback name of the APP_URL port.
func TestContainerLoopbackOnly(t *testing.T) {
	h := newNetworkHarness(t, config.Resolved{
		Mode: config.ModeNetwork, AppURL: "http://localhost:7878", LoopbackHTTP: true,
	}, false)

	const api = "/api/v1/projects"

	for _, c := range []struct {
		name    string
		host    string
		headers map[string]string
		want    int
	}{
		{"localhost", "localhost:7878", nil, http.StatusUnauthorized},
		{"the host's address", "192.168.1.5:7878", nil, http.StatusForbidden},
		{"127.0.0.1 is a loopback alias", "127.0.0.1:7878", nil, http.StatusUnauthorized},
		{"[::1] is a loopback alias", "[::1]:7878", nil, http.StatusUnauthorized},
		{"LOCALHOST is case-insensitive", "LOCALHOST:7878", nil, http.StatusUnauthorized},
		{"loopback alias, another port", "127.0.0.1:7879", nil, http.StatusForbidden},
		{"foo.localhost", "foo.localhost:7878", nil, http.StatusForbidden},
		{
			"origin of a loopback alias", "127.0.0.1:7878",
			map[string]string{"Origin": "http://127.0.0.1:7878"},
			http.StatusUnauthorized,
		},
		{"another port", "localhost:7879", nil, http.StatusForbidden},
		{
			"origin of the host's address", "localhost:7878",
			map[string]string{"Origin": "http://192.168.1.5:7878"},
			http.StatusForbidden,
		},
		{
			"own origin", "localhost:7878",
			map[string]string{"Origin": "http://localhost:7878"},
			http.StatusUnauthorized,
		},
	} {
		if res := h.do(t, api, c.host, c.headers); res.status != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, res.status, c.want)
		}
	}

	if !strings.Contains(h.log.String(), "reachable only as http://localhost:7878") ||
		!strings.Contains(h.log.String(), "TLS") {
		t.Errorf("start-up log lacks the container line:\n%s", h.log)
	}
}

// A command on the same machine asks the owner for a proof of the per-run secret before it sends an
// API key (SDD-007 section 3.8): loopback peers only, no relay header, no Host rule, no credential.
func TestNetworkOwnerProvesTheRunSecret(t *testing.T) {
	h := newNetworkHarness(t, resolved(), false)

	const nonce = "00112233445566778899aabbccddeeff"

	get := func(host string, headers map[string]string) (int, string) {
		t.Helper()

		req, err := http.NewRequest(http.MethodGet, h.srv.URL+"/api/v1/handshake?nonce="+nonce, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}

		req.Host = host

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		res, err := h.srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}

		defer res.Body.Close()

		body, _ := io.ReadAll(res.Body)

		return res.StatusCode, string(body)
	}

	status, body := get("127.0.0.1:7878", nil)
	if status != http.StatusOK || !strings.Contains(body, auth.Proof(testRuntimeSecret, nonce)) {
		t.Fatalf("handshake = %d %s", status, body)
	}

	if status, _ := get(
		"127.0.0.1:7878",
		map[string]string{"X-Forwarded-For": "1.2.3.4"},
	); status != http.StatusForbidden {
		t.Errorf("handshake through a relay = %d, want 403", status)
	}
}

func TestNetworkWithNoUsersAnswers401AndLogsHowToCreateOne(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)

	for _, path := range []string{"/api/v1/projects", "/openapi.json", "/auth/launch"} {
		res := h.do(t, path, "video.example.com", map[string]string{"Authorization": "Bearer vvk_x.y"})
		if res.status != http.StatusUnauthorized || res.www != "Bearer" {
			t.Errorf("%s: status = %d, www-authenticate = %q", path, res.status, res.www)
		}
	}

	if !strings.Contains(h.log.String(), "user:create") ||
		!strings.Contains(h.log.String(), "STUARTY_ADMIN_EMAIL") {
		t.Errorf("log does not say how to create a user:\n%s", h.log)
	}

	// The health probe needs no credential and, from a loopback peer with no
	// relay header, no Host rule (SDD-007 section 3.7 item 6).
	if res := h.do(t, "/up", "127.0.0.1:7878", nil); res.status != http.StatusOK {
		t.Errorf("/up: status = %d", res.status)
	}

	if res := h.do(
		t,
		"/up",
		"127.0.0.1:7878",
		map[string]string{"X-Forwarded-For": "1.2.3.4"},
	); res.status != http.StatusForbidden {
		t.Errorf("/up through a relay: status = %d, want 403", res.status)
	}
}
