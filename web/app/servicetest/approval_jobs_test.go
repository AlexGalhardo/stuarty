package servicetest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/web/app/jobs"

	"github.com/AlexGalhardo/vukem/queue"
)

const testKind = "test.generate"

// probe is the test job kind: its Run is the call that must not happen before a person approves.
// It stands for the outbound call and the spending of a real kind, which later epics register.
type probe struct {
	calls    atomic.Int32
	estimate core.Money
	conn     string
	// cost is what Run reports as spent; nil reports nothing.
	cost *core.Money
	fail error
	// summary replaces the summary Prepare gives when it is set.
	summary string
}

func (p *probe) kind() service.ApprovableKind {
	return service.ApprovableKind{
		Name: testKind,
		Prepare: func(_ context.Context, _ service.JobStartRequest) (service.Prepared, error) {
			summary := "Generate an image for the intro"
			if p.summary != "" {
				summary = p.summary
			}

			return service.Prepared{
				Summary: summary, ConnectionID: p.conn, Estimate: p.estimate,
				Input: json.RawMessage(`{"prompt":"a cat"}`),
			}, nil
		},
		Run: func(_ context.Context, _ service.JobRun) (service.JobOutcome, error) {
			p.calls.Add(1)

			if p.fail != nil {
				return service.JobOutcome{Cost: p.cost}, p.fail
			}

			return service.JobOutcome{Result: json.RawMessage(`{"ok":true}`), Cost: p.cost}, nil
		},
	}
}

// newJobEnv is an environment whose jobs run inline after the commit, with the probe registered.
func newJobEnv(tb testing.TB, p *probe) *env {
	tb.Helper()

	lazy := &lazyRunner{}

	e := newEnvWith(tb, func(_ *env, o *service.Options) {
		q, err := jobs.NewQueue(queue.Sync{}, lazy)
		if err != nil {
			tb.Fatal(err)
		}

		o.Enqueue = jobs.Dispatcher{Queue: q}
		o.JobKinds = []service.ApprovableKind{p.kind()}
	})

	r, ok := service.RunnerOf(e.svc)
	if !ok {
		tb.Fatal("the service has no job runner")
	}

	lazy.mu.Lock()
	lazy.r = r
	lazy.mu.Unlock()

	return e
}

func (e *env) submit(ctx context.Context, project string) (*service.Job, error) {
	return e.svc.JobStart(ctx, service.JobStartRequest{ProjectID: project, Kind: testKind})
}

func (e *env) spentToday() int64 {
	e.t.Helper()

	n, err := e.store.Approvals().Spent(context.Background(), approval.SpentQuery{
		Day: approval.Day(e.clock.now()), Currency: "USD",
	})
	if err != nil {
		e.t.Fatal(err)
	}

	return n
}

func usd(micros int64) core.Money { return core.Money{Micros: micros, Currency: "USD"} }

// Acceptance criterion 1 of epic 05: a job of an approvable kind submitted by an agent stays in
// waiting_approval and makes no call until a person approves it.
func TestJobApproval_WaitingJobDoesNotRun(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000)}
	e := newJobEnv(t, p)
	project := e.newProject("p")

	job, err := e.submit(e.as(token), project)
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobWaitingApproval || job.ApprovalID == "" || job.StartedAt != nil || job.Kind != testKind {
		t.Fatalf("job = %+v, want waiting_approval with an approval id", job)
	}

	if !strings.Contains(job.Text, "waiting for approval "+job.ApprovalID) ||
		!strings.Contains(job.Text, "you cannot") {
		t.Errorf("the text does not tell the agent it waits and cannot decide: %q", job.Text)
	}

	if p.calls.Load() != 0 {
		t.Fatal("the job ran before anyone approved it")
	}

	req, err := e.svc.ApprovalGet(e.as(token), service.ApprovalRef{ID: job.ApprovalID})
	if err != nil || req.State != approval.StatePending || req.Actor != token || req.Kind != testKind ||
		req.Estimate != usd(500_000) || req.JobID == nil || *req.JobID != job.JobID || req.Summary == "" ||
		req.ProjectID == nil || *req.ProjectID != project {
		t.Fatalf("request = %+v, %v", req, err)
	}

	// It is read like any job, and counts as active.
	got, err := e.svc.JobGet(e.as(token), service.JobGetRequest{JobID: job.JobID})
	if err != nil || got.State != service.JobWaitingApproval {
		t.Fatalf("job_get = %+v, %v", got, err)
	}

	list, err := e.svc.JobList(e.as(token), service.JobListRequest{ProjectID: project, State: "active"})
	if err != nil || len(list.Jobs) != 1 {
		t.Fatalf("job_list active = %+v, %v", list, err)
	}

	if e.spentToday() != 0 {
		t.Error("something was reserved before approval")
	}

	if evs := e.eventsOf(service.EventApprovalRequested); len(evs) != 1 || evs[0].Approval.ID != job.ApprovalID {
		t.Errorf("approval.requested events = %+v", evs)
	}

	if n := len(e.audits("approval.request")); n != 1 {
		t.Errorf("request audit rows = %d, want 1", n)
	}
}

