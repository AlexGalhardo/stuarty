package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
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

// inboxStreamAs opens GET /inbox/events in network mode with the session cookie of a member.
func inboxStreamAs(t *testing.T, h *networkHarness, cookie string) *sseClient {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	hr, err := http.NewRequestWithContext(ctx, http.MethodGet, h.srv.URL+"/api/v1/inbox/events", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	hr.Host = netHost
	hr.Header.Set("Origin", netOrigin)
	hr.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})

	resp, err := h.srv.Client().Do(hr) //nolint:bodyclose // the reader goroutine closes it
	if err != nil {
		t.Fatal(err)
	}

	return readSSE(t, resp, cancel)
}

func (h *networkHarness) pendingFor(t *testing.T, id, owner string) {
	t.Helper()

	now := time.Now().UnixMilli()
	row := approval.Row{
		ID: id, Actor: "local:claude-code", OwnerUserID: owner, Kind: "publish.post", Summary: "secret plan of " + owner,
		PayloadRef: "draft:1", State: approval.StatePending, CreatedMs: now,
		ExpiresMs: now + approval.ExpireAfter.Milliseconds(),
	}

	if err := h.st.Approvals().InsertRequest(context.Background(), &row); err != nil {
		t.Fatal(err)
	}
}

// SDD-015 section 4.1, SDD-007 T-06: in network mode each member's inbox stream carries that member's rows and never
// another's, when it opens and on every change, including a change made by the other member.
func TestSecurity_T06_InboxStreamCarriesOnlyTheMembersOwnRows(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	ana := h.user(t, "ana@example.com", service.RoleMember)
	bob := h.user(t, "bob@example.com", service.RoleMember)

	h.pendingFor(t, "apr_000000000a", ana.ID)
	h.pendingFor(t, "apr_000000000b", bob.ID)

	anaStream := inboxStreamAs(t, h, h.session(t, "ana@example.com"))
	if anaStream.status != http.StatusOK {
		t.Fatalf("Ana's stream = %d %s", anaStream.status, anaStream.body)
	}

	bobStream := inboxStreamAs(t, h, h.session(t, "bob@example.com"))
	if bobStream.status != http.StatusOK {
		t.Fatalf("Bob's stream = %d %s", bobStream.status, bobStream.body)
	}

	frames := map[string][]sseEvent{}
	read := func(name string, c *sseClient) sseEvent {
		t.Helper()

		ev, ok := c.next(5 * time.Second)
		if !ok || ev.event != "inbox.snapshot" {
			t.Fatalf("%s: frame = %+v, %v, want inbox.snapshot", name, ev, ok)
		}

		frames[name] = append(frames[name], ev)

		return ev
	}

	only := func(name string, ev sseEvent, want string) {
		t.Helper()

		var res service.InboxListResult
		if err := json.Unmarshal([]byte(ev.data), &res); err != nil {
			t.Fatal(err)
		}

		if len(res.Rows) != 1 || res.Rows[0].ID != "approval:"+want {
			t.Errorf("%s's frame = %s, want only approval:%s", name, ev.data, want)
		}
	}

	only("ana", read("ana", anaStream), "apr_000000000a")
	only("bob", read("bob", bobStream), "apr_000000000b")

	// Bob decides his own request: Bob's stream follows, and Ana's list did not change, so she is sent nothing.
	bobCtx := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:" + bob.ID, Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: bob.ID, Browser: true,
	})
	if _, err := h.svc.ApprovalDecide(bobCtx, service.ApprovalDecideRequest{
		ID: "apr_000000000b", Decision: "reject", Reason: "no",
	}); err != nil {
		t.Fatal(err)
	}

	if ev := read("bob", bobStream); !strings.Contains(ev.data, `"state":"rejected"`) {
		t.Errorf("Bob's frame after his decision = %s", ev.data)
	}

	if ev, ok := anaStream.next(500 * time.Millisecond); ok {
		t.Errorf("Ana was sent a frame for Bob's decision: %+v", ev)
	}

	for _, ev := range frames["ana"] {
		if strings.Contains(ev.raw, "apr_000000000b") || strings.Contains(ev.raw, bob.ID) {
			t.Errorf("Ana's stream carries a request of Bob: %s", ev.raw)
		}
	}

	for _, ev := range frames["bob"] {
		if strings.Contains(ev.raw, "apr_000000000a") || strings.Contains(ev.raw, ana.ID) {
			t.Errorf("Bob's stream carries a request of Ana: %s", ev.raw)
		}
	}
}

func TestInboxStreamNeedsCredentials(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if res := h.do(req{path: "/api/v1/inbox/events", noAuth: true}); res.status != http.StatusUnauthorized {
		t.Errorf("the stream with no credentials = %d, want 401", res.status)
	}
}
