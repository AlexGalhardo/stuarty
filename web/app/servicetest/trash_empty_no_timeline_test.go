package servicetest

import (
	"context"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// A project with no timeline holds its generations under its own id (SDD-005 section 5.12.3). Emptying the trash
// stops them as it stops the jobs of a timeline, and keeps the project while one still runs: deleting it would
// delete the job row without a final state (epic 56 review).
func TestTrashEmptyKeepsAProjectWithNoTimelineAndARunningGeneration(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{jobWait: 50 * time.Millisecond})
	ctx := context.Background()

	made, err := testproject.Create(m.as(ui), m.svc, "Busy no timeline")
	if err != nil {
		t.Fatal(err)
	}

	project := made.ProjectID
	row := service.JobRow{
		ID: "job_running_generation", Kind: service.JobImageGenerate, ProjectID: &project, Actor: ui,
		State: service.JobQueued, CreatedMs: 1, UpdatedMs: 1,
	}

	if err := m.store.Write(ctx, func(ctx context.Context, tx service.Tx) error {
		return tx.Jobs().CreateLimited(ctx, row, nil)
	}); err != nil {
		t.Fatal(err)
	}

	if ok, err := m.store.Jobs().Claim(ctx, row.ID, 2); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}

	m.trash(project)

	res, err := m.svc.TrashEmpty(m.browser(), service.TrashEmptyRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if res.Projects != 0 {
		t.Errorf("result = %+v; the project of a running generation was deleted", res)
	}

	if _, err := m.store.Productions().Get(ctx, project); err != nil {
		t.Errorf("the project is gone: %v", err)
	}

	got, err := m.store.Jobs().Get(ctx, row.ID)
	if err != nil {
		t.Fatalf("the job row is gone: %v", err)
	}

	if !got.CancelRequested {
		t.Errorf("the running generation was not asked to stop: %+v", got)
	}
}
