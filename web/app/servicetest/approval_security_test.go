package servicetest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/jobs"

	"github.com/AlexGalhardo/vukem/queue"
)

// asUserKey is an API key of a user of network mode.
func asUserKey(id, user string) context.Context {
	return service.WithCaller(context.Background(), service.Caller{
		Actor: "key:" + id, Scopes: service.AllScopes, OwnerUserID: user,
	})
}

// SDD-007 T-51: one submitter cannot fill the approval queue and block everyone else.
func TestSecurity_T51_OneSubmitterCannotFillTheQueue(t *testing.T) {
	t.Parallel()

	e := newJobEnv(t, &probe{estimate: usd(1)})
	project := e.newProject("p")

	// One local agent fills its own share.
	for range service.MaxPendingPerSubmitter {
		if _, err := e.submit(e.as(token), project); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := e.submit(e.as(token), project); code(err) != "E_LIMIT_EXCEEDED" {
		t.Fatalf("a submitter past its share = %v, want E_LIMIT_EXCEEDED", err)
	}

	// Another agent still gets in.
	if _, err := e.submit(e.as("local:other-agent"), project); err != nil {
		t.Errorf("another agent was blocked by the first: %v", err)
	}

	// In network mode the share is the user, not the key: two keys of one user share it.
	mine, theirs := "usr_0000000001", "usr_0000000002"

	own := func(user string) string {
		res, err := e.svc.ProjectCreate(asUserKey("key_0000000009", user), service.ProjectCreateRequest{Name: user})
		if err != nil {
			t.Fatal(err)
		}

		return res.ProjectID
	}

	minePrj, theirsPrj := own(mine), own(theirs)

	for i := range service.MaxPendingPerSubmitter {
		if _, err := e.submit(asUserKey(fmt.Sprintf("key_000000000%d", i%2), mine), minePrj); err != nil {
			t.Fatalf("user request %d: %v", i, err)
		}
	}

	if _, err := e.submit(asUserKey("key_0000000002", mine), minePrj); code(err) != "E_LIMIT_EXCEEDED" {
		t.Errorf("a third key of the same user past the share = %v, want E_LIMIT_EXCEEDED", err)
	}

	if _, err := e.submit(asUserKey("key_0000000003", theirs), theirsPrj); err != nil {
		t.Errorf("another user was blocked: %v", err)
	}
}

// The workspace as a whole is bounded too, by a much larger number.
func TestSecurity_T51_TheWholeQueueIsBounded(t *testing.T) {
	t.Parallel()

	e := newJobEnv(t, &probe{estimate: usd(1)})
	project := e.newProject("p")

	for i := range service.MaxPendingApprovals {
		e.addRequestFor(fmt.Sprintf("usr_%010d", i), "")
	}

	if _, err := e.submit(e.as(token), project); code(err) != "E_LIMIT_EXCEEDED" {
		t.Errorf("a full workspace queue = %v, want E_LIMIT_EXCEEDED", err)
	}
}

// waiting is a kind whose Run waits for its context, as a long generation does.
type waiting struct {
	p     *probe
	block chan struct{}
	// during runs inside Run, before it waits.
	during func()
}

func (w *waiting) kind() service.ApprovableKind {
	k := w.p.kind()
	k.Run = func(ctx context.Context, _ service.JobRun) (service.JobOutcome, error) {
		w.p.calls.Add(1)

		if w.during != nil {
			w.during()
		}

		select {
		case <-w.block:
			return service.JobOutcome{Result: json.RawMessage(`{"ok":true}`), Cost: w.p.cost}, nil
		case <-ctx.Done():
			// An interrupted run reports no cost and an error: the job is requeued, not failed.
			return service.JobOutcome{}, ctx.Err()
		}
	}

	return k
}

// newHeldEnv is an environment whose approved jobs stay queued until the test runs them.
func newHeldEnv(tb testing.TB, w *waiting) (*env, service.JobRunner) {
	tb.Helper()

	lazy := &lazyRunner{}

	var hold atomic.Bool

	hold.Store(true)

	e := newEnvWith(tb, func(_ *env, o *service.Options) {
		q, err := jobs.NewQueue(queue.Sync{}, lazy)
		if err != nil {
			tb.Fatal(err)
		}

		o.Enqueue = gated{inner: jobs.Dispatcher{Queue: q}, hold: &hold}
		o.JobKinds = []service.ApprovableKind{w.kind()}
	})

	r, ok := service.RunnerOf(e.svc)
	if !ok {
		tb.Fatal("the service has no job runner")
	}

	lazy.mu.Lock()
	lazy.r = r
	lazy.mu.Unlock()

	return e, r
}

func (e *env) approvedJob(project string) *service.Job {
	e.t.Helper()

	job, err := e.submit(e.as(token), project)
	if err != nil {
		e.t.Fatal(err)
	}

	if err := e.approve(job.ApprovalID); err != nil {
		e.t.Fatal(err)
	}

	return job
}

// SDD-007 T-50: a job interrupted by a shutdown and requeued stays counted against the ceiling, so
// the run that follows cannot spend past it.
func TestSecurity_T50_InterruptedJobCannotSpendPastTheCeiling(t *testing.T) {
	t.Parallel()

	w := &waiting{p: &probe{estimate: usd(600_000)}, block: make(chan struct{})}
	e, runner := newHeldEnv(t, w)
	project := e.newProject("p")
	e.setCeiling("", 0, 1_000_000)

	job := e.approvedJob(project)

	// The shutdown interrupts the run: the job goes back to the queue.
	shutdown, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- runner.RunJob(shutdown, job.JobID) }()

	eventually(t, "the run to start", func() bool { return w.p.calls.Load() > 0 })
	stop()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("an interrupted RunJob = %v, want the context error", err)
	}

	got, err := e.svc.JobGet(e.as(token), service.JobGetRequest{JobID: job.JobID})
	if err != nil || got.State != service.JobQueued {
		t.Fatalf("the interrupted job = %+v, %v, want it requeued", got, err)
	}

	// Still counted: the interruption did not release the reservation.
	if n := e.spentToday(); n != 600_000 {
		t.Fatalf("spent today after the interruption = %d, want the 600000 still reserved", n)
	}

	// So another request of that size cannot be approved on top of it.
	other, err := e.submit(e.as(token), project)
	if err == nil {
		err = e.approve(other.ApprovalID)
	}

	if code(err) != service.CodeCostCeiling {
		t.Fatalf("a second 600000 on top of the requeued job = %v, want E_COST_CEILING", err)
	}

	// The requeued run finishes and settles once.
	close(w.block)

	if err := runner.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	if fin, err := e.svc.JobGet(
		e.as(token),
		service.JobGetRequest{JobID: job.JobID},
	); err != nil ||
		fin.State != service.JobSucceeded {
		t.Fatalf("the requeued job = %+v, %v", fin, err)
	}

	if n := e.spentToday(); n != 600_000 {
		t.Errorf("spent today after the second run = %d, want 600000 (settled once)", n)
	}
}

