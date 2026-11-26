package servicetest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/jobs"

	"github.com/AlexGalhardo/vukem/queue"
)

func TestJobApproval_PanicReleasesReservation(t *testing.T) {
	t.Parallel()

	lazy := &lazyRunner{}
	est := usd(500_000)

	e := newEnvWith(t, func(_ *env, o *service.Options) {
		q, err := jobs.NewQueue(queue.Sync{}, lazy)
		if err != nil {
			t.Fatal(err)
		}

		o.Enqueue = jobs.Dispatcher{Queue: q}
		o.JobKinds = []service.ApprovableKind{{
			Name: testKind,
			Prepare: func(_ context.Context, _ service.JobStartRequest) (service.Prepared, error) {
				return service.Prepared{
					Summary:  "Generate with panic",
					Estimate: est,
					Input:    json.RawMessage(`{"prompt":"boom"}`),
				}, nil
			},
			Run: func(_ context.Context, _ service.JobRun) (service.JobOutcome, error) {
				panic("simulated handler panic")
			},
		}}
	})

	r, ok := service.RunnerOf(e.svc)
	if !ok {
		t.Fatal("the service has no job runner")
	}

	lazy.mu.Lock()
	lazy.r = r
	lazy.mu.Unlock()

	project := e.newProject("p")

	job, err := e.submit(e.as(token), project)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ApprovalDecide(
		e.asPerson(),
		service.ApprovalDecideRequest{ID: job.ApprovalID, Decision: "approve"},
	); err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.JobGet(e.as(token), service.JobGetRequest{JobID: job.JobID})
	if err != nil {
		t.Fatal(err)
	}

	if got.State != service.JobFailed {
		t.Fatalf("job state = %s, want failed", got.State)
	}

	if spent := e.spentToday(); spent != 0 {
		t.Errorf("spentToday = %d after handler panic, want 0 (reservation leaked)", spent)
	}
}
