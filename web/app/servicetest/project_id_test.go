package servicetest

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// A timeline id stands in the place of a project id on every surface, so a new project must not take the id of a
// timeline of another project: the id would then answer for the new project and hide the timeline.
func TestProjectCreateRefusesTheIDOfATimeline(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	owner := e.newProject("Owner project")
	_, second := e.addTimeline(owner, "Second cut")

	req := testproject.Request("Taken id")
	req.ID = second

	if _, err := e.svc.ProjectCreate(e.as(ui), req); code(err) != command.CodeIDExists {
		t.Fatalf("err = %v, want E_ID_EXISTS", err)
	}

	got, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: second})
	if err != nil || got.ProjectID != owner || got.TimelineID != second {
		t.Fatalf("the timeline no longer answers for its project: %+v, %v", got, err)
	}
}
