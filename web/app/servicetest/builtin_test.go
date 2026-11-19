package servicetest

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/resources/skills"
)

// A fresh workspace lists the skills that ship with the product, as built in, reviewed, and with no lint finding
// (SDD-018 section 16, epic 16 acceptance criteria 2 and 5).
func TestBuiltInSkillsAreInstalledWhenTheServiceStarts(t *testing.T) {
	t.Parallel()

	e := newEnvWith(t, func(e *env, o *service.Options) {
		o.Workspace = string(e.dir)
		o.BuiltInSkills = skills.FS()
	})
	agent := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "local:claude-code-k3m9x2", Scopes: service.AllScopes},
	)

	// An agent sees them without any review by a person: the product wrote and read them.
	list, err := e.svc.SkillList(agent, service.SkillListRequest{AgentView: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Entries) < 8 {
		t.Fatalf("%d built-in skills: %+v", len(list.Entries), list.Entries)
	}

	seen := map[string]bool{}

	for i := range list.Entries {
		en := &list.Entries[i]

		seen[en.Name] = true

		if en.Origin != "built-in" || !en.Reviewed || en.State != "ok" || en.LintErrors != 0 || en.LintWarns != 0 {
			t.Errorf("%s: %+v", en.Name, *en)
		}
	}

	for _, name := range []string{"start-here", "brief-a-production", "short-vertical-video", "recall-learnings"} {
		if !seen[name] {
			t.Errorf("%s is not installed", name)
		}
	}

	// Only the built-in origin is offered by the filter, and a person edits one of them: it becomes a user skill.
	got, err := e.svc.SkillGet(agent, service.LibraryRef{Name: "start-here"})
	if err != nil || got.Entry.Origin != "built-in" || got.Body == "" {
		t.Fatalf("%+v %v", got, err)
	}

	person := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
	)

	if _, err := e.svc.SkillWrite(person, service.SkillWriteRequest{
		Name: "start-here", Description: got.Entry.Description, Body: "My own router.",
	}); err != nil {
		t.Fatal(err)
	}

	edited, err := e.svc.SkillGet(person, service.LibraryRef{Name: "start-here"})
	if err != nil || edited.Entry.Origin != "user" || edited.Body != "My own router." {
		t.Fatalf("%+v %v", edited, err)
	}

	byOrigin, err := e.svc.SkillList(agent, service.SkillListRequest{Origin: "built-in", AgentView: true})
	if err != nil {
		t.Fatal(err)
	}

	for i := range byOrigin.Entries {
		if byOrigin.Entries[i].Name == "start-here" {
			t.Fatal("an edited built-in skill is still listed as built-in")
		}
	}
}
