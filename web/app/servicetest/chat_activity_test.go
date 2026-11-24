package servicetest

import (
	"context"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// The activity of a hosted session: the skill and the tool it is using (SDD-017 section 8.1, epic 76).

// newActivityWorld is a chat world whose service also keeps a library, so a skill can be read.
func newActivityWorld(t *testing.T) *chatWorld {
	t.Helper()

	w := &chatWorld{tokens: agenthost.NewTokens(nil)}

	w.env = newEnvWith(t, func(e *env, o *service.Options) {
		w.host = agenthost.New(agenthost.Options{
			Store: e.store.Chat(), Tokens: w.tokens, Workspace: string(e.dir), Version: "test",
			Limits: agenthost.Limits{InputClose: 200 * time.Millisecond},
			// An address nobody listens on: the fake agent announces a call of an MCP tool before it makes it,
			// which is all the activity needs.
			MCPURL: func() string { return "http://127.0.0.1:1/mcp" },
		})
		o.Agents = w.host
		o.Workspace = string(e.dir)
	})

	t.Cleanup(w.host.Shutdown)

	return w
}

// reviewedSkill writes a skill and has a person review it, so that an agent may read it.
func (w *chatWorld) reviewedSkill(t *testing.T, name string) {
	t.Helper()

	if _, err := w.svc.SkillWrite(person(), service.SkillWriteRequest{
		Name: name, Description: "Write captions for a short video.", Body: "# Captions\n\nKeep lines short.\n",
	}); err != nil {
		t.Fatalf("SkillWrite = %v", err)
	}

	got, err := w.svc.SkillGet(person(), service.LibraryRef{Name: name})
	if err != nil {
		t.Fatalf("SkillGet = %v", err)
	}

	if _, err := w.svc.SkillMarkReviewed(person(), service.SkillReviewRequest{
		Name: name, ContentHash: got.ContentHash,
	}); err != nil {
		t.Fatalf("SkillMarkReviewed = %v", err)
	}
}

// tokenCtx is the context of a request made with a session token bound to a session of a project.
func (w *chatWorld) tokenCtx(t *testing.T, session, project string) context.Context {
	t.Helper()

	return agentCtx(t, w.tokens, session, project)
}

// skillEvents are the skill_loaded events of a session, as the transcript holds them.
func (w *chatWorld) skillEvents(t *testing.T, session string) []chat.Event {
	t.Helper()

	res, err := w.svc.ChatTranscriptGet(w.as(ui), service.ChatTranscriptRequest{SessionID: session})
	if err != nil {
		t.Fatalf("ChatTranscriptGet = %v", err)
	}

	var out []chat.Event

	for _, ev := range res.Events {
		if ev.Kind == chat.KindSkillLoaded {
			out = append(out, ev)
		}
	}

	return out
}

// Acceptance criteria 1 and 3 of epic 76: a skill the agent of a session read is recorded in that session, with
// its hash; the same skill read by a person, by the local token or by a key records nothing.
func TestChatActivityRecordsOnlyTheSkillAnAgentReadWithItsSessionToken(t *testing.T) {
	t.Parallel()

	w := newActivityWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "echo", "echo")
	w.reviewedSkill(t, "captions")

	s := w.start(t, project, "echo")
	other := w.start(t, w.newProject("Another"), "echo")

	// A person in the browser, the local token and a key: none carries a session.
	for name, ctx := range map[string]context.Context{
		"a person": person(),
		"a key": service.WithCaller(context.Background(), service.Caller{
			Actor: "key:key_aaaaaaaaaa", Scopes: service.AllScopes, ProjectID: project,
		}),
		"the local token": service.WithCaller(context.Background(), service.Caller{
			Actor: "local:cli", Scopes: service.AllScopes,
		}),
	} {
		if _, err := w.svc.SkillGet(ctx, service.LibraryRef{Name: "captions"}); err != nil {
			t.Fatalf("SkillGet as %s = %v", name, err)
		}
	}

	if evs := w.skillEvents(t, s.ID); len(evs) != 0 {
		t.Fatalf("a read with no session token recorded %+v", evs)
	}

	got, err := w.svc.SkillGet(w.tokenCtx(t, s.ID, project), service.LibraryRef{Name: "captions", AgentView: true})
	if err != nil {
		t.Fatalf("SkillGet with the session token = %v", err)
	}

	evs := w.skillEvents(t, s.ID)
	if len(evs) != 1 || evs[0].Skill == nil || evs[0].Skill.Name != "captions" ||
		evs[0].Skill.Hash != got.ContentHash ||
		evs[0].Skill.Hash == "" {
		t.Fatalf("events = %+v, want one skill_loaded for captions with hash %q", evs, got.ContentHash)
	}

	// The session of another token is untouched.
	if evs := w.skillEvents(t, other.ID); len(evs) != 0 {
		t.Fatalf("another session recorded %+v", evs)
	}
}

