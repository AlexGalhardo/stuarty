package servicetest

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/testproject"
	"github.com/AlexGalhardo/stuarty/web/app/jobs"

	"github.com/AlexGalhardo/vukem/queue"
)

// newProjectWideEnv is newJobEnv with the probe registered as work of the whole project, which is the only kind of
// job a project with no timeline can hold.
func newProjectWideEnv(t *testing.T, p *probe) *env {
	t.Helper()

	k := p.kind()
	k.ProjectWide = true

	lazy := &lazyRunner{}

	e := newEnvWith(t, func(_ *env, o *service.Options) {
		q, err := jobs.NewQueue(queue.Sync{}, lazy)
		if err != nil {
			t.Fatal(err)
		}

		o.Enqueue = jobs.Dispatcher{Queue: q}
		o.JobKinds = []service.ApprovableKind{k}
	})

	r, ok := service.RunnerOf(e.svc)
	if !ok {
		t.Fatal("the service has no job runner")
	}

	lazy.mu.Lock()
	lazy.r = r
	lazy.mu.Unlock()

	return e
}

// timelineCarrying stores, in a project, a timeline item whose id is the id of another project (a state a bundle could
// write before its import refused it).
func (e *env) timelineCarrying(holder, id string) {
	e.t.Helper()

	if _, err := e.store.DB().ExecContext(context.Background(), `INSERT INTO project_items
 (id, project_id, kind, title, position, state, ref, created_ms, updated_ms, created_by)
 VALUES ('pit_carried001', ?, 'timeline', 'carried', 9, 'active', ?, 1, 1, 'u')`, holder, id); err != nil {
		e.t.Fatal(err)
	}
}

// An id that names a project is that project: a timeline of another project that carries the same id does not take
// the ceiling away from it, and a rule of the other project does not approve its work (D-132).
func TestCeiling_AProjectKeepsItsLimitsWhenATimelineCarriesItsID(t *testing.T) {
	p := &probe{estimate: usd(100)}
	e := newProjectWideEnv(t, p)

	victim, err := testproject.Create(e.as(ui), e.svc, "Victim project")
	if err != nil {
		t.Fatal(err)
	}

	holder := e.newProject("holder")
	e.timelineCarrying(holder, victim.ProjectID)
	e.setCeiling(victim.ProjectID, 1, 0)

	_, err = e.svc.JobStart(e.as(token), service.JobStartRequest{ProjectID: victim.ProjectID, Kind: testKind})
	if code(err) != "E_COST_CEILING" {
		t.Fatalf("over the ceiling of the project = %v, want E_COST_CEILING", err)
	}

	e.setCeiling(victim.ProjectID, 1_000, 0)

	limit := int64(1_000)
	if _, err := e.svc.ApprovalRuleSet(e.asPerson(), service.ApprovalRuleSetRequest{
		Kind: testKind, ProjectID: holder, Currency: "USD", PerItemMicros: &limit,
	}); err != nil {
		t.Fatal(err)
	}

	job, err := e.svc.JobStart(e.as(token), service.JobStartRequest{ProjectID: victim.ProjectID, Kind: testKind})
	if err != nil || job.State != service.JobWaitingApproval || p.calls.Load() != 0 {
		t.Fatalf("a rule of another project decided this one: %+v, %v, %d calls", job, err, p.calls.Load())
	}
}

// VV-315: a post is kept on the timeline of its export, and the queue of its project still shows it; the id of the
// timeline lists that timeline alone.
func TestApprovalList_AProjectListsTheRequestsOfEveryTimeline(t *testing.T) {
	p := newPubEnv(t)
	_, second := p.addSecondTimeline()
	exp := p.seedExport(second, "job_0000000974", ".mp4", []byte("fake mp4 bytes"))

	job, err := p.pub.PublishNow(p.as(token), service.PublishNowRequest{ProjectID: p.project, PublishArgs: p.args(exp)})
	if err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]string{"the project": p.project, "the timeline": second} {
		list, err := p.svc.ApprovalList(p.as(token), service.ApprovalListRequest{ProjectID: id})
		if err != nil {
			t.Fatal(err)
		}

		if list.Total != 1 || list.Requests[0].ID != job.ApprovalID {
			t.Errorf("the queue of %s = %+v, want the post %s", name, list.Requests, job.ApprovalID)
		}
	}

	other := p.newProject("other")

	list, err := p.svc.ApprovalList(p.as(token), service.ApprovalListRequest{ProjectID: other})
	if err != nil || list.Total != 0 {
		t.Errorf("the queue of another project = %+v, %v; want it empty", list, err)
	}
}

// VV-367: what a ceiling counts for a project is what the summary reports for it, with history written before the
// migration, a timeline in the trash, a project with no timeline, an id no project holds, and a timeline that carries
// the id of another project.
func TestCostSummary_EqualsWhatTheCeilingCounts(t *testing.T) {
	e := newProjectWideEnv(t, &probe{estimate: usd(1)})
	ctx := context.Background()

	project, second := e.staleOnLaterTimelines(1_000_000, 1)

	third, err := e.svc.ProjectItemAdd(
		e.as(ui),
		service.ProjectItemAddRequest{ProjectID: project, Kind: "timeline", Title: "Third"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectItemTrash(e.as(ui), service.ProjectItemTrashRequest{
		ProjectID: project, ItemID: third.Item.ID,
	}); err != nil {
		t.Fatal(err)
	}

	bare, err := testproject.Create(e.as(ui), e.svc, "Bare project")
	if err != nil {
		t.Fatal(err)
	}

	holder := e.newProject("holder")
	e.timelineCarrying(holder, bare.ProjectID)

	day := approval.Day(e.clock.now())
	entries := []struct {
		id, on string
		micros int64
	}{
		{"led_review0001", project, 100},
		{"led_review0002", second, 40},
		{"led_review0003", third.Item.TimelineID, 7},
		{"led_review0004", bare.ProjectID, 11},
		{"led_review0005", holder, 5},
		{"led_review0006", "prj_gone000001", 3},
	}

	for _, l := range entries {
		if _, err := e.store.DB().ExecContext(ctx, `INSERT INTO cost_ledger (id, request_id, job_id, connection_id,
 project_id, kind, currency, estimate_micros, actual_micros, state, day, created_ms, settled_ms)
 VALUES (?, 'apr_h', 'job_h', '', ?, 'text.generate', 'USD', ?, ?, 'settled', ?, 1, 1)`,
			l.id, l.on, l.micros, l.micros, day); err != nil {
			t.Fatal(err)
		}
	}

	sum, err := e.svc.CostSummary(e.as(token), service.CostSummaryRequest{})
	if err != nil {
		t.Fatal(err)
	}

	lines := map[string]int64{}

	for _, l := range sum.Today {
		if l.Scope == "project" {
			lines[l.ProjectID] = l.Micros
		}
	}

	want := map[string]int64{project: 147, bare.ProjectID: 11, holder: 5, "prj_gone000001": 3}
	if len(lines) != len(want) {
		t.Errorf("the summary has the lines %v, want %v", lines, want)
	}

	for id, micros := range want {
		spent, err := e.store.Approvals().Spent(ctx, approval.SpentQuery{Day: day, Currency: "USD", Project: id})
		if err != nil {
			t.Fatal(err)
		}

		if spent != micros || lines[id] != micros {
			t.Errorf("%s: the ceiling counts %d and the summary reports %d, want %d", id, spent, lines[id], micros)
		}
	}
}