func TestJobApproval_Approve(t *testing.T) {
	t.Parallel()

	spent := usd(420_000)
	p := &probe{estimate: usd(500_000), cost: &spent}
	e := newJobEnv(t, p)
	project := e.newProject("p")

	job, err := e.submit(e.as(token), project)
	if err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.ApprovalDecide(
		e.asPerson(),
		service.ApprovalDecideRequest{ID: job.ApprovalID, Decision: "approve"},
	)
	if err != nil || got.State != approval.StateApproved || got.DecidedBy == nil || *got.DecidedBy != ui {
		t.Fatalf("approve = %+v, %v", got, err)
	}

	// The queue ran it inline after the commit.
	done, err := e.svc.JobGet(e.as(token), service.JobGetRequest{JobID: job.JobID})
	if err != nil || done.State != service.JobSucceeded || p.calls.Load() != 1 {
		t.Fatalf("job after approval = %+v (%d calls), %v", done, p.calls.Load(), err)
	}

	// The real cost, not the estimate, is what the day counts.
	if n := e.spentToday(); n != 420_000 {
		t.Errorf("spent today = %d, want the real cost 420000", n)
	}

	if evs := e.eventsOf(
		service.EventApprovalDecided,
	); len(evs) != 1 ||
		evs[0].Approval.State != approval.StateApproved {
		t.Errorf("approval.decided events = %+v", evs)
	}

	if n := len(e.audits("approval.approve")); n != 1 {
		t.Errorf("approve audit rows = %d, want 1", n)
	}
}

// A job that succeeds and reports no cost is settled at its estimate: the day never counts less than
// what may have been spent.
func TestJobApproval_NoReportedCostSettlesAtTheEstimate(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000)}
	e := newJobEnv(t, p)

	job, err := e.submit(e.as(token), e.newProject("p"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ApprovalDecide(
		e.asPerson(),
		service.ApprovalDecideRequest{ID: job.ApprovalID, Decision: "approve"},
	); err != nil {
		t.Fatal(err)
	}

	if n := e.spentToday(); n != 500_000 {
		t.Errorf("spent today = %d, want the estimate 500000", n)
	}
}

func TestJobApproval_FailedJobGivesTheReservationBack(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		cost *core.Money
		want int64
	}{
		"nothing spent":   {nil, 0},
		"spent then died": {&core.Money{Micros: 120_000, Currency: "USD"}, 120_000},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := &probe{estimate: usd(500_000), cost: tc.cost, fail: errors.New("the provider said no")}
			e := newJobEnv(t, p)

			job, err := e.submit(e.as(token), e.newProject("p"))
			if err != nil {
				t.Fatal(err)
			}

			if _, err := e.svc.ApprovalDecide(
				e.asPerson(),
				service.ApprovalDecideRequest{ID: job.ApprovalID, Decision: "approve"},
			); err != nil {
				t.Fatal(err)
			}

			done, err := e.svc.JobGet(e.as(token), service.JobGetRequest{JobID: job.JobID})
			if err != nil || done.State != service.JobFailed {
				t.Fatalf("job = %+v, %v", done, err)
			}

			if n := e.spentToday(); n != tc.want {
				t.Errorf("spent today = %d, want %d", n, tc.want)
			}
		})
	}
}

