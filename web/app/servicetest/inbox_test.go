package servicetest

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/core/inbox"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// The unified read of what waits for a person (SDD-015 section 4.1, epic 77).

// ask starts a session of a fake-agent mode in a project, makes it ask for a permission and returns the
// event that carries it.
func (w *chatWorld) ask(ctx context.Context, t *testing.T, project, profile string) (*service.ChatSession, chat.Event) {
	t.Helper()

	w.profile(t, profile, profile)

	s, err := w.svc.ChatSessionStart(ctx, service.ChatSessionStartRequest{ProjectID: project, Profile: profile})
	if err != nil {
		t.Fatalf("ChatSessionStart = %v", err)
	}

	if _, err := w.svc.ChatSend(ctx, service.ChatSendRequest{SessionID: s.ID, Text: "go"}); err != nil {
		t.Fatalf("ChatSend = %v", err)
	}

	return s, w.wait(t, s.ID, isKind(chat.KindPermissionAsked))
}

func (w *chatWorld) inbox(ctx context.Context, t *testing.T) *service.InboxListResult {
	t.Helper()

	res, err := w.svc.InboxList(ctx, service.InboxListRequest{})
	if err != nil {
		t.Fatalf("InboxList = %v", err)
	}

	return res
}

func rowOf(res *service.InboxListResult, id string) *inbox.Row {
	for i := range res.Rows {
		if res.Rows[i].ID == id {
			return &res.Rows[i]
		}
	}

	return nil
}

// Acceptance criterion 1 of epic 77: an open chat permission and a pending approval are both listed, each with its
// source, and the summary text is flagged as untrusted.
func TestInboxListsAChatPermissionAndAnApprovalWithTheirSource(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")

	s, ask := w.ask(person(), t, project, "permission-stuarty")
	req := w.addRequestFor("", project)

	res := w.inbox(w.as(ui), t)

	chatRow := rowOf(res, "chat:"+ask.Permission.RequestID)
	if chatRow == nil || chatRow.Source != inbox.SourceChat || chatRow.State != inbox.StateWaiting ||
		chatRow.SessionID != s.ID || chatRow.ProjectID != project || chatRow.Agent != "permission-stuarty" {
		t.Fatalf("the chat permission is not listed as a waiting chat row: %+v", res.Rows)
	}

	apr := rowOf(res, "approval:"+req.ID)
	if apr == nil {
		t.Fatalf("the approval is not listed: %+v", res.Rows)
	}

	listed := apr.Source == inbox.SourceApproval && apr.State == inbox.StateWaiting && apr.SourceID == req.ID
	described := apr.Tool == "publish.post" && apr.Summary == "Post the launch video" && !apr.Undoable

	if !listed || !described {
		t.Fatalf("the approval is not listed as a waiting approval row: %+v", res.Rows)
	}

	if res.Waiting != 2 || len(res.Rows) != 2 || res.UntrustedNotice == "" {
		t.Errorf("waiting = %d, rows = %d, notice %q", res.Waiting, len(res.Rows), res.UntrustedNotice)
	}

	// The tool is Stuarty's: what the call acts on and where come from the allowlist, and the only operation
	// it names is a command of the editor registry, so the call is one undo step.
	if chatRow.Item != "ast_3c5v8n2w6d" || chatRow.Destination != "prj_a4k9m2x7bq" ||
		chatRow.ActionKey != "ui.agent_activity.tool.edit" || !chatRow.Undoable {
		t.Errorf("chat row = %+v", chatRow)
	}
}

// Acceptance criterion 2 of epic 77: the read decides nothing. A row is answered where its source is answered, and the
// read then shows the new state.
func TestInboxRowsAreDecidedWhereTheirSourceDecides(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")

	s, ask := w.ask(person(), t, project, "permission")
	req := w.addRequestFor("", project)

	if _, err := w.svc.ChatAnswerPermission(person(), service.ChatAnswerRequest{
		SessionID: s.ID, RequestID: ask.Permission.RequestID, Answer: "allow",
	}); err != nil {
		t.Fatalf("ChatAnswerPermission = %v", err)
	}

	if _, err := w.svc.ApprovalDecide(w.asPerson(), service.ApprovalDecideRequest{
		ID: req.ID, Decision: "reject", Reason: "not now",
	}); err != nil {
		t.Fatalf("ApprovalDecide = %v", err)
	}

	res := w.inbox(w.as(ui), t)

	if r := rowOf(
		res,
		"chat:"+ask.Permission.RequestID,
	); r == nil || r.State != inbox.StateApproved ||
		r.AnsweredAt == nil {
		t.Errorf("the allowed permission = %+v", r)
	}

	if r := rowOf(res, "approval:"+req.ID); r == nil || r.State != inbox.StateRejected || r.AnsweredAt == nil {
		t.Errorf("the rejected approval = %+v", r)
	}

	if res.Waiting != 0 {
		t.Errorf("waiting = %d, want 0", res.Waiting)
	}
}

