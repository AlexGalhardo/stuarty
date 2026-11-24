package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
)

// SDD-015 section 4.1: GET /inbox lists what waits for the caller, and only reads: the route of a decision is the one of
// its source.
func TestInboxListOverHTTPShowsTheApprovalsThatWait(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.pendingRequest("apr_0000000001", "Post the launch video")

	res := h.do(req{path: "/api/v1/inbox"})
	if res.status != http.StatusOK {
		t.Fatalf("GET /inbox = %d: %s", res.status, res.body)
	}

	var out service.InboxListResult
	if err := json.Unmarshal(res.body, &out); err != nil {
		t.Fatal(err)
	}

	if out.Waiting != 1 || len(out.Rows) != 1 || out.Rows[0].ID != "approval:apr_0000000001" ||
		out.Rows[0].Source != "approval" || out.Rows[0].Cost == nil || out.Rows[0].Cost.Micros != 2_500_000 ||
		out.Rows[0].Summary != "Post the launch video" || out.UntrustedNotice == "" {
		t.Errorf("result = %+v", out)
	}

	if strings.Contains(string(res.body), canary) {
		t.Error("the answer carries the local token")
	}

	// A read: no method but GET answers on the route.
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		if got := h.do(req{method: m, path: "/api/v1/inbox", body: "{}"}); got.status < 400 {
			t.Errorf("%s /inbox = %d, want a refusal", m, got.status)
		}
	}

	if got := h.do(req{path: "/api/v1/inbox", noAuth: true}); got.status != http.StatusUnauthorized {
		t.Errorf("GET /inbox with no credentials = %d, want 401", got.status)
	}
}

// The stream sends the list when it opens and again, whole, when a request is settled; it carries no credential.
func TestInboxStreamSendsTheListAndThenTheListAgain(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.pendingRequest("apr_0000000001", "Post the launch video")

	c := h.stream("/api/v1/inbox/events", nil)
	if c.status != http.StatusOK || !strings.HasPrefix(c.header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status %d, content type %q", c.status, c.header.Get("Content-Type"))
	}

	first, ok := c.next(5 * time.Second)
	if !ok || first.event != "inbox.snapshot" {
		t.Fatalf("first frame = %+v, %v, want inbox.snapshot", first, ok)
	}

	var before service.InboxListResult
	if err := json.Unmarshal([]byte(first.data), &before); err != nil || before.Waiting != 1 {
		t.Fatalf("first snapshot = %+v, %v", before, err)
	}

	person := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})

	if _, err := h.svc.ApprovalDecide(person, service.ApprovalDecideRequest{
		ID: "apr_0000000001", Decision: "reject", Reason: "no",
	}); err != nil {
		t.Fatal(err)
	}

	next, ok := c.next(5 * time.Second)
	if !ok || next.event != "inbox.snapshot" {
		t.Fatalf("frame after the decision = %+v, %v, want inbox.snapshot", next, ok)
	}

	var after service.InboxListResult
	if err := json.Unmarshal([]byte(next.data), &after); err != nil || after.Waiting != 0 || len(after.Rows) != 1 ||
		after.Rows[0].State != "rejected" {
		t.Fatalf("second snapshot = %+v, %v", after, err)
	}

	if strings.Contains(first.raw+next.raw, canary) {
		t.Error("a frame of the stream carries the local token")
	}

	if _, ok := c.next(300 * time.Millisecond); ok {
		t.Error("the stream sent a frame nobody caused")
	}
}

func TestInboxStreamNeedsCredentials(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if res := h.do(req{path: "/api/v1/inbox/events", noAuth: true}); res.status != http.StatusUnauthorized {
		t.Errorf("the stream with no credentials = %d, want 401", res.status)
	}
}