func TestJobApproval_Reject(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000)}
	e := newJobEnv(t, p)
	project := e.newProject("p")

	job, err := e.submit(e.as(token), project)
	if err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.ApprovalDecide(e.asPerson(), service.ApprovalDecideRequest{
		ID: job.ApprovalID, Decision: "reject", Reason: "not this one",
	})
	if err != nil || got.State != approval.StateRejected || got.Reason != "not this one" {
		t.Fatalf("reject = %+v, %v", got, err)
	}

	done, err := e.svc.JobGet(e.as(token), service.JobGetRequest{JobID: job.JobID})
	if err != nil || done.State != service.JobCancelled || done.Error == nil ||
		done.Error.Code != service.CodeCancelled ||
		!strings.Contains(done.Error.Message, "not this one") {
		t.Fatalf("job after rejection = %+v, %v", done, err)
	}

	if p.calls.Load() != 0 || e.spentToday() != 0 {
		t.Error("a rejected job ran or spent")
	}

	// Rejecting does not leave an approvable request behind.
	if _, err := e.svc.ApprovalDecide(
		e.asPerson(),
		service.ApprovalDecideRequest{ID: job.ApprovalID, Decision: "approve"},
	); code(
		err,
	) != "E_INVALID_STATE" {
		t.Errorf("approving after the rejection = %v, want E_INVALID_STATE", err)
	}
}

// Cancelling a waiting job rejects its request.
func TestJobApproval_Cancel(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000)}
	e := newJobEnv(t, p)
	project := e.newProject("p")

	job, err := e.submit(e.as(token), project)
	if err != nil {
		t.Fatal(err)
	}

	cancelled, err := e.svc.JobCancel(e.as(token), service.JobCancelRequest{JobID: job.JobID})
	if err != nil || cancelled.State != service.JobCancelled {
		t.Fatalf("cancel = %+v, %v", cancelled, err)
	}

	req, err := e.svc.ApprovalGet(e.as(token), service.ApprovalRef{ID: job.ApprovalID})
	if err != nil || req.State != approval.StateRejected || req.DecidedBy == nil || *req.DecidedBy != token ||
		!strings.Contains(req.Reason, "cancelled by "+token) {
		t.Fatalf("request after the cancel = %+v, %v", req, err)
	}

	if _, err := e.svc.ApprovalDecide(
		e.asPerson(),
		service.ApprovalDecideRequest{ID: job.ApprovalID, Decision: "approve"},
	); code(
		err,
	) != "E_INVALID_STATE" {
		t.Errorf("approving a cancelled job = %v, want E_INVALID_STATE", err)
	}

	if p.calls.Load() != 0 {
		t.Error("a cancelled job ran")
	}

	// A second cancel changes nothing.
	if again, err := e.svc.JobCancel(
		e.as(token),
		service.JobCancelRequest{JobID: job.JobID},
	); err != nil ||
		again.State != service.JobCancelled {
		t.Errorf("second cancel = %+v, %v", again, err)
	}

	if n := len(e.audits("approval.cancel")); n != 1 {
		t.Errorf("cancel audit rows = %d, want 1", n)
	}
}

func TestJobApproval_TrashingTheProjectCancelsTheWaitingJob(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000)}
	e := newJobEnv(t, p)
	project := e.newProject("p")

	job, err := e.submit(e.as(token), project)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectTrash(e.as(ui), service.ProjectTrashRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	done, err := e.svc.JobGet(e.as(token), service.JobGetRequest{JobID: job.JobID})
	if err != nil || done.State != service.JobCancelled {
		t.Fatalf("job after the trash = %+v, %v", done, err)
	}

	req, err := e.svc.ApprovalGet(e.as(token), service.ApprovalRef{ID: job.ApprovalID})
	if err != nil || req.State != approval.StateRejected {
		t.Fatalf("request after the trash = %+v, %v", req, err)
	}
}

