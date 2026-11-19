package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
)

func (h *harness) pendingRequest(id, summary string) {
	h.t.Helper()

	now := time.Now().UnixMilli()
	row := approval.Row{
		ID: id, Actor: "local:claude-code", Kind: "publish.post", Summary: summary, PayloadRef: "draft:1",
		Estimate: core.Money{Micros: 2_500_000, Currency: "USD"}, State: approval.StatePending, CreatedMs: now,
		ExpiresMs: now + approval.ExpireAfter.Milliseconds(),
	}

	if err := h.store.Approvals().InsertRequest(context.Background(), &row); err != nil {
		h.t.Fatal(err)
	}
}

// SDD-015 section 7: the stream of the approval queue sends the pending requests when it opens, then
// the decisions as they are made, so the page updates without a reload. It carries no credential.
func TestApprovalsStreamSendsThePendingAndThenTheDecisions(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.pendingRequest("apr_0000000001", "Post the launch video")
	h.pendingRequest("apr_0000000002", "Post the second video")

	c := h.stream("/api/v1/approvals/events", nil)
	if c.status != http.StatusOK || !strings.HasPrefix(c.header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status %d, content type %q", c.status, c.header.Get("Content-Type"))
	}

	seen := map[string]string{}

	for range 2 {
		ev, ok := c.next(5 * time.Second)
		if !ok || ev.event != "approval.requested" {
			t.Fatalf("snapshot event = %+v, %v, want approval.requested", ev, ok)
		}

		var r approval.Request
		if err := json.Unmarshal([]byte(ev.data), &r); err != nil {
			t.Fatal(err)
		}

		seen[r.ID] = r.Summary
		if r.Estimate.Micros != 2_500_000 || r.State != approval.StatePending || r.Actor != "local:claude-code" {
			t.Errorf("snapshot request = %+v", r)
		}
	}

	if len(seen) != 2 {
		t.Fatalf("the snapshot carried %v, want both pending requests", seen)
	}

	// A person decides: the stream says so.
	person := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})

	if _, err := h.svc.ApprovalDecide(person, service.ApprovalDecideRequest{
		ID: "apr_0000000001", Decision: "reject", Reason: "no",
	}); err != nil {
		t.Fatal(err)
	}

	ev, ok := c.next(5 * time.Second)
	if !ok || ev.event != "approval.decided" {
		t.Fatalf("event after the decision = %+v, %v, want approval.decided", ev, ok)
	}

	var r approval.Request
	if err := json.Unmarshal([]byte(ev.data), &r); err != nil || r.ID != "apr_0000000001" ||
		r.State != approval.StateRejected || r.Reason != "no" {
		t.Fatalf("decided request = %+v, %v", r, err)
	}

	if strings.Contains(ev.raw, canary) {
		t.Error("a frame of the stream carries the local token")
	}

	if _, ok := c.next(300 * time.Millisecond); ok {
		t.Error("the stream sent an event nobody caused")
	}
}

// A stream with no credentials is refused before it becomes a stream.
func TestApprovalsStreamNeedsCredentials(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if res := h.do(req{path: "/api/v1/approvals/events", noAuth: true}); res.status != http.StatusUnauthorized {
		t.Errorf("the stream with no credentials = %d, want 401", res.status)
	}
}
