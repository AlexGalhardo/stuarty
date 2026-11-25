package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// T-167: the route the desktop shell asks for the indicator is local, for the admin token only, and reads the
// host's count of helpers, never a recording row.
func TestSecurity_T167_TheCaptureStatusRouteIsLocalAndForTheAdminOnly(t *testing.T) {
	h := newHarness(t)

	res := h.get("/api/v1/system/capture")
	if res.status != http.StatusOK {
		t.Fatalf("status = %d %s", res.status, res.body)
	}

	if got := res.json(t); got["active"] != false || got["helpers"] != float64(0) || got["since_ms"] != float64(0) {
		t.Errorf("an idle server says %v", got)
	}

	if res := h.do(req{path: "/api/v1/system/capture", noAuth: true}); res.status != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", res.status)
	}

	// it only reads: a post is not a route
	if res := h.post("/api/v1/system/capture", `{}`); res.status == http.StatusOK {
		t.Errorf("a POST was accepted: %s", res.body)
	}

	// in network mode no caller finds it, an admin included
	n := newNetworkHarness(t, resolved(), true)
	n.user(t, "ana@example.com", service.RoleAdmin)
	ana := n.session(t, "ana@example.com")
	key := n.makeKey(t, ana, `{"name":"ops","scopes":["read","admin"],"expires_in_days":30}`)

	for name, res := range map[string]netResp{
		"session": n.send(t, http.MethodGet, "/api/v1/system/capture", "", ana, nil),
		"key":     n.asKey(t, key.Secret, http.MethodGet, "/api/v1/system/capture", ""),
	} {
		if res.status != http.StatusNotFound || res.code() != "E_NOT_FOUND" {
			t.Errorf("%s = %d %s, want 404 E_NOT_FOUND", name, res.status, res.body)
		}
	}
}