// Acceptance criterion 5 of epic 77: a tool outside Stuarty reports no item, no destination and no undo.
func TestInboxANotStuartyToolReportsNothingBeyondItsTitle(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")

	_, ask := w.ask(person(), t, project, "permission-other")

	r := rowOf(w.inbox(w.as(ui), t), "chat:"+ask.Permission.RequestID)
	if r == nil {
		t.Fatal("the permission is not listed")
	}

	if r.Tool != "Run shell command" || r.Item != "" || r.Destination != "" || r.ActionKey != "" || r.Undoable {
		t.Errorf("row = %+v, want only the title", r)
	}

	raw := mustJSON(t, r)
	if strings.Contains(raw, "evil.example") {
		t.Errorf("the input of the call reached the row: %s", raw)
	}
}

// Acceptance criterion 3 of epic 77 (SDD-007 T-72): the session token of a hosted agent is refused with the one
// code an agent learns, before its scope is looked at.
func TestSecurity_T72_SessionTokenCannotReadTheInbox(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")

	s, _ := w.ask(person(), t, project, "permission")

	_, err := w.svc.InboxList(agentCtx(t, w.tokens, s.ID, project), service.InboxListRequest{})
	se := serviceError(t, err)

	if se.Code != service.CodePrivilegedSession || se.HTTPStatus() != 403 || se.Hint == "" {
		t.Errorf("InboxList with a session token = %s (%d) hint %q, want E_PRIVILEGED_SESSION 403 with a hint",
			se.Code, se.HTTPStatus(), se.Hint)
	}

	// The local token, a key and a person read it.
	for name, ctx := range map[string]context.Context{
		"the local token": w.as("local:cli"), "a person": person(), "a key": w.asKey(),
	} {
		if _, err := w.svc.InboxList(ctx, service.InboxListRequest{}); err != nil {
			t.Errorf("InboxList as %s = %v", name, err)
		}
	}

	noScope := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:key_aaaaaaaaaa", Scopes: []string{service.ScopeEdit}},
	)
	if _, err := w.svc.InboxList(noScope, service.InboxListRequest{}); code(err) != service.CodeForbidden {
		t.Errorf("InboxList without the read scope = %v, want E_FORBIDDEN", err)
	}

	if _, err := w.svc.InboxList(
		context.Background(),
		service.InboxListRequest{},
	); code(
		err,
	) != service.CodeUnauthorized {
		t.Errorf("InboxList without credentials = %v, want E_UNAUTHORIZED", err)
	}
}

