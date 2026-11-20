package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

func TestSystemSchedulerStatusPauseResume(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	sched, ok := service.SchedulerOf(h.svc)
	if !ok {
		t.Fatal("the service has no scheduler")
	}

	status := h.post("/api/v1/system/scheduler", `{"action":"status"}`)
	if status.status != http.StatusOK {
		t.Fatalf("status = %d %s", status.status, status.body)
	}

	if got := status.json(t); got["paused"] != false || got["scheduled"] != float64(0) {
		t.Errorf("status = %v", got)
	}

	// The bearer token alone reads, and never pauses.
	if res := h.post("/api/v1/system/scheduler", `{"action":"pause"}`); res.status != http.StatusForbidden {
		t.Errorf("pause with the token alone = %d %s, want 403", res.status, res.body)
	}

	if sched.Paused() {
		t.Fatal("paused by a request that was refused")
	}

	paused := h.do(req{
		method: http.MethodPost, path: "/api/v1/system/scheduler", body: `{"action":"pause"}`,
		headers: launcherHeaders(launcherSecret),
	})
	if paused.status != http.StatusOK || paused.json(t)["paused"] != true || !sched.Paused() {
		t.Fatalf("pause = %d %s", paused.status, paused.body)
	}

	resumed := h.do(req{
		method: http.MethodPost, path: "/api/v1/system/scheduler", body: `{"action":"resume"}`,
		headers: launcherHeaders(launcherSecret),
	})
	if resumed.status != http.StatusOK || resumed.json(t)["paused"] != false || sched.Paused() {
		t.Fatalf("resume = %d %s", resumed.status, resumed.body)
	}

	if res := h.post("/api/v1/system/scheduler", `{"action":"explode"}`); res.status == http.StatusOK {
		t.Errorf("an unknown action was accepted: %s", res.body)
	}

	if res := h.do(req{
		method: http.MethodPost, path: "/api/v1/system/scheduler", body: `{"action":"status"}`, noAuth: true,
	}); res.status != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", res.status)
	}
}
