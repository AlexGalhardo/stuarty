package servicetest

import (
	"errors"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// A plan window pinned to a revision that is not the current one is refused on
// the row it loads, with the current revision (SDD-003 section 6.3).
func TestPlanWindowPinsTheRevision(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})
	project := m.exportable()
	ctx := m.as(ui)

	head, err := m.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: project})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.PlanWindow(ctx, service.PlanWindowRequest{
		ProjectID: project, Until: 10, Pin: head.Revision,
	}); err != nil {
		t.Fatalf("the current revision: %v", err)
	}

	_, err = m.svc.PlanWindow(ctx, service.PlanWindowRequest{ProjectID: project, Until: 10, Pin: head.Revision - 1})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != command.CodeInvalidState || se.Details["current_revision"] != head.Revision {
		t.Errorf("a stale pin = %v, want E_INVALID_STATE with the current revision", err)
	}
}