func TestChatActivityARefusedReadRecordsNothing(t *testing.T) {
	t.Parallel()

	w := newActivityWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "echo", "echo")

	s := w.start(t, project, "echo")

	ctx := w.tokenCtx(t, s.ID, project)
	outside := service.WithCaller(
		context.Background(), service.Caller{Actor: "local:claude-code-k3m9x2", Scopes: service.AllScopes},
	)

	// Written by an agent, so not reviewed: an agent view is refused.
	if _, err := w.svc.SkillWrite(outside, service.SkillWriteRequest{
		Name: "draft", Description: "d", Body: "Body.\n",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := w.svc.SkillGet(ctx, service.LibraryRef{Name: "draft", AgentView: true}); err == nil {
		t.Fatal("an unreviewed skill was read through an agent view")
	}

	if _, err := w.svc.SkillGet(ctx, service.LibraryRef{Name: "missing", AgentView: true}); err == nil {
		t.Fatal("a skill that does not exist was read")
	}

	if evs := w.skillEvents(t, s.ID); len(evs) != 0 {
		t.Fatalf("a refused read recorded %+v", evs)
	}
}

func (w *chatWorld) activity(ctx context.Context, t *testing.T, session string) *service.ChatActivityResult {
	t.Helper()

	res, err := w.svc.ChatActivityGet(ctx, service.ChatRef{SessionID: session})
	if err != nil {
		t.Fatalf("ChatActivityGet = %v", err)
	}

	return res
}

// Acceptance criteria 1, 2 and 4 of epic 76: the activity is none, then available, then loaded as the session
// reads a skill, and a call of a tool of Stuarty carries its technical name and its description key.
func TestChatActivityTellsTheSkillAndTheTool(t *testing.T) {
	t.Parallel()

	w := newActivityWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "mcp", "mcp")

	s := w.start(t, project, "mcp")
	reader := w.as(ui)

	if a := w.activity(reader, t, s.ID); a.Skill.State != chat.SkillNone || a.Skill.Name != "" || a.Tool != nil ||
		a.SkillsLoaded == nil || len(a.SkillsLoaded) != 0 || a.SessionID != s.ID || a.UntrustedNotice == "" {
		t.Fatalf("a new session with an empty library = %+v", a)
	}

	w.reviewedSkill(t, "captions")

	if a := w.activity(reader, t, s.ID); a.Skill.State != chat.SkillAvailable || a.Skill.Name != "" {
		t.Fatalf("with a reviewed skill and none read = %+v", a.Skill)
	}

	// A person reading the skill changes nothing.
	if _, err := w.svc.SkillGet(person(), service.LibraryRef{Name: "captions"}); err != nil {
		t.Fatal(err)
	}

	if a := w.activity(reader, t, s.ID); a.Skill.State == chat.SkillLoaded {
		t.Fatalf("a read by a person loaded the skill: %+v", a)
	}

	// The agent reads it, with the token of its session.
	if _, err := w.svc.SkillGet(
		w.tokenCtx(t, s.ID, project),
		service.LibraryRef{Name: "captions", AgentView: true},
	); err != nil {
		t.Fatal(err)
	}

	a := w.activity(reader, t, s.ID)
	if a.Skill.State != chat.SkillLoaded || a.Skill.Name != "captions" || len(a.SkillsLoaded) != 1 ||
		a.SkillsLoaded[0].Name != "captions" || a.SkillsLoaded[0].Hash == "" {
		t.Fatalf("after the agent read the skill = %+v", a)
	}

	// The agent calls a tool of Stuarty, the way the fake agent does: it names the server and the tool and
	// sends the action.
	if _, err := w.svc.ChatSend(person(), service.ChatSendRequest{
		SessionID: s.ID, Text: `mcp library {"action":"get","kind":"skill","name":"captions"}`,
	}); err != nil {
		t.Fatal(err)
	}

	w.wait(t, s.ID, func(e chat.Event) bool {
		return e.Kind == chat.KindToolCall && e.Tool.Status == chat.ToolCompleted
	})

	a = w.activity(reader, t, s.ID)
	if a.Tool == nil || a.Tool.TechnicalName != "library" || a.Tool.Action != "get" || a.Tool.FromAgent ||
		a.Tool.DescriptionKey != "ui.agent_activity.action.library_get" || a.Tool.Status != chat.ToolCompleted ||
		a.Tool.Title != "mcp__stuarty__library" {
		t.Fatalf("the tool = %+v", a.Tool)
	}

	if a.Skill.Name != "captions" || a.UpdatedAt.IsZero() {
		t.Fatalf("activity = %+v", a)
	}
}

