package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/ratelimit"

	"github.com/AlexGalhardo/vukem-video/service"
	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
)

func pagesNetworkHarness(t *testing.T) *networkHarness {
	t.Helper()

	loose := ratelimit.NewMemory(1_000_000, time.Minute)
	built := fstest.MapFS{"build/.vite/manifest.json": {Data: []byte(manifest)}}

	return newNetworkHarnessRates(t, resolved(), true, &httpapi.LoginLimits{
		PerEmailAndAddress: loose, PerEmail: loose, PerAddress: loose,
	}, httpapi.RateLimits{}, vukem.WithVite(built))
}

var htmlAccept = map[string]string{"Accept": "text/html,application/xhtml+xml"}

// SDD-012 section 2.1: in network mode a browser with no session that opens a page is
// sent to /login with the page to come back to; the login page itself needs no
// session; the API still answers 401.
func TestNetworkPagesLeadToLogin(t *testing.T) {
	h := pagesNetworkHarness(t)

	res := h.send(t, http.MethodGet, "/projects?sort=name", "", "", htmlAccept)
	if res.status != http.StatusSeeOther || res.header.Get("Location") != "/login?next=%2Fprojects%3Fsort%3Dname" {
		t.Errorf("/projects with no session = %d to %q", res.status, res.header.Get("Location"))
	}

	res = h.send(t, http.MethodGet, "/settings/keys", "", "", htmlAccept)
	if res.status != http.StatusSeeOther {
		t.Errorf("/settings/keys with no session = %d", res.status)
	}

	res = h.send(t, http.MethodGet, "/login", "", "", htmlAccept)
	if res.status != http.StatusOK || !strings.Contains(unescaped(res.body), "Auth/Login") {
		t.Errorf("/login = %d %s", res.status, res.body)
	}

	// A call of the API is never redirected, and a page asked for as JSON is 401.
	if res = h.send(t, http.MethodGet, "/api/v1/projects", "", "", htmlAccept); res.status != http.StatusUnauthorized {
		t.Errorf("the API with no session = %d", res.status)
	}

	if res = h.send(t, http.MethodGet, "/projects", "", "", nil); res.status != http.StatusUnauthorized {
		t.Errorf("/projects with no session, not asking for HTML = %d", res.status)
	}

	// Only a GET of the login page is open.
	if res = h.send(t, http.MethodPost, "/login", "{}", "", nil); res.status == http.StatusOK {
		t.Errorf("POST /login = %d", res.status)
	}
}

// The login page goes back to a path of this server and to nothing else.
func TestNetworkLoginPageNextIsASafePath(t *testing.T) {
	h := pagesNetworkHarness(t)

	for next, want := range map[string]string{
		"%2Fprojects%3Fsort%3Dname":     "/projects?sort=name",
		"%2F%2Fevil.example":            "/projects",
		"https%3A%2F%2Fevil.example":    "/projects",
		"%2F%5Cevil.example":            "/projects",
		"%2Flogin%3Fnext%3D%2Fprojects": "/projects",
		"":                              "/projects",
	} {
		res := h.send(t, http.MethodGet, "/login?next="+next, "", "", htmlAccept)
		body := unescaped(res.body)

		if res.status != http.StatusOK || !strings.Contains(body, `"next":"`+want+`"`) {
			t.Errorf("next=%q: %d, the page does not carry next=%q: %s", next, res.status, want, res.body)
		}
	}
}

// SDD-012 section 2.1: /settings/keys is a page of a user with a session.
func TestNetworkKeysPageNeedsASession(t *testing.T) {
	h := pagesNetworkHarness(t)
	h.user(t, "ana@example.com", service.RoleAdmin)

	res := h.send(t, http.MethodGet, "/settings/keys", "", h.session(t, "ana@example.com"), htmlAccept)
	if res.status != http.StatusOK || !strings.Contains(unescaped(res.body), "Settings/Keys") {
		t.Errorf("/settings/keys = %d %s", res.status, res.body)
	}
}

// SDD-012 section 2.1: there are no users and no keys in local mode, so neither page exists.
func TestLocalModeHasNoLoginOrKeysPage(t *testing.T) {
	h := pagesHarness(t)
	ck := cookie(h)

	for _, path := range []string{"/login", "/settings/keys"} {
		res := h.do(req{path: path, noAuth: true, headers: map[string]string{"Cookie": ck}})
		if res.status != http.StatusNotFound {
			t.Errorf("%s in local mode = %d, want 404", path, res.status)
		}
	}
}

// unescaped is a page as text, with the slashes of its JSON data not escaped.
func unescaped(body []byte) string { return strings.ReplaceAll(string(body), `\/`, "/") }
