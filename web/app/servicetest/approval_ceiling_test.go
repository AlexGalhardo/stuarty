package servicetest

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
)

func (e *env) setCeiling(project string, perItem, perDay int64) {
	e.t.Helper()

	req := service.CostCeilingSetRequest{ProjectID: project, Currency: "USD"}
	if perItem > 0 {
		req.PerItemMicros = &perItem
	}

	if perDay > 0 {
		req.PerDayMicros = &perDay
	}

	if _, err := e.svc.CostCeilingSet(e.asPerson(), req); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) approve(id string) error {
	_, err := e.svc.ApprovalDecide(e.asPerson(), service.ApprovalDecideRequest{ID: id, Decision: "approve"})

	return err
}

// nothingWritten checks that a refused submission left no request, no job and no spending.
func (e *env) nothingWritten(project string, p *probe) {
	e.t.Helper()

	if list, err := e.svc.ApprovalList(
		e.as(token),
		service.ApprovalListRequest{State: "all"},
	); err != nil ||
		list.Total != 0 {
		e.t.Errorf("a refused submission left requests: %+v, %v", list, err)
	}

	if jobs, err := e.svc.JobList(e.as(token), service.JobListRequest{ProjectID: project, State: "all"}); err != nil ||
		len(jobs.Jobs) != 0 {
		e.t.Errorf("a refused submission left jobs: %+v, %v", jobs, err)
	}

	if e.spentToday() != 0 || p.calls.Load() != 0 {
		e.t.Errorf("a refused submission spent %d and made %d calls", e.spentToday(), p.calls.Load())
	}
}

// Acceptance criterion 4 of epic 05: an action whose estimate exceeds a ceiling is refused with an
// actionable error and nothing is spent.
func TestCeiling_PerItem(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000)}
	e := newJobEnv(t, p)
	project := e.newProject("p")
	e.setCeiling("", 400_000, 0)

	_, err := e.submit(e.as(token), project)

	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.CodeCostCeiling || se.HTTPStatus() != http.StatusUnprocessableEntity {
		t.Fatalf("submitting over the limit per item = %v, want E_COST_CEILING (422)", err)
	}

	if se.Hint == "" || !strings.Contains(se.Message, "0.400000 USD") || !strings.Contains(se.Message, "0.500000 USD") {
		t.Errorf("the refusal is not actionable: %q / %q", se.Message, se.Hint)
	}

	for key, want := range map[string]any{
		"scope": "workspace", "reason": "per_item", "limit_micros": int64(400_000), "estimate_micros": int64(500_000),
		"spent_micros": int64(0), "currency": "USD",
	} {
		if se.Details[key] != want {
			t.Errorf("details[%s] = %v, want %v", key, se.Details[key], want)
		}
	}

	e.nothingWritten(project, p)

	// At the limit it is allowed.
	p.estimate = usd(400_000)

	if job, err := e.submit(e.as(token), project); err != nil || job.State != service.JobWaitingApproval {
		t.Errorf("an estimate at the limit = %+v, %v", job, err)
	}
}

func TestCeiling_PerDay(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(400_000)}
	e := newJobEnv(t, p)
	project := e.newProject("p")
	e.setCeiling("", 0, 1_000_000)

	// Three requests wait, as an agent can ask for many things before anyone answers.
	var ids []string

	for range 3 {
		job, err := e.submit(e.as(token), project)
		if err != nil {
			t.Fatal(err)
		}

		ids = append(ids, job.ApprovalID)
	}

	for _, id := range ids[:2] {
		if err := e.approve(id); err != nil {
			t.Fatal(err)
		}
	}

	if e.spentToday() != 800_000 {
		t.Fatalf("spent today = %d, want 800000", e.spentToday())
	}

	// The third no longer fits the day: approving it is refused, nothing is spent and it stays pending.
	err := e.approve(ids[2])
	if code(err) != service.CodeCostCeiling {
		t.Fatalf("approving over the daily ceiling = %v, want E_COST_CEILING", err)
	}

	if e.spentToday() != 800_000 || p.calls.Load() != 2 {
		t.Errorf("a refused approval spent or ran: %d spent, %d calls", e.spentToday(), p.calls.Load())
	}

	if req, _ := e.svc.ApprovalGet(
		e.as(token),
		service.ApprovalRef{ID: ids[2]},
	); req == nil ||
		req.State != approval.StatePending {
		t.Errorf("the refused request = %+v, want it still pending", req)
	}

	// A new submission is refused up front.
	if _, err := e.submit(e.as(token), project); code(err) != service.CodeCostCeiling {
		t.Errorf("submitting with the day used = %v, want E_COST_CEILING", err)
	}

	// The person raises the ceiling; then the same request can be approved.
	e.setCeiling("", 0, 2_000_000)

	if err := e.approve(ids[2]); err != nil {
		t.Fatalf("approving after the ceiling was raised: %v", err)
	}

	// And the next day starts empty.
	e.clock.advance(24 * time.Hour)

	if _, err := e.submit(e.as(token), project); err != nil {
		t.Errorf("submitting on the next day: %v", err)
	}
}