func TestChatActivityOfAnEndedSessionIsReadFromTheTranscript(t *testing.T) {
	t.Parallel()

	w := newActivityWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "echo", "echo")
	w.reviewedSkill(t, "captions")

	s := w.start(t, project, "echo")

	if _, err := w.svc.SkillGet(
		w.tokenCtx(t, s.ID, project),
		service.LibraryRef{Name: "captions", AgentView: true},
	); err != nil {
		t.Fatal(err)
	}

	ended, err := w.svc.ChatCancel(person(), service.ChatCancelRequest{SessionID: s.ID, End: true})
	if err != nil || ended.Live {
		t.Fatalf("end = %+v, %v", ended, err)
	}

	a := w.activity(w.as(ui), t, s.ID)
	if a.State != chat.StateEnded || a.Skill.State != chat.SkillLoaded || a.Skill.Name != "captions" {
		t.Fatalf("an ended session = %+v", a)
	}
}

// Acceptance criterion 5 of epic 76 (SDD-007): a member asking for the activity of another member's session gets
// the answer of an id that does not exist, and so does a key limited to another project.
func TestSecurity_T71_ChatActivityOfAnotherMembersSessionIsNotFound(t *testing.T) {
	t.Parallel()

	w := newActivityWorld(t)
	w.profile(t, "echo", "echo")

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

	aliceSession, err := w.svc.ChatSessionStart(
		alice,
		service.ChatSessionStartRequest{ProjectID: mine.ProjectID, Profile: "echo"},
	)
	if err != nil {
		t.Fatal(err)
	}

	bobSession, err := w.svc.ChatSessionStart(
		bob,
		service.ChatSessionStartRequest{ProjectID: theirs.ProjectID, Profile: "echo"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := w.svc.ChatActivityGet(alice, service.ChatRef{SessionID: aliceSession.ID}); err != nil {
		t.Fatalf("a member reading the activity of their own session = %v", err)
	}

	_, refused := w.svc.ChatActivityGet(alice, service.ChatRef{SessionID: bobSession.ID})
	_, missing := w.svc.ChatActivityGet(alice, service.ChatRef{SessionID: "cht_0000000999"})

	if code(refused) != service.CodeChatNotFound || code(missing) != service.CodeChatNotFound {
		t.Fatalf("another member's session = %v, a made-up id = %v", refused, missing)
	}

	if refused.Error() != missing.Error() {
		t.Errorf("the two answers differ, so one reveals that the session exists: %q and %q", refused, missing)
	}

	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:key_aaaaaaaaaa", Scopes: service.AllScopes, ProjectID: mine.ProjectID,
	})
	if _, err := w.svc.ChatActivityGet(
		limited,
		service.ChatRef{SessionID: bobSession.ID},
	); code(
		err,
	) != service.CodeChatNotFound {
		t.Errorf("a key of another project = %v", err)
	}

	if _, err := w.svc.ChatActivityGet(
		context.Background(),
		service.ChatRef{SessionID: aliceSession.ID},
	); code(
		err,
	) != service.CodeUnauthorized {
		t.Errorf("no credentials = %v", err)
	}

	noRead := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:key_bbbbbbbbbb", Scopes: []string{service.ScopeEdit}},
	)
	if _, err := w.svc.ChatActivityGet(
		noRead,
		service.ChatRef{SessionID: aliceSession.ID},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("a key without the read scope = %v", err)
	}
}
