package servicetest

import (
	"context"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// TestNestedKeyLimitedToAProject: a composition names a timeline of its own production only, so a key limited to
// one project cannot reach another project through one: placing a timeline of another project is E_NOT_FOUND, as
// for any caller, and the frames of that timeline stay out of reach. Its own nested timeline is in reach.
func TestNestedKeyLimitedToAProject(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	mine, child, _ := nestedProject(t, e)
	other := e.newProject("other")

	if _, err := e.textIn(other, e.editIn(t, other, op("track.add", `{"kind":"visual"}`)).Created[0], "0s",
		"secret"); err != nil {
		t.Fatal(err)
	}

	limited := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:k", Scopes: service.AllScopes, ProjectID: mine},
	)

	track := e.editIn(t, mine, op("track.add", `{"kind":"visual"}`)).Created[0]
	place := op("clip.add_composition", fmt.Sprintf(`{"track_id":%q,"timeline_id":%q,"start":"5s"}`, track, other))

	for name, ctx := range map[string]context.Context{"the person": e.as(ui), "a limited key": limited} {
		_, err := e.svc.Edit(ctx, service.EditRequest{
			ProjectID: mine, TimelineID: mine, BaseRevision: e.head(mine).Revision,
			Operations: []command.Operation{place},
		})
		if code(err) != command.CodeNotFound {
			t.Errorf("%s placed a timeline of another project: %v", name, err)
		}
	}

	if _, err := e.svc.PlanWindow(limited, service.PlanWindowRequest{
		ProjectID: other, TimelineID: other, From: 0, Until: 30,
	}); err == nil {
		t.Error("a limited key read the plan of another project")
	}

	if _, err := e.svc.PlanWindow(limited, service.PlanWindowRequest{
		ProjectID: child, TimelineID: child, From: 0, Until: 30,
	}); err != nil {
		t.Errorf("a limited key cannot read the nested timeline of its own project: %v", err)
	}
}