// The day counts the real costs: a job that cost less than its estimate leaves room.
func TestCeiling_PerDayCountsTheRealCost(t *testing.T) {
	t.Parallel()

	cheap := usd(100_000)
	p := &probe{estimate: usd(500_000), cost: &cheap}
	e := newJobEnv(t, p)
	project := e.newProject("p")
	e.setCeiling("", 0, 600_000)

	first, err := e.submit(e.as(token), project)
	if err != nil {
		t.Fatal(err)
	}

	if err := e.approve(first.ApprovalID); err != nil {
		t.Fatal(err)
	}

	if e.spentToday() != 100_000 {
		t.Fatalf("spent today = %d, want the real 100000", e.spentToday())
	}

	// 100k real + 500k estimate = 600k fits; by estimates it would have been 1M.
	if _, err := e.submit(e.as(token), project); err != nil {
		t.Errorf("a second request that fits the real spending: %v", err)
	}
}

func TestCeiling_PerProject(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(600_000)}
	e := newJobEnv(t, p)
	limited, free := e.newProject("limited"), e.newProject("free")
	e.setCeiling(limited, 0, 1_000_000)

	one, err := e.submit(e.as(token), limited)
	if err != nil {
		t.Fatal(err)
	}

	if err := e.approve(one.ApprovalID); err != nil {
		t.Fatal(err)
	}

	// 600k spent on the limited project: another 600k there is refused, with the project named as the scope.
	_, err = e.submit(e.as(token), limited)

	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.CodeCostCeiling || se.Details["scope"] != "project" ||
		se.Details["reason"] != "per_day" || se.Details["spent_micros"] != int64(600_000) {
		t.Fatalf("the second request on the limited project = %v, want a project ceiling refusal", err)
	}

	// The other project has no ceiling of its own and is not bound by that one.
	if _, err := e.submit(e.as(token), free); err != nil {
		t.Errorf("a request on the other project: %v", err)
	}
}

