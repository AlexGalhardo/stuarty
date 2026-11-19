package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/ratelimit"
)

// inertiaProps reads the props that the first visit of an Inertia page writes into the HTML.
func inertiaProps(t *testing.T, res netResp) map[string]any {
	t.Helper()

	const open, end = `<script data-page="app" type="application/json">`, "</script>"

	_, rest, ok := strings.Cut(string(res.body), open)
	if !ok {
		t.Fatalf("no page in the answer (%d): %s", res.status, res.body)
	}

	raw, _, _ := strings.Cut(rest, end)

	var page struct {
		Props map[string]any `json:"props"`
	}

	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		t.Fatalf("not an Inertia page (%d): %v: %s", res.status, err, res.body)
	}

	return page.Props
}

// VV-011: in network mode every page shares the signed-in user, which draws the
// keys link and the log out button, and the name of the app.
func TestNetworkPagesShareTheUserAndTheAppName(t *testing.T) {
	built := fstest.MapFS{"build/.vite/manifest.json": {Data: []byte(manifest)}}
	loose := ratelimit.NewMemory(1_000_000, time.Minute)
	h := newNetworkHarnessRates(t, resolved(), true, &httpapi.LoginLimits{
		PerEmailAndAddress: loose, PerEmail: loose, PerAddress: loose,
	}, httpapi.RateLimits{}, vukem.WithVite(built))
	h.user(t, "ana@example.com", service.RoleAdmin)
	session := h.session(t, "ana@example.com")

	res := h.send(t, http.MethodGet, "/projects", "", session, map[string]string{"Accept": "text/html"})
	if res.status != http.StatusOK {
		t.Fatalf("/projects = %d %s", res.status, res.body)
	}

	props := inertiaProps(t, res)
	if props["appName"] != "Stuarty" {
		t.Errorf("appName = %v", props["appName"])
	}

	user, _ := props["auth"].(map[string]any)["user"].(map[string]any)
	if user["email"] != "ana@example.com" || user["role"] != service.RoleAdmin {
		t.Errorf("auth.user = %v", props["auth"])
	}

	login := h.send(t, http.MethodGet, "/login", "", "", map[string]string{"Accept": "text/html"})

	props = inertiaProps(t, login)
	if auth, ok := props["auth"].(map[string]any); !ok || auth["user"] != nil {
		t.Errorf("the login page auth = %v, want auth.user null", props["auth"])
	}
}

// In local mode there are no users: the layout must not draw the account links.
func TestLocalPagesShareTheAppNameAndNoUser(t *testing.T) {
	h := pagesHarness(t)

	res := h.do(req{
		path: "/projects", noAuth: true, headers: map[string]string{"Cookie": cookie(h), "Accept": "text/html"},
	})
	if res.status != http.StatusOK {
		t.Fatalf("/projects = %d %s", res.status, res.body)
	}

	props := inertiaProps(t, netResp{status: res.status, body: res.body})
	if props["appName"] != "Stuarty" || props["auth"] != nil {
		t.Errorf("local props: appName %v, auth %v", props["appName"], props["auth"])
	}
}
