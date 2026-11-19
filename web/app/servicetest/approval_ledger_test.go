package servicetest

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

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