func TestJobApproval_Expire(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000)}
	e := newJobEnv(t, p)

	job, err := e.submit(e.as(token), e.newProject("p"))
	if err != nil {
		t.Fatal(err)
	}

	e.clock.advance(approval.ExpireAfter + 1)

	if _, err := e.svc.ApprovalList(e.as(token), service.ApprovalListRequest{}); err != nil {
		t.Fatal(err)
	}

	done, err := e.svc.JobGet(e.as(token), service.JobGetRequest{JobID: job.JobID})
	if err != nil || done.State != service.JobCancelled || done.Error == nil ||
		done.Error.Code != service.CodeApprovalExpired {
		t.Fatalf("job after the time = %+v, %v", done, err)
	}

	if p.calls.Load() != 0 {
		t.Error("an expired job ran")
	}
}

// Acceptance criterion 3 of epic 05: a rule written from a privileged session approves a matching
// action, and the audit log names the rule.
func TestJobApproval_RuleAutoApproves(t *testing.T) {
	t.Parallel()

	spent := usd(300_000)
	p := &probe{estimate: usd(500_000), conn: "con_0000000001", cost: &spent}
	e := newJobEnv(t, p)
	e.addConnection("con_0000000001")
	project := e.newProject("p")

	perItem := int64(500_000)

	rule, err := e.svc.ApprovalRuleSet(e.asPerson(), service.ApprovalRuleSetRequest{
		Kind: testKind, ConnectionID: "con_0000000001", ProjectID: project, Currency: "USD", PerItemMicros: &perItem,
	})
	if err != nil {
		t.Fatal(err)
	}

	job, err := e.submit(e.as(token), project)
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobSucceeded || p.calls.Load() != 1 {
		t.Fatalf("job = %+v (%d calls), want it run at once by the rule", job, p.calls.Load())
	}

	req, err := e.svc.ApprovalGet(e.as(token), service.ApprovalRef{ID: job.ApprovalID})
	if err != nil || req.State != approval.StateApproved || req.RuleID == nil || *req.RuleID != rule.ID ||
		req.DecidedBy == nil || *req.DecidedBy != "rule:"+rule.ID {
		t.Fatalf("request = %+v, %v", req, err)
	}

	audit := e.audits("approval.auto")
	if len(audit) != 1 || audit[0].SubjectID != job.ApprovalID || audit[0].Actor != token ||
		!strings.Contains(string(audit[0].Details), rule.ID) {
		t.Fatalf("audit = %+v, want one row naming the rule %s", audit, rule.ID)
	}

	if n := e.spentToday(); n != 300_000 {
		t.Errorf("spent today = %d, want 300000", n)
	}

	if len(e.eventsOf(service.EventApprovalRequested)) != 1 || len(e.eventsOf(service.EventApprovalDecided)) != 1 {
		t.Error("an automatic approval must publish both events")
	}
}

func TestJobApproval_RuleDoesNotCoverWhatItDoesNotMatch(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000), conn: "con_0000000001"}
	e := newJobEnv(t, p)
	e.addConnection("con_0000000001")
	e.addConnection("con_0000000002")
	perItem := int64(500_000)

	// A rule for another connection, one over its own limit, and a disabled one.
	off := false

	for _, r := range []service.ApprovalRuleSetRequest{
		{Kind: testKind, ConnectionID: "con_0000000002", Currency: "USD", PerItemMicros: &perItem},
		{Kind: testKind, Currency: "USD", PerItemMicros: new(int64(499_999))},
		{Kind: testKind, Currency: "USD", PerItemMicros: &perItem, Enabled: &off},
		{Kind: testKind}, // no limit per item: never covers a costly action
	} {
		if _, err := e.svc.ApprovalRuleSet(e.asPerson(), r); err != nil {
			t.Fatal(err)
		}
	}

	job, err := e.submit(e.as(token), e.newProject("p"))
	if err != nil || job.State != service.JobWaitingApproval || p.calls.Load() != 0 {
		t.Fatalf("job = %+v, %v (%d calls): no rule covers it, a person must decide", job, err, p.calls.Load())
	}
}