// Acceptance criterion 4 of epic 77 (SDD-007 T-06): a member reads their own requests and no row of another member,
// of either source; a key limited to a project reads that project's rows only.
func TestSecurity_T06_InboxShowsOnlyTheCallersRequests(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)

	member := func(id string) context.Context {
		return service.WithCaller(context.Background(), service.Caller{
			Actor: "user:" + id, Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: id, Browser: true,
		})
	}

	alice, bob := member("usr_0000000001"), member("usr_0000000002")

	mine, err := testproject.CreateWithTimeline(alice, w.svc, "Alice video")
	if err != nil {
		t.Fatal(err)
	}

	theirs, err := testproject.CreateWithTimeline(bob, w.svc, "Bob video")
	if err != nil {
		t.Fatal(err)
	}

	_, aliceAsk := w.ask(alice, t, mine.ProjectID, "permission")
	_, bobAsk := w.ask(bob, t, theirs.ProjectID, "permission-stuarty")
	aliceReq := w.addRequestFor("usr_0000000001", mine.ProjectID)
	bobReq := w.addRequestFor("usr_0000000002", theirs.ProjectID)

	own := w.inbox(alice, t)
	if rowOf(own, "chat:"+aliceAsk.Permission.RequestID) == nil || rowOf(own, "approval:"+aliceReq.ID) == nil {
		t.Fatalf("Alice does not see her own requests: %+v", own.Rows)
	}

	for _, id := range []string{"chat:" + bobAsk.Permission.RequestID, "approval:" + bobReq.ID} {
		if rowOf(own, id) != nil {
			t.Errorf("Alice sees %s, a request of Bob", id)
		}
	}

	if len(own.Rows) != 2 || own.Waiting != 2 {
		t.Errorf("Alice's list = %d rows, %d waiting, want 2 and 2", len(own.Rows), own.Waiting)
	}

	theirsList := w.inbox(bob, t)
	if rowOf(theirsList, "chat:"+aliceAsk.Permission.RequestID) != nil ||
		rowOf(theirsList, "approval:"+aliceReq.ID) != nil {
		t.Errorf("Bob sees a request of Alice: %+v", theirsList.Rows)
	}

	// Not even the sentence of the other member's request is in the answer.
	raw := mustJSON(t, own)
	if strings.Contains(raw, "ast_3c5v8n2w6d") || strings.Contains(raw, theirs.ProjectID) {
		t.Errorf("Alice's answer carries a trace of Bob's work: %s", raw)
	}

	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:key_aaaaaaaaaa", Scopes: service.AllScopes, ProjectID: mine.ProjectID,
	})

	for _, r := range w.inbox(limited, t).Rows {
		if r.ProjectID != mine.ProjectID {
			t.Errorf("a key limited to %s reads %+v", mine.ProjectID, r)
		}
	}

	if got := w.inbox(limited, t); len(got.Rows) != 0 && rowOf(got, "chat:"+bobAsk.Permission.RequestID) != nil {
		t.Error("a limited key reads the request of another project")
	}
}

// SDD-007 T-71: what the agent put in the input of a call beyond the allowlist reaches neither the row nor the stream
// of the transcript, and the text that does is cleaned.
func TestSecurity_T71_InboxCarriesOnlyTheAllowlistOfAPermission(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")

	_, ask := w.ask(person(), t, project, "permission-stuarty")

	raw := mustJSON(t, w.inbox(w.as(ui), t))

	for _, secret := range []string{"sk-hostile-secret-value", "hostile-bearer", "sk-hostile-in-args", "api_key", "authorization"} {
		if strings.Contains(raw, secret) {
			t.Errorf("the answer carries %q: %s", secret, raw)
		}
	}

	if strings.ContainsRune(raw, 0) || strings.Contains(raw, `\u0000`) {
		t.Errorf("a control character reached the answer: %s", raw)
	}

	if ask.Permission.Detail == nil || ask.Permission.Detail.Item != "ast_3c5v8n2w6d" {
		t.Errorf("the detail of the event = %+v", ask.Permission.Detail)
	}
}

// The settled rows are capped and the approval half reads each state: a long history never hides a request that waits.
func TestInboxKeepsEveryWaitingApprovalBeyondTheCapOfTheSettledOnes(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)

	var waiting []string

	for range inbox.MaxRecent + 5 {
		waiting = append(waiting, w.addRequest().ID)
	}

	res := w.inbox(w.as(ui), t)
	if res.Waiting != len(waiting) || len(res.Rows) != len(waiting) {
		t.Fatalf("rows = %d, waiting = %d, want %d of each", len(res.Rows), res.Waiting, len(waiting))
	}

	for _, id := range waiting[:inbox.MaxRecent] {
		if _, err := w.svc.ApprovalDecide(
			w.asPerson(),
			service.ApprovalDecideRequest{ID: id, Decision: "reject"},
		); err != nil {
			t.Fatal(err)
		}
	}

	res = w.inbox(w.as(ui), t)
	if res.Waiting != 5 || len(res.Rows) != 5+inbox.MaxRecent {
		t.Errorf("after settling %d: waiting = %d, rows = %d", inbox.MaxRecent, res.Waiting, len(res.Rows))
	}

	for i, r := range res.Rows {
		if (i < 5) != (r.State == inbox.StateWaiting) {
			t.Fatalf("row %d is %s: the waiting rows are not first", i, r.State)
		}
	}

	if r := res.Rows[0]; r.Source != inbox.SourceApproval || !strings.HasPrefix(r.SourceID, "apr_") {
		t.Errorf("first row = %+v", r)
	}
}
