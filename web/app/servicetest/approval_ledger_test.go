package servicetest

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/jobs"

	"github.com/AlexGalhardo/vukem/queue"
)

// beforeWrite runs a function once, before the next write transaction of the service.
type beforeWrite struct {
	service.Store

	next func()
}

func (b *beforeWrite) Write(ctx context.Context, fn func(ctx context.Context, tx service.Tx) error) error {
	if f := b.next; f != nil {
		b.next = nil
		f()
	}

	return b.Store.Write(ctx, fn)
}

// SDD-015 section 5: a cancel that races an approval still cancels. JobCancel reads the job waiting,
// a person approves it before the cancel writes, and the cancel then stops the job as a queued one
// instead of answering it queued and letting it run.
func TestJobApproval_CancelThatRacesAnApprovalStillCancels(t *testing.T) {
	t.Parallel()

	p := &probe{estimate: usd(500_000)}
	hook := &beforeWrite{}
	lazy := &lazyRunner{}

	var hold atomic.Bool

	hold.Store(true)

	e := newEnvWith(t, func(_ *env, o *service.Options) {
		q, err := jobs.NewQueue(queue.Sync{}, lazy)
		if err != nil {
			t.Fatal(err)
		}

		hook.Store = o.Store
		o.Store = hook
		o.Enqueue = gated{inner: jobs.Dispatcher{Queue: q}, hold: &hold}
		o.JobKinds = []service.ApprovableKind{p.kind()}
	})

	job, err := e.submit(e.as(token), e.newProject("p"))
	if err != nil {
		t.Fatal(err)
	}

	hook.next = func() {
		if err := e.approve(job.ApprovalID); err != nil {
			t.Errorf("the approval inside the race: %v", err)
		}
	}

	got, err := e.svc.JobCancel(e.as(token), service.JobCancelRequest{JobID: job.JobID})
	if err != nil || got.State != service.JobCancelled {
		t.Fatalf("a cancel that raced an approval = %+v, %v, want the job cancelled", got, err)
	}

	if n := e.spentToday(); n != 0 {
		t.Errorf("spent today = %d, want 0: the job never ran", n)
	}

	if p.calls.Load() != 0 {
		t.Error("the job ran after it was cancelled")
	}
}

// SDD-015 sections 3 and 5: a summary a person could misread (a bidirectional override, a zero-width
// character, a tag character) is refused at the submission as the caller's error, and writes nothing.
func TestJobApproval_SummaryThatCouldLookLikeSomethingElseIsRefused(t *testing.T) {
	t.Parallel()

	for name, summary := range map[string]string{
		"a bidirectional override": "Post to ‮TXT.exe",
		"a zero-width space":       "Pay​ 1 USD",
		"a tag character":          "Post \U000E0041",
		"a C1 control":             "Post \u0085 now",
	} {
		p := &probe{estimate: usd(1), summary: summary}
		e := newJobEnv(t, p)
		project := e.newProject("p")

		_, err := e.submit(e.as(token), project)

		var se *service.Error
		if !errors.As(err, &se) || se.Code != "E_VALIDATION" || se.Path != "args" || se.Hint == "" {
			t.Errorf("%s: %v, want E_VALIDATION on args with a hint", name, err)
		}

		e.nothingWritten(project, p)
	}
}

// SDD-007 T-50: the check of the ceiling and the reservation are one transaction, so people approving
// at the same moment, or a rule approving submissions that arrive together, never pass the ceiling.
func TestSecurity_T50_ConcurrentApprovalsNeverPassTheCeiling(t *testing.T) {
	t.Parallel()

	const n = 10

	w := &waiting{p: &probe{estimate: usd(300_000)}, block: make(chan struct{})}
	close(w.block)

	e, _ := newHeldEnv(t, w)
	project := e.newProject("p")
	e.setCeiling("", 0, 1_000_000)

	ids := make([]string, 0, n)

	for range n {
		job, err := e.submit(e.as(token), project)
		if err != nil {
			t.Fatal(err)
		}

		ids = append(ids, job.ApprovalID)
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		approved int
	)

	for _, id := range ids {
		wg.Go(func() {
			err := e.approve(id)

			mu.Lock()
			defer mu.Unlock()

			switch code(err) {
			case "":
				approved++
			case service.CodeCostCeiling:
			default:
				t.Errorf("approving %s = %v", id, err)
			}
		})
	}

	wg.Wait()

	if approved != 3 || e.spentToday() != 900_000 {
		t.Fatalf("%d approved at once, %d reserved; want 3 and 900000 under a ceiling of 1000000",
			approved, e.spentToday())
	}

	// A rule approving submissions that arrive together holds the same line.
	perItem := int64(300_000)
	if _, err := e.svc.ApprovalRuleSet(e.asPerson(), service.ApprovalRuleSetRequest{
		Kind: testKind, Currency: "USD", PerItemMicros: &perItem,
	}); err != nil {
		t.Fatal(err)
	}

	e.setCeiling("", 0, 2_000_000)

	var queued int

	for range n {
		wg.Go(func() {
			job, err := e.submit(e.as("local:agent-burst"), project)

			mu.Lock()
			defer mu.Unlock()

			switch {
			case err == nil && job.State == service.JobQueued:
				queued++
			case code(err) == service.CodeCostCeiling:
			default:
				t.Errorf("a submission under the rule = %+v, %v", job, err)
			}
		})
	}

	wg.Wait()

	if queued != 3 || e.spentToday() != 1_800_000 {
		t.Errorf("the rule approved %d at once, %d reserved; want 3 more and 1800000 under 2000000",
			queued, e.spentToday())
	}
}

