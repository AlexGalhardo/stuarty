package servicetest

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/store"

	"github.com/AlexGalhardo/vukem/schema"
)

const ceilingsMigration = "20261121184500_ceilings_to_project"

// staleOnLaterTimelines gives the project two more timelines and stores, the way a workspace written before VV-315
// holds them, a ceiling and an approval rule that name the id of the second timeline. It returns the project and
// that id.
func (e *env) staleOnLaterTimelines(ceilingPerItem, rulePerItem int64) (project, second string) {
	e.t.Helper()

	project = e.newProject("p")

	added, err := e.svc.ProjectItemAdd(
		e.as(ui),
		service.ProjectItemAddRequest{ProjectID: project, Kind: "timeline", Title: "Second"},
	)
	if err != nil {
		e.t.Fatal(err)
	}

	second = added.Item.TimelineID

	ctx := context.Background()
	if _, err := e.store.DB().ExecContext(ctx,
		`INSERT INTO cost_ceilings (project_id, currency, per_item_micros, updated_by, updated_ms)
 VALUES (?, 'USD', ?, 'person', 1)`, second, ceilingPerItem); err != nil {
		e.t.Fatal(err)
	}

	if _, err := e.store.DB().ExecContext(ctx,
		`INSERT INTO approval_rules (id, kind, project_id, currency, per_item_micros, created_by, created_ms)
 VALUES ('rul_stale00001', ?, ?, 'USD', ?, 'person', 1)`, testKind, second, rulePerItem); err != nil {
		e.t.Fatal(err)
	}

	return project, second
}

// migrate runs the ceilings migration again on the workspace, as an upgrade of the binary would.
func (e *env) migrate() {
	e.t.Helper()

	ctx := context.Background()
	if _, err := e.store.DB().ExecContext(ctx, `DELETE FROM migrations WHERE id = ?`, ceilingsMigration); err != nil {
		e.t.Fatal(err)
	}

	m, err := schema.NewMigrator(e.store.DB(), store.Migrations())
	if err != nil {
		e.t.Fatal(err)
	}

	if ran, err := m.Up(ctx); err != nil || len(ran) != 1 {
		e.t.Fatalf("Up = %v, %v; want only %s", ran, err, ceilingsMigration)
	}
}

// VV-366, D-132: a ceiling stored with the id of a later timeline limits nothing until it is migrated, and after
// the migration it refuses a job above its limit on every timeline of the project, through the real spend path.
func TestCeiling_MigratedFromALaterTimelineLimitsTheWholeProject(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000)}
	e := newJobEnv(t, p)
	project, second := e.staleOnLaterTimelines(400_000, 1)

	// Before the migration the row matches nothing: this is the silent loss the migration repairs.
	if _, err := e.svc.JobStart(e.as(token), service.JobStartRequest{
		ProjectID: project, TimelineID: second, Kind: testKind,
	}); err != nil {
		t.Fatalf("before the migration the stale ceiling was expected to match nothing, got %v", err)
	}

	e.migrate()

	for name, timeline := range map[string]string{"the second timeline": second, "the first timeline": project} {
		_, err := e.svc.JobStart(e.as(token), service.JobStartRequest{
			ProjectID: project, TimelineID: timeline, Kind: testKind,
		})

		var se *service.Error
		if !errors.As(err, &se) || se.Code != service.CodeCostCeiling {
			t.Errorf("a job above the migrated ceiling on %s = %v, want E_COST_CEILING", name, err)
		}
	}

	if p.calls.Load() != 0 {
		t.Errorf("a job ran %d times above the ceiling", p.calls.Load())
	}
}

// D-132: a rule stored with the id of a later timeline is not widened to the project and never auto-approves:
// the post waits for a person, while a rule written for the project does approve it.
func TestApprovalRule_StoredOnALaterTimelineNeverAutoApproves(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(100_000)}
	e := newJobEnv(t, p)
	project, second := e.staleOnLaterTimelines(1_000_000, 1_000_000)

	e.migrate()

	for name, timeline := range map[string]string{"the second timeline": second, "the first timeline": project} {
		job, err := e.svc.JobStart(e.as(token), service.JobStartRequest{
			ProjectID: project, TimelineID: timeline, Kind: testKind,
		})
		if err != nil {
			t.Fatal(err)
		}

		if job.State != service.JobWaitingApproval || p.calls.Load() != 0 {
			t.Fatalf("a rule on a later timeline id approved a job on %s: %+v, %d calls", name, job, p.calls.Load())
		}
	}

	limit := int64(1_000_000)
	if _, err := e.svc.ApprovalRuleSet(e.asPerson(), service.ApprovalRuleSetRequest{
		Kind: testKind, ProjectID: project, Currency: "USD", PerItemMicros: &limit,
	}); err != nil {
		t.Fatal(err)
	}

	job, err := e.svc.JobStart(e.as(token), service.JobStartRequest{
		ProjectID: project, TimelineID: second, Kind: testKind,
	})
	if err != nil || job.State == service.JobWaitingApproval {
		t.Fatalf("a rule written for the project did not cover its second timeline: %+v, %v", job, err)
	}
}

// VV-367: the summary reports the spending the ceiling sums, by project: the spending of a second timeline is on
// the line of its project, not on a line named by the id of the timeline.
func TestCostSummary_GroupsTheSpendingOfEveryTimelineByProject(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(300_000)}
	e := newJobEnv(t, p)
	project, second := e.staleOnLaterTimelines(1_000_000, 1_000_000)

	for _, timeline := range []string{project, second} {
		job, err := e.svc.JobStart(e.as(token), service.JobStartRequest{
			ProjectID: project, TimelineID: timeline, Kind: testKind,
		})
		if err != nil {
			t.Fatal(err)
		}

		if err := e.approve(job.ApprovalID); err != nil {
			t.Fatal(err)
		}
	}

	sum, err := e.svc.CostSummary(e.as(token), service.CostSummaryRequest{})
	if err != nil {
		t.Fatal(err)
	}

	want := []service.CostLine{
		{Scope: "project", ProjectID: project, Currency: "USD", Micros: 600_000},
		{Scope: "workspace", Currency: "USD", Micros: 600_000},
	}

	if len(sum.Today) != 2 || sum.Today[0] != want[0] || sum.Today[1] != want[1] {
		t.Errorf("today = %+v, want %+v", sum.Today, want)
	}

	if len(sum.Days) != 1 || sum.Days[0].ProjectID != project || sum.Days[0].Micros != 600_000 {
		t.Errorf("days = %+v, want one line of %s at 600000", sum.Days, project)
	}
}
