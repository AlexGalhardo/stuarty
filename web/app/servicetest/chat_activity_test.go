package servicetest

import (
	"context"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
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