// SDD-007 T-50: a run that spends and is then interrupted by a shutdown is requeued with its
// reservation kept, and what it already spent is recorded too, so the next run cannot spend past the
// ceiling on top of money that is gone.
func TestSecurity_T50_InterruptedRunThatSpentIsRecorded(t *testing.T) {
	t.Parallel()

	first := usd(100_000)
	w := &waiting{p: &probe{estimate: usd(600_000)}, block: make(chan struct{}), interrupted: &first}
	e, runner := newHeldEnv(t, w)
	project := e.newProject("p")
	e.setCeiling("", 0, 1_000_000)

	job := e.approvedJob(project)

	shutdown, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- runner.RunJob(shutdown, job.JobID) }()

	eventually(t, "the run to start", func() bool { return w.p.calls.Load() > 0 })
	stop()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("an interrupted RunJob = %v, want the context error", err)
	}

	if got, err := e.svc.JobGet(e.as(token), service.JobGetRequest{JobID: job.JobID}); err != nil ||
		got.State != service.JobQueued {
		t.Fatalf("the interrupted job = %+v, %v, want it requeued", got, err)
	}

	// The reservation of 600000 stays, and the 100000 already spent is counted besides.
	if n := e.spentToday(); n != 700_000 {
		t.Fatalf("spent today after the interruption = %d, want 700000", n)
	}

	// So 400000 more no longer fits under the ceiling of 1000000.
	w.p.estimate = usd(400_000)

	other, err := e.submit(e.as(token), project)
	if err == nil {
		err = e.approve(other.ApprovalID)
	}

	if code(err) != service.CodeCostCeiling {
		t.Fatalf("400000 on top of an interrupted run that spent = %v, want E_COST_CEILING", err)
	}

	// The second run finishes and settles its reservation at the estimate: both runs are counted.
	close(w.block)

	if err := runner.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	if n := e.spentToday(); n != 700_000 {
		t.Errorf("spent today after the second run = %d, want 700000 (100000 then 600000)", n)
	}
}

// SDD-015 section 6: an approved job that is cancelled before it runs spent nothing, so its
// reservation goes back; otherwise it would hold the daily ceiling for an action that never ran.
func TestJobApproval_CancelledBeforeItRunsGivesTheReservationBack(t *testing.T) {
	t.Parallel()

	w := &waiting{p: &probe{estimate: usd(600_000)}, block: make(chan struct{})}
	close(w.block)

	e, _ := newHeldEnv(t, w)
	project := e.newProject("p")
	e.setCeiling("", 0, 1_000_000)

	job := e.approvedJob(project)

	if n := e.spentToday(); n != 600_000 {
		t.Fatalf("spent today after the approval = %d, want the 600000 reserved", n)
	}

	got, err := e.svc.JobCancel(e.as(token), service.JobCancelRequest{JobID: job.JobID})
	if err != nil || got.State != service.JobCancelled {
		t.Fatalf("cancel of the queued job = %+v, %v", got, err)
	}

	if n := e.spentToday(); n != 0 {
		t.Fatalf("spent today after the cancel = %d, want 0: the job never ran", n)
	}

	if w.p.calls.Load() != 0 {
		t.Error("the cancelled job ran")
	}

	// The ceiling is free again for the next request of the same size.
	next := e.approvedJob(project)
	if next.ApprovalID == "" {
		t.Fatal("the next request has no approval id")
	}
}