func TestJobApproval_ConnectionRemovedBeforeApproval(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(1), conn: "con_0000000001"}
	e := newJobEnv(t, p)
	e.addConnection("con_0000000001")

	job, err := e.submit(e.as(token), e.newProject("p"))
	if err != nil {
		t.Fatal(err)
	}

	if err := e.store.Connections().Delete(context.Background(), "con_0000000001"); err != nil {
		t.Fatal(err)
	}

	_, err = e.svc.ApprovalDecide(e.asPerson(), service.ApprovalDecideRequest{ID: job.ApprovalID, Decision: "approve"})
	if code(err) != service.CodeConnectionNotFound {
		t.Fatalf("approving with the destination gone = %v, want E_CONNECTION_NOT_FOUND", err)
	}

	if req, _ := e.svc.ApprovalGet(
		e.as(token),
		service.ApprovalRef{ID: job.ApprovalID},
	); req == nil ||
		req.State != approval.StatePending {
		t.Errorf("the request after the failed approval = %+v, want it still pending", req)
	}

	if p.calls.Load() != 0 {
		t.Error("the job ran with its destination gone")
	}

	// Submitting for a connection that does not exist is refused at once.
	p.conn = "con_0000000099"

	if _, err := e.submit(e.as(token), e.newProject("q")); code(err) != service.CodeConnectionNotFound {
		t.Errorf("a submission for a missing connection = %v, want E_CONNECTION_NOT_FOUND", err)
	}
}

func TestJobApproval_DryRunStartsNothing(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000)}
	e := newJobEnv(t, p)
	project := e.newProject("p")

	job, err := e.svc.JobStart(e.as(token), service.JobStartRequest{ProjectID: project, Kind: testKind, DryRun: true})
	if err != nil || job.State != service.JobWaitingApproval {
		t.Fatalf("dry run = %+v, %v", job, err)
	}

	if list, err := e.svc.ApprovalList(
		e.as(token),
		service.ApprovalListRequest{State: "all"},
	); err != nil ||
		list.Total != 0 {
		t.Errorf("a dry run left requests: %+v, %v", list, err)
	}

	if p.calls.Load() != 0 {
		t.Error("a dry run ran the job")
	}
}

func TestJobApproval_ScopeAndLimits(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(1)}
	e := newJobEnv(t, p)
	project := e.newProject("p")

	readOnly := service.WithCaller(
		context.Background(),
		service.Caller{Actor: key, Scopes: []string{service.ScopeRead}},
	)
	if _, err := e.submit(readOnly, project); code(err) != service.CodeForbidden {
		t.Errorf("a read-only caller submitting = %v, want E_FORBIDDEN", err)
	}

	if _, err := e.submit(e.as(token), "prj_9999999999"); code(err) != "E_NOT_FOUND" {
		t.Errorf("a missing project = %v, want E_NOT_FOUND", err)
	}

	for range service.MaxPendingPerSubmitter {
		if _, err := e.submit(e.as(token), project); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := e.submit(e.as(token), project); code(err) != "E_LIMIT_EXCEEDED" {
		t.Errorf("the 51st waiting request = %v, want E_LIMIT_EXCEEDED", err)
	}
}

func TestJobKindRegistration(t *testing.T) {
	t.Parallel()

	ok := func(context.Context, service.JobStartRequest) (service.Prepared, error) {
		return service.Prepared{}, nil
	}
	run := func(context.Context, service.JobRun) (service.JobOutcome, error) { return service.JobOutcome{}, nil }

	for name, kinds := range map[string][]service.ApprovableKind{
		"a built-in name":   {{Name: "export", Prepare: ok, Run: run}},
		"a bad name":        {{Name: "Bad Name", Prepare: ok, Run: run}},
		"no Run":            {{Name: "x.y", Prepare: ok}},
		"no Prepare":        {{Name: "x.y", Run: run}},
		"an unknown scope":  {{Name: "x.y", Scope: "root", Prepare: ok, Run: run}},
		"the same name too": {{Name: "x.y", Prepare: ok, Run: run}, {Name: "x.y", Prepare: ok, Run: run}},
	} {
		e := newEnv(t)

		if _, err := service.New(service.Options{Store: e.store, JobKinds: kinds}); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