// A rule cannot lift a ceiling: the ceilings are read before any rule.
func TestCeiling_ARuleDoesNotBypassIt(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000)}
	e := newJobEnv(t, p)
	project := e.newProject("p")
	e.setCeiling("", 100_000, 0)

	perItem := int64(900_000)

	if _, err := e.svc.ApprovalRuleSet(e.asPerson(), service.ApprovalRuleSetRequest{
		Kind: testKind, Currency: "USD", PerItemMicros: &perItem,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.submit(e.as(token), project); code(err) != service.CodeCostCeiling {
		t.Fatalf("a rule that covers the action = %v, want the ceiling to refuse it", err)
	}

	e.nothingWritten(project, p)
}

func TestCostSummaryShowsWhatWasSpent(t *testing.T) {
	t.Parallel()

	spent := usd(300_000)
	p := &probe{estimate: usd(500_000), conn: "con_0000000001", cost: &spent}
	e := newJobEnv(t, p)
	e.addConnection("con_0000000001")
	project := e.newProject("p")

	perDay := int64(5_000_000)
	e.setCeiling("", 0, perDay)

	for range 2 {
		job, err := e.submit(e.as(token), project)
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

	if len(sum.Ceilings) != 1 || sum.Ceilings[0].PerDay == nil || *sum.Ceilings[0].PerDay != perDay {
		t.Errorf("ceilings = %+v", sum.Ceilings)
	}

	want := []service.CostLine{
		{Scope: "project", ProjectID: project, Currency: "USD", Micros: 600_000},
		{Scope: "workspace", Currency: "USD", Micros: 600_000},
	}

	if len(sum.Today) != 2 || sum.Today[0] != want[0] || sum.Today[1] != want[1] {
		t.Errorf("today = %+v, want %+v", sum.Today, want)
	}

	wantDay := approval.Total{
		Day: approval.Day(
			e.clock.now(),
		),
		ConnectionID: "con_0000000001",
		ProjectID:    project,
		Currency:     "USD",
		Micros:       600_000,
	}

	if len(sum.Days) != 1 || sum.Days[0] != wantDay {
		t.Errorf("days = %+v, want %+v", sum.Days, wantDay)
	}
}

// VV-315: the ceiling of a project holds for every timeline of it and the spending of all of them counts together,
// while the request still carries the timeline its job is kept on.
func TestCeiling_ProjectCoversEveryTimeline(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(300_000)}
	e := newJobEnv(t, p)
	project := e.newProject("p")

	added, err := e.svc.ProjectItemAdd(
		e.as(ui),
		service.ProjectItemAddRequest{ProjectID: project, Kind: "timeline", Title: "Second"},
	)
	if err != nil {
		t.Fatal(err)
	}

	second := added.Item.TimelineID
	e.setCeiling(project, 0, 500_000)

	first, err := e.svc.JobStart(e.as(token), service.JobStartRequest{ProjectID: project, TimelineID: second, Kind: testKind})
	if err != nil {
		t.Fatal(err)
	}

	if err := e.approve(first.ApprovalID); err != nil {
		t.Fatal(err)
	}

	req, err := e.svc.ApprovalGet(e.as(token), service.ApprovalRef{ID: first.ApprovalID})
	if err != nil || req.ProjectID == nil || *req.ProjectID != second {
		t.Fatalf("the request carries %+v, %v; want the timeline %s", req, err, second)
	}

	_, err = e.svc.JobStart(e.as(token), service.JobStartRequest{ProjectID: project, Kind: testKind, TimelineID: project})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.CodeCostCeiling {
		t.Fatalf("over the ceiling of the project on its other timeline = %v, want E_COST_CEILING", err)
	}
}

// A rule or a ceiling is written for a project: an id of a later timeline would never match, so it is refused.
func TestCeilingAndRule_RefuseATimelineID(t *testing.T) {
	t.Parallel()

	e := newJobEnv(t, &probe{estimate: usd(1)})
	project := e.newProject("p")

	added, err := e.svc.ProjectItemAdd(
		e.as(ui),
		service.ProjectItemAddRequest{ProjectID: project, Kind: "timeline", Title: "Second"},
	)
	if err != nil {
		t.Fatal(err)
	}

	limit := int64(1000)

	_, err = e.svc.CostCeilingSet(e.asPerson(), service.CostCeilingSetRequest{
		ProjectID: added.Item.TimelineID, Currency: "USD", PerItemMicros: &limit,
	})
	if code(err) != "E_VALIDATION" {
		t.Errorf("a ceiling on a timeline id = %v, want E_VALIDATION", err)
	}

	_, err = e.svc.ApprovalRuleSet(e.asPerson(), service.ApprovalRuleSetRequest{
		Kind: testKind, ProjectID: added.Item.TimelineID, Currency: "USD", PerItemMicros: &limit,
	})
	if code(err) != "E_VALIDATION" {
		t.Errorf("a rule on a timeline id = %v, want E_VALIDATION", err)
	}

	if _, err := e.svc.CostCeilingSet(e.asPerson(), service.CostCeilingSetRequest{
		ProjectID: project, Currency: "USD", PerItemMicros: &limit,
	}); err != nil {
		t.Errorf("a ceiling on the project = %v", err)
	}
}
