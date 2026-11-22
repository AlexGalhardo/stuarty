package servicetest

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// A project of another user with several timelines answers an incomplete request as an id of nothing does: the check
// of the request comes first in both, so the code does not say that the project exists (T-06).
func TestSecurity_T06_SeveralTimelinesDoNotTellThatAProjectExists(t *testing.T) {
	e := newProjectWideEnv(t, &probe{estimate: usd(1)})
	owner := networkPerson("usr_0000000002", service.AllScopes, false)
	other := networkPerson("usr_0000000001", service.AllScopes, false)

	p, err := testproject.CreateWithTimeline(owner, e.svc, "Two timelines")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectItemAdd(owner, service.ProjectItemAddRequest{
		ProjectID: p.ProjectID, Kind: "timeline", Title: "Second",
	}); err != nil {
		t.Fatal(err)
	}

	_, theirs := e.svc.PlanWindow(other, service.PlanWindowRequest{ProjectID: p.ProjectID})
	_, none := e.svc.PlanWindow(other, service.PlanWindowRequest{ProjectID: "prj_zzzzzzzzzz"})

	if code(theirs) != code(none) || code(theirs) == "" {
		t.Errorf("a project of another user answers %v, an id of nothing %v", theirs, none)
	}

	_, theirs = e.svc.PlanWindow(other, service.PlanWindowRequest{ProjectID: p.ProjectID, Until: 1})
	_, none = e.svc.PlanWindow(other, service.PlanWindowRequest{ProjectID: "prj_zzzzzzzzzz", Until: 1})

	if code(theirs) != "E_NOT_FOUND" || code(none) != "E_NOT_FOUND" {
		t.Errorf("a complete request answers %v for the project and %v for nothing, want E_NOT_FOUND", theirs, none)
	}
}
