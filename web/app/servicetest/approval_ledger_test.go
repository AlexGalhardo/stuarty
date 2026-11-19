package servicetest

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

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
