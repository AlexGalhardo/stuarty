package agenthost

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// SDD-017 section 8.1: a skill the agent read is a transcript event of its own session.
func TestRecordSkillAddsAnEventToTheSessionAndStoresIt(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_0000000042", e.profile("echo"))
	f := follow(t, e.host, s.ID)

	if !e.host.RecordSkill(s.ID, chat.SkillRef{Name: "captions", Hash: "sha256:aa"}) {
		t.Fatal("RecordSkill = false for a live session")
	}

	ev := f.next(kind(chat.KindSkillLoaded))
	if ev.Skill == nil || ev.Skill.Name != "captions" || ev.Skill.Hash != "sha256:aa" || ev.Text != "" {
		t.Fatalf("event = %+v", ev)
	}

	stored, err := e.store.EventList(context.Background(), s.ID, 0, chat.MaxTranscriptPage)
	if err != nil {
		t.Fatal(err)
	}

	var found bool

	for _, se := range stored {
		if se.Kind == chat.KindSkillLoaded && se.Seq == ev.Seq && se.Skill != nil && se.Skill.Name == "captions" {
			found = true
		}
	}

	if !found {
		t.Fatalf("the event was not stored: %+v", stored)
	}
}

func TestRecordSkillTwiceInARowIsOneEventAndAnotherSkillIsAnother(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_0000000042", e.profile("echo"))
	f := follow(t, e.host, s.ID)

	a, b := chat.SkillRef{Name: "a", Hash: "h1"}, chat.SkillRef{Name: "b", Hash: "h1"}

	for _, ref := range []chat.SkillRef{a, a, b, a, {Name: "a", Hash: "h2"}} {
		e.host.RecordSkill(s.ID, ref)
	}

	var names []string

	for range 4 {
		ev := f.next(kind(chat.KindSkillLoaded))
		names = append(names, ev.Skill.Name+":"+ev.Skill.Hash)
	}

	want := []string{"a:h1", "b:h1", "a:h1", "a:h2"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("events = %v, want %v", names, want)
		}
	}
}

func TestRecordSkillOnASessionThatIsNotLiveRecordsNothing(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_0000000042", e.profile("echo"))

	e.host.End(s.ID, chat.EndUser)

	if e.host.RecordSkill(s.ID, chat.SkillRef{Name: "a"}) ||
		e.host.RecordSkill("cht_9999999999", chat.SkillRef{Name: "a"}) {
		t.Fatal("RecordSkill = true for a session that is not live")
	}

	stored, err := e.store.EventList(context.Background(), s.ID, 0, chat.MaxTranscriptPage)
	if err != nil {
		t.Fatal(err)
	}

	for _, se := range stored {
		if se.Kind == chat.KindSkillLoaded {
			t.Fatalf("an ended session got %+v", se)
		}
	}
}

// The activity of a live session is told from the skills it read and its last tool call, oldest first, which the
// host keeps in memory.
func TestActivityEventsAreTheSkillsAndTheLastToolCall(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_0000000042", e.profile("echo"))
	f := follow(t, e.host, s.ID)

	if got, ok := e.host.ActivityEvents(s.ID); !ok || len(got) != 0 {
		t.Fatalf("a new session = %+v, %v", got, ok)
	}

	e.host.RecordSkill(s.ID, chat.SkillRef{Name: "a", Hash: "h"})

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	f.next(func(ev chat.Event) bool { return ev.Kind == chat.KindToolCall && ev.Tool.Status == chat.ToolCompleted })
	e.host.RecordSkill(s.ID, chat.SkillRef{Name: "b", Hash: "h"})

	got, ok := e.host.ActivityEvents(s.ID)
	if !ok || len(got) != 3 {
		t.Fatalf("events = %+v, %v", got, ok)
	}

	kinds := []chat.Kind{got[0].Kind, got[1].Kind, got[2].Kind}
	if kinds[0] != chat.KindSkillLoaded || kinds[1] != chat.KindToolCall || kinds[2] != chat.KindSkillLoaded ||
		got[0].Seq >= got[1].Seq || got[1].Seq >= got[2].Seq {
		t.Fatalf("kinds and order = %v %+v", kinds, got)
	}

	if got[1].Tool.Status != chat.ToolCompleted {
		t.Errorf("the tool call is not the merged one: %+v", got[1].Tool)
	}

	e.host.End(s.ID, chat.EndUser)

	if _, ok := e.host.ActivityEvents(s.ID); ok {
		t.Error("a session that is not live answered from memory")
	}
}
