package httpapi_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/auth/hash"
	"github.com/AlexGalhardo/vukem/vukemtest"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/config"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
	"github.com/AlexGalhardo/vukem-video/web/bootstrap"
)

type networkHarness struct {
	srv  *httptest.Server
	log  *bytes.Buffer
	auth *auth.Network
	clk  *clock
}

// newNetworkHarness serves the API in network mode over a plain or a TLS
// listener (the TLS one makes r.TLS non-nil, which is what a process that
// terminates TLS sees).
func newNetworkHarness(t *testing.T, res config.Resolved, useTLS bool) *networkHarness {
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

	svc, err := service.New(service.Options{Store: st})
	if err != nil {
		t.Fatal(err)
	}

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

	deps := &httpapi.Deps{
		Service: svc, Registry: command.Default(), Workspace: dir, Network: netw, Auth: netAuth,
		Sessions: auth.NewSessions(now), Launch: auth.NewLaunch(now), Now: now,
	}

	app := vukemtest.New(t, func(opts ...vukem.Option) (*vukem.App, error) {
		return bootstrap.NewWith(deps, opts...)
	})

	h := &networkHarness{log: logs, auth: netAuth, clk: clk}
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

// SDD-007 section 4.3 item 4: plain HTTP only as the loopback name of APP_URL.
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
		{"127.0.0.1 is another name", "127.0.0.1:7878", nil, http.StatusForbidden},
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

func TestNetworkWithNoUsersAnswers401AndLogsHowToCreateOne(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)

	for _, path := range []string{"/api/v1/projects", "/openapi.json", "/api/v1/handshake", "/auth/launch"} {
		res := h.do(t, path, "video.example.com", map[string]string{"Authorization": "Bearer vvk_x.y"})
		if res.status != http.StatusUnauthorized || res.www != "Bearer" {
			t.Errorf("%s: status = %d, www-authenticate = %q", path, res.status, res.www)
		}
	}

	if !strings.Contains(h.log.String(), "user:create") ||
		!strings.Contains(h.log.String(), "VUKEM_VIDEO_ADMIN_EMAIL") {
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
