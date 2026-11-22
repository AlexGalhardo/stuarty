package servicetest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// spend records a settled cost of today on the project.
func (e *env) spend(project string, micros int64) {
	e.t.Helper()

	id := fmt.Sprintf("%s_%d", project, micros)

	if _, err := e.store.DB().ExecContext(context.Background(), `INSERT INTO cost_ledger (id, request_id, job_id,
 connection_id, project_id, kind, currency, estimate_micros, actual_micros, state, day, created_ms, settled_ms)
 VALUES (?, 'apr_spent', ?, '', ?, 'text.generate', 'USD', ?, ?, 'settled', ?, 1, 1)`,
		"led_"+id, "job_"+id, project, micros, micros, approval.Day(e.clock.now())); err != nil {
		e.t.Fatal(err)
	}
}

// VV-372 (SDD-015 section 4): a refusal by a workspace ceiling does not tell a member what the workspace spent, since
// that figure adds up the spending of the other users. It keeps the limit and the estimate. An administrator and a
// refusal by the ceiling of the member's own project still say it.
func TestSecurity_T06_ACeilingRefusalHidesWhatTheWorkspaceSpent(t *testing.T) {
	const (
		spentByOther = int64(700_000)
		estimate     = int64(400_000)
		limit        = int64(1_000_000)
	)

	e := newProjectWideEnv(t, &probe{estimate: usd(estimate)})
	member := networkPerson("usr_0000000001", memberScopes, false)
	admin := networkPerson("usr_0000000003", service.AllScopes, true)

	mine, err := testproject.Create(member, e.svc, "Member project")
	if err != nil {
		t.Fatal(err)
	}

	theirs, err := testproject.Create(admin, e.svc, "Admin project")
	if err != nil {
		t.Fatal(err)
	}

	e.spend(theirs.ProjectID, spentByOther)
	e.setCeiling("", 0, limit)

	for _, dry := range []bool{false, true} {
		_, err = e.svc.JobStart(member, service.JobStartRequest{ProjectID: mine.ProjectID, Kind: testKind, DryRun: dry})
		refused := serviceError(t, err)

		if refused.Code != "E_COST_CEILING" {
			t.Fatalf("dry %v: the member got %v, want E_COST_CEILING", dry, err)
		}

		if _, ok := refused.Details["spent_micros"]; ok {
			t.Errorf("dry %v: the details tell a member what the workspace spent: %v", dry, refused.Details)
		}

		if strings.Contains(refused.Message, "0.700000") || strings.Contains(refused.Message, "already spent") {
			t.Errorf("dry %v: the message tells a member what the workspace spent: %s", dry, refused.Message)
		}

		if refused.Details["limit_micros"] != limit || refused.Details["estimate_micros"] != estimate {
			t.Errorf("dry %v: the refusal lost the limit or the estimate: %v", dry, refused.Details)
		}
	}

	_, err = e.svc.JobStart(admin, service.JobStartRequest{ProjectID: theirs.ProjectID, Kind: testKind})
	if got := serviceError(t, err); got.Details["spent_micros"] != spentByOther {
		t.Errorf("an administrator reads the workspace spending as %v, want %d", got.Details["spent_micros"], spentByOther)
	}

	e.setCeiling("", 0, 10*limit)
	e.setCeiling(mine.ProjectID, 0, estimate/2)
	e.spend(mine.ProjectID, 100)

	_, err = e.svc.JobStart(member, service.JobStartRequest{ProjectID: mine.ProjectID, Kind: testKind})
	if got := serviceError(t, err); got.Details["scope"] != "project" || got.Details["spent_micros"] != int64(100) {
		t.Errorf("the refusal by the ceiling of the own project = %v, want its spending", got.Details)
	}
}

// D-132: the figure a member reads for their project is the figure its ceiling counts, the spending of every timeline
// of it on one line.
func TestCostSummary_AMemberReadsWhatTheCeilingCounts(t *testing.T) {
	e := newProjectWideEnv(t, &probe{estimate: usd(1)})
	member := networkPerson("usr_0000000001", memberScopes, false)

	p, err := testproject.CreateWithTimeline(member, e.svc, "Two timelines")
	if err != nil {
		t.Fatal(err)
	}

	second, err := e.svc.ProjectItemAdd(member, service.ProjectItemAddRequest{
		ProjectID: p.ProjectID, Kind: "timeline", Title: "Second",
	})
	if err != nil {
		t.Fatal(err)
	}

	e.spend(p.ProjectID, 100)
	e.spend(second.Item.TimelineID, 20)

	sum, err := e.svc.CostSummary(member, service.CostSummaryRequest{})
	if err != nil {
		t.Fatal(err)
	}

	counted, err := e.store.Approvals().Spent(context.Background(), approval.SpentQuery{
		Day: sum.Day, Currency: "USD", Project: p.ProjectID,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(sum.Today) != 1 || sum.Today[0].ProjectID != p.ProjectID || sum.Today[0].Micros != counted || counted != 120 {
		t.Errorf("the member reads %+v, the ceiling counts %d, want one line of 120", sum.Today, counted)
	}
}

// SDD-015 section 4: a failed read of the owner of a line is an error, never a shorter list that looks complete.
func TestCostSummary_AFailedReadOfTheOwnerIsAnError(t *testing.T) {
	e := newProjectWideEnv(t, &probe{estimate: usd(1)})
	member := networkPerson("usr_0000000001", memberScopes, false)
	admin := networkPerson("usr_0000000003", service.AllScopes, true)

	p, err := testproject.Create(member, e.svc, "Member project")
	if err != nil {
		t.Fatal(err)
	}

	limit := int64(1_000)
	if _, err := e.svc.ApprovalRuleSet(admin, service.ApprovalRuleSetRequest{
		Kind: testKind, ProjectID: p.ProjectID, Currency: "USD", PerItemMicros: &limit,
	}); err != nil {
		t.Fatal(err)
	}

	e.setCeiling(p.ProjectID, 0, limit)
	e.spend(p.ProjectID, 10)

	// The owner of a project is read with its first timeline, so this leaves the ledger readable and that read broken.
	if _, err := e.store.DB().ExecContext(context.Background(),
		`ALTER TABLE timelines RENAME TO timelines_gone`); err != nil {
		t.Fatal(err)
	}

	if res, err := e.svc.CostSummary(member, service.CostSummaryRequest{}); err == nil {
		t.Errorf("CostSummary answered %+v over a failed read", res)
	}

	if res, err := e.svc.ApprovalRuleList(member, service.ApprovalRuleListRequest{}); err == nil {
		t.Errorf("ApprovalRuleList answered %+v over a failed read", res)
	}

	if _, err := e.svc.CostSummary(admin, service.CostSummaryRequest{}); err != nil {
		t.Errorf("an administrator reads no owner, so the summary stands: %v", err)
	}
}