// A job that starts with no reservation (its entry was released) reserves again, and is refused when
// that no longer fits the ceiling.
func TestSecurity_T50_StartingWithNoReservationReservesAgain(t *testing.T) {
	t.Parallel()

	w := &waiting{p: &probe{estimate: usd(600_000)}, block: make(chan struct{})}
	close(w.block)

	e, runner := newHeldEnv(t, w)
	project := e.newProject("p")
	e.setCeiling("", 0, 1_000_000)

	first := e.approvedJob(project)

	// The reservation is lost: the run puts it back before it spends.
	if _, err := e.store.Approvals().Release(context.Background(), first.JobID, 0); err != nil {
		t.Fatal(err)
	}

	if e.spentToday() != 0 {
		t.Fatal("the release did not free the entry")
	}

	if err := runner.RunJob(context.Background(), first.JobID); err != nil {
		t.Fatal(err)
	}

	if n := e.spentToday(); n != 600_000 {
		t.Errorf("spent after a run that reserved again = %d, want 600000", n)
	}

	// A second job loses its reservation while the first has used the ceiling: it must not run.
	w.p.estimate = usd(400_000)
	second := e.approvedJob(project)

	if _, err := e.store.Approvals().Release(context.Background(), second.JobID, 0); err != nil {
		t.Fatal(err)
	}

	e.setCeiling("", 0, 900_000)

	before := w.p.calls.Load()

	if err := runner.RunJob(context.Background(), second.JobID); err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.JobGet(e.as(token), service.JobGetRequest{JobID: second.JobID})
	if err != nil || got.State != service.JobFailed || got.Error == nil || got.Error.Code != service.CodeCostCeiling {
		t.Fatalf("a job that no longer fits = %+v, %v, want failed with E_COST_CEILING", got, err)
	}

	if w.p.calls.Load() != before {
		t.Error("a job refused by the ceiling still ran")
	}
}

// A run that finishes with no reservation left is still recorded, at the cost it reports.
func TestSecurity_T50_FinishingWithNoReservationStillRecordsTheCost(t *testing.T) {
	t.Parallel()

	spent := usd(250_000)
	w := &waiting{p: &probe{estimate: usd(500_000), cost: &spent}, block: make(chan struct{})}
	close(w.block)

	e, runner := newHeldEnv(t, w)
	job := e.approvedJob(e.newProject("p"))

	// The reservation disappears while the job runs.
	w.during = func() {
		if _, err := e.store.Approvals().Release(context.Background(), job.JobID, 0); err != nil {
			t.Error(err)
		}
	}

	if err := runner.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	if n := e.spentToday(); n != 250_000 {
		t.Errorf("spent today = %d, want the real cost 250000 recorded", n)
	}
}
