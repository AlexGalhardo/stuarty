package servicetest

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/service/approval"
)

// chatCallers are the callers that are not a person in a browser: each must be refused every
// operation of a person with one code, whatever scope it holds.
func chatCallers(t *testing.T, w *chatWorld, project string) map[string]context.Context {
	t.Helper()

	return map[string]context.Context{
		"the local token": w.as(token),
		"an API key":      w.asKey(),
		"a hosted agent":  agentCtx(t, w.tokens, "cht_0000000009", project),
		"a read-only key": service.WithCaller(
			context.Background(), service.Caller{Actor: key, Scopes: []string{service.ScopeRead}},
		),
	}
}

// SDD-007 T-70: a profile is a command that will run on the machine, so only a person in the app
// writes one; every agent credential is refused, and nothing it tried is stored.
func TestSecurity_T70_ProfileIsWrittenByAPersonOnly(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "mine", "echo")

	set := service.AgentProfileSetRequest{Name: "evil", Command: fakeAgentBinary(t), Args: []string{"--mode=echo"}}

	for who, ctx := range chatCallers(t, w, project) {
		_, err := w.svc.AgentProfileSet(ctx, set)
		if c := code(err); c != service.CodePrivilegedSession {
			t.Errorf("AgentProfileSet by %s = %q, want %s", who, c, service.CodePrivilegedSession)
		}

		// Replacing a profile of the person is writing one too.
		replace := set
		replace.Name = "mine"

		if _, err := w.svc.AgentProfileSet(ctx, replace); code(err) != service.CodePrivilegedSession {
			t.Errorf("a replacement by %s = %v", who, err)
		}

		if _, err := w.svc.AgentProfileRemove(
			ctx,
			service.AgentProfileRemoveRequest{Name: "mine"},
		); code(
			err,
		) != service.CodePrivilegedSession {
			t.Errorf("AgentProfileRemove by %s = %v", who, err)
		}
	}

	// A browser session without the scope of the work is refused too.
	narrow := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: []string{service.ScopeRead}, Browser: true,
	})
	if _, err := w.svc.AgentProfileSet(narrow, set); err == nil {
		t.Error("a browser session without the scope wrote a profile")
	}

	list, err := w.svc.AgentProfileList(person(), service.AgentProfileListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Profiles) != 1 || list.Profiles[0].Name != "mine" || list.Profiles[0].Args[0] != "--mode=echo" {
		t.Errorf("a refused call changed the profiles: %+v", list.Profiles)
	}
}

// SDD-007 T-72: the token of a session is never a person and never admin: it cannot start,
// steer or end a chat, nor answer the permission request of its own agent.
func TestSecurity_T72_SessionTokenIsNotAPersonAndNotAdmin(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "ask", "permission")

	s := w.start(t, project, "ask")

	if _, err := w.svc.ChatSend(person(), service.ChatSendRequest{SessionID: s.ID, Text: "go"}); err != nil {
		t.Fatal(err)
	}

	ask := w.wait(t, s.ID, isKind(chat.KindPermissionAsked))

	// The grant is the one the host issued: no admin scope, whatever the code asks later.
	for _, scope := range agenthost.GrantScopes() {
		if scope == service.ScopeAdmin {
			t.Fatal("a session token carries the admin scope")
		}
	}

	agent := agentCtx(t, w.tokens, s.ID, project)

	calls := map[string]func() error{
		"ChatSessionStart": func() error {
			_, err := w.svc.ChatSessionStart(agent, service.ChatSessionStartRequest{ProjectID: project, Profile: "ask"})
			return err
		},
		"ChatSend": func() error {
			_, err := w.svc.ChatSend(agent, service.ChatSendRequest{SessionID: s.ID, Text: "more"})
			return err
		},
		"ChatCancel": func() error {
			_, err := w.svc.ChatCancel(agent, service.ChatCancelRequest{SessionID: s.ID, End: true})
			return err
		},
		"ChatAnswerPermission": func() error {
			_, err := w.svc.ChatAnswerPermission(agent, service.ChatAnswerRequest{
				SessionID: s.ID, RequestID: ask.Permission.RequestID, Answer: "allow",
			})
			return err
		},
		"ChatSessionDelete": func() error {
			_, err := w.svc.ChatSessionDelete(agent, service.ChatRef{SessionID: s.ID})
			return err
		},
		"ProjectCreate": func() error {
			_, err := w.svc.ProjectCreate(agent, service.ProjectCreateRequest{Name: "x"})
			return err
		},
	}

	for name, call := range calls {
		c := code(call())
		if c != service.CodePrivilegedSession && c != service.CodeForbidden {
			t.Errorf("%s by the agent = %q", name, c)
		}
	}

	// Nothing the agent tried took effect: the request still waits for a person.
	got, err := w.svc.ChatSessionGet(person(), service.ChatRef{SessionID: s.ID})
	if err != nil || !got.Live || got.State != chat.StateRunning {
		t.Fatalf("the session after the refusals = %+v, %v", got, err)
	}
}

// SDD-007 T-74: the chat decides no approval, and the agent reaches none of the operations of the
// queue that only a person may call; a chat answer names a request of the chat, never one of the
// queue.
func TestSecurity_T74_ChatCannotDecideApprovals(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "ask", "permission")

	pending := w.addRequestFor("", project)
	s := w.start(t, project, "ask")

	agent := agentCtx(t, w.tokens, s.ID, project)

	decide := service.ApprovalDecideRequest{ID: pending.ID, Decision: "approve"}
	if c := code(callDecide(agent, w, decide)); c != service.CodePrivilegedSession {
		t.Errorf("ApprovalDecide by the agent = %q, want %s", c, service.CodePrivilegedSession)
	}

	if _, err := w.svc.ApprovalRuleSet(
		agent,
		service.ApprovalRuleSetRequest{Kind: "publish.post"},
	); code(
		err,
	) != service.CodePrivilegedSession {
		t.Errorf("ApprovalRuleSet by the agent = %v", err)
	}

	// A person answering in the chat with the id of a queue request does not decide it.
	if _, err := w.svc.ChatAnswerPermission(person(), service.ChatAnswerRequest{
		SessionID: s.ID, RequestID: pending.ID, Answer: "allow",
	}); code(err) != "E_CHAT_NOT_FOUND" {
		t.Errorf("an answer naming a queue request = %v", err)
	}

	got, err := w.svc.ApprovalGet(person(), service.ApprovalRef{ID: pending.ID})
	if err != nil || got.State != approval.StatePending {
		t.Fatalf("the request after the attempts = %+v, %v", got, err)
	}

	rules, err := w.svc.ApprovalRuleList(person(), service.ApprovalRuleListRequest{})
	if err != nil || len(rules.Rules) != 0 {
		t.Fatalf("a rule was written: %+v, %v", rules, err)
	}
}

func callDecide(ctx context.Context, w *chatWorld, req service.ApprovalDecideRequest) error {
	_, err := w.svc.ApprovalDecide(ctx, req)

	return err
}
