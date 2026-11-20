package publish

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
)

// schedRig is a manager with a fake service, a fake network and a clock the test moves.
type schedRig struct {
	*rig
	clock *fakeClock
}

func newSchedRig(t *testing.T) *schedRig {
	t.Helper()

	r := newRig(t)
	r.env.auto = true

	return &schedRig{rig: r, clock: newFakeClock(t0, r.env)}
}

func (r *schedRig) scheduler(owner string) *Scheduler {
	return NewScheduler(r.m, SchedulerOptions{Clock: r.clock, Owner: owner})
}

// plan creates a publication with one target at t0 plus after, scheduled and approved.
func (r *schedRig) plan(after time.Duration, late *int) (*Publication, string) {
	r.t.Helper()

	p := r.create(
		TargetInput{
			ConnectionID:      "con_1",
			At:                t0.Add(after).Format("2006-01-02T15:04:05"),
			Zone:              "UTC",
			LateWindowMinutes: late,
		},
	)
	id := p.Targets[0].ID

	if _, err := r.m.Schedule(context.Background(), ScheduleInput{ID: p.ID, TargetID: id}); err != nil {
		r.t.Fatal(err)
	}

	return p, id
}

func (r *schedRig) tick(s *Scheduler) int {
	r.t.Helper()

	n, err := s.Tick(context.Background())
	if err != nil {
		r.t.Fatal(err)
	}

	s.Wait()

	return n
}

func TestSchedulerSendsAtTheTimeAndOnlyOnce(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	s := r.scheduler("a")
	p1, early := r.plan(time.Hour, nil)
	_, late := r.plan(2*time.Hour, nil)

	if n := r.tick(s); n != 0 {
		t.Fatalf("started %d before any time", n)
	}

	r.clock.advance(time.Hour)

	if n := r.tick(s); n != 1 || r.gw.sent(early) != 1 || r.gw.sent(late) != 0 {
		t.Fatalf("at the first time: started %d, sent %d / %d", n, r.gw.sent(early), r.gw.sent(late))
	}

	tg := r.target(p1, early)
	if tg.State != core.TargetPublished || tg.RemoteID == "" || tg.URL == "" || tg.Attempts != 1 ||
		tg.LeaseOwner != "" ||
		tg.LeaseUntil != nil {
		t.Errorf("target = %+v", tg)
	}

	if len(r.env.settled) != 1 || r.env.settled[0] != early {
		t.Errorf("settled = %v", r.env.settled)
	}

	for range 3 {
		if n := r.tick(s); n != 0 {
			t.Fatalf("started %d more", n)
		}
	}

	r.clock.advance(time.Hour)

	if n := r.tick(s); n != 1 || r.gw.total() != 2 || r.gw.sent(early) != 1 || r.gw.sent(late) != 1 {
		t.Fatalf("at the second time: started %d, total %d", n, r.gw.total())
	}
}

func TestSchedulerTwoSchedulersClaimOnce(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	a, b := r.scheduler("a"), r.scheduler("b")

	const posts = 20

	ids := make([]string, 0, posts)

	var p *Publication

	for range posts {
		p, _ = r.plan(time.Minute, nil)
		ids = append(ids, p.Targets[0].ID)
	}

	r.clock.advance(time.Minute)

	var wg sync.WaitGroup

	started := make([]int, 2)

	for i, s := range []*Scheduler{a, b} {
		wg.Go(func() {
			for range 4 {
				n, err := s.Tick(context.Background())
				if err != nil {
					t.Error(err)
				}

				started[i] += n
			}
		})
	}

	wg.Wait()
	a.Wait()
	b.Wait()

	if started[0]+started[1] != posts {
		t.Fatalf("the schedulers started %d + %d sends for %d targets", started[0], started[1], posts)
	}

	for _, id := range ids {
		if got := r.gw.sent(id); got != 1 {
			t.Errorf("target %s was sent %d times", id, got)
		}
	}
}

func TestSchedulerRestartMidPublishResumes(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	r.gw.block = make(chan struct{})

	p, id := r.plan(time.Minute, nil)
	crashed := r.scheduler("crashed")

	r.clock.advance(time.Minute)

	ctx, die := context.WithCancel(context.Background())
	if n, err := crashed.Tick(ctx); err != nil || n != 1 {
		t.Fatalf("Tick = %d, %v", n, err)
	}

	<-r.gw.started

	// The process is killed mid-send: the send is cut and nothing records an end.
	die()
	crashed.Wait()

	mid := r.target(p, id)
	if mid.State != core.TargetPublishing || mid.LeaseOwner != "crashed" || mid.Attempts != 1 || r.gw.sent(id) != 0 {
		t.Fatalf("after the crash: %+v", mid)
	}

	// Another scheduler leaves it while the lease is alive.
	other := r.scheduler("other")
	r.clock.advance(core.Lease / 2)

	if n := r.tick(other); n != 0 {
		t.Fatalf("took a target whose lease is alive")
	}

	// The lease ends: it takes it over, and sends it with the same post.
	close(r.gw.block)
	r.clock.advance(core.Lease)

	if n := r.tick(other); n != 1 {
		t.Fatalf("did not resume: %d", n)
	}

	done := r.target(p, id)
	if done.State != core.TargetPublished || done.Attempts != 2 || r.gw.sent(id) != 1 || done.LeaseOwner != "" {
		t.Errorf("after the resume: %+v, sent %d", done, r.gw.sent(id))
	}
}

func TestSchedulerStopsAPostThatKeepsInterrupting(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	p, id := r.plan(time.Minute, nil)

	cur, _ := r.env.store.Get(context.Background(), p.ID)
	tg := cur.Target(id)
	tg.State, tg.Attempts, tg.LeaseOwner = core.TargetPublishing, core.MaxAttempts, "dead"
	lease := t0.Add(time.Minute)
	tg.LeaseUntil = &lease

	_ = r.env.store.Save(context.Background(), &cur)

	r.clock.advance(time.Hour)

	if n := r.tick(r.scheduler("a")); n != 0 || r.gw.total() != 0 {
		t.Fatalf("sent a post that was interrupted %d times", core.MaxAttempts)
	}

	got := r.target(p, id)
	if got.State != core.TargetFailed || got.Error == nil || got.Error.Code != "interrupted" || got.Error.Hint == "" {
		t.Errorf("target = %+v, error = %+v", got, got.Error)
	}

	if !contains(r.env.released, id) {
		t.Error("the cost of a failed post was kept")
	}
}

func TestSchedulerMissedTime(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		late     *int
		stopped  time.Duration
		wantSent int
		want     core.TargetState
	}{
		{"on time", nil, time.Hour, 1, core.TargetPublished},
		{"in the grace", nil, time.Hour + 59*time.Second, 1, core.TargetPublished},
		{"one minute late", nil, time.Hour + 2*time.Minute, 0, core.TargetMissed},
		{"the product was stopped", nil, 5 * time.Hour, 0, core.TargetMissed},
		{"late but inside the window", new(240), 4 * time.Hour, 1, core.TargetPublished},
		{"past the window", new(30), 4 * time.Hour, 0, core.TargetMissed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := newSchedRig(t)
			p, id := r.plan(time.Hour, tc.late)
			s := r.scheduler("a")

			r.clock.advance(tc.stopped)
			r.tick(s)

			if got := r.gw.sent(id); got != tc.wantSent {
				t.Errorf("sent %d, want %d", got, tc.wantSent)
			}

			if got := r.target(p, id).State; got != tc.want {
				t.Errorf("state = %s, want %s", got, tc.want)
			}

			// However long it runs after, a missed post is not sent.
			if tc.want == core.TargetMissed {
				r.clock.advance(48 * time.Hour)
				r.tick(s)

				if r.gw.total() != 0 || r.target(p, id).State != core.TargetMissed {
					t.Error("a missed post moved on its own")
				}

				if !contains(r.env.released, id) {
					t.Error("the cost of a missed post was kept")
				}
			}
		})
	}
}

func TestSchedulerTransientErrorsRetryWithBackoffAndACeiling(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	r.gw.fail, r.gw.failN = &SendError{Kind: "unavailable", Code: "E_PUBLISH", Message: "down", Hint: "wait"}, 100
	p, id := r.plan(time.Minute, nil)
	s := r.scheduler("a")

	r.clock.advance(time.Minute)

	// Each failure waits twice as long as the one before, up to the attempts the ceiling allows.
	for i, wait := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute} {
		if n := r.tick(s); n != 1 {
			t.Fatalf("attempt %d: started %d", i+1, n)
		}

		tg := r.target(p, id)
		if tg.State != core.TargetScheduled || tg.Attempts != i+1 || tg.NextAttemptAt == nil ||
			!tg.NextAttemptAt.Equal(r.clock.Now().Add(wait)) || tg.Error == nil || !tg.Error.Retryable {
			t.Fatalf("after attempt %d: %+v (error %+v), want the next at +%s", i+1, tg, tg.Error, wait)
		}

		r.clock.advance(wait - time.Second)

		if n := r.tick(s); n != 0 {
			t.Fatalf("attempt %d came a second early", i+2)
		}

		r.clock.advance(time.Second)
	}

	// The fifth attempt is the last.
	if n := r.tick(s); n != 1 {
		t.Fatalf("fifth attempt: started %d", n)
	}

	last := r.target(p, id)
	if last.State != core.TargetFailed || last.Attempts != core.MaxAttempts || last.NextAttemptAt != nil ||
		last.Error == nil {
		t.Fatalf("after the ceiling: %+v", last)
	}

	if r.gw.sent(id) != core.MaxAttempts {
		t.Errorf("sent %d times, want %d", r.gw.sent(id), core.MaxAttempts)
	}

	r.clock.advance(time.Hour)

	if n := r.tick(s); n != 0 {
		t.Fatalf("a failed post was sent again")
	}
}

func TestSchedulerPermanentErrorsAreNotRetried(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"quota", "auth", "not_eligible", "invalid", "rejected", "internal"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			r := newSchedRig(t)
			r.gw.fail, r.gw.failN = &SendError{Kind: kind, Code: "E_PUBLISH", Message: "no", Hint: "fix it"}, 100
			p, id := r.plan(time.Minute, nil)
			s := r.scheduler("a")

			r.clock.advance(time.Minute)
			r.tick(s)

			tg := r.target(p, id)
			if tg.State != core.TargetFailed || tg.Error == nil || tg.Error.Kind != kind || tg.Error.Hint == "" ||
				tg.NextAttemptAt != nil {
				t.Fatalf("target = %+v, error = %+v", tg, tg.Error)
			}

			r.clock.advance(time.Hour)
			r.tick(s)

			if r.gw.sent(id) != 1 || len(r.env.settled) != 0 || !contains(r.env.released, id) {
				t.Errorf("sent %d, settled %v, released %v", r.gw.sent(id), r.env.settled, r.env.released)
			}
		})
	}
}

func TestSchedulerNeverSendsASnapshotThatChanged(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	p, id := r.plan(time.Minute, nil)

	cur, _ := r.env.store.Get(context.Background(), p.ID)
	cur.Target(id).Snapshot = []byte(`{"title":"not what was approved"}`)
	_ = r.env.store.Save(context.Background(), &cur)

	r.clock.advance(time.Minute)
	r.tick(r.scheduler("a"))

	got := r.target(p, id)
	if got.State != core.TargetFailed || r.gw.total() != 0 || got.Error == nil || got.Error.Code != "payload_changed" {
		t.Fatalf("target = %+v, error = %+v, sent %d", got, got.Error, r.gw.total())
	}
}

func TestSchedulerLeaseIsRenewedWhileSending(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	r.gw.block = make(chan struct{})

	p, id := r.plan(time.Minute, nil)
	s := r.scheduler("a")

	r.clock.advance(time.Minute)

	if n, err := s.Tick(context.Background()); err != nil || n != 1 {
		t.Fatalf("Tick = %d, %v", n, err)
	}

	<-r.gw.started
	waitFor(t, "the heartbeat to wait on the clock", func() bool { return r.clock.pending() > 0 })

	first := *r.target(p, id).LeaseUntil

	// A third of the lease passes: the heartbeat renews it.
	r.clock.advance(core.Lease / 3)

	waitFor(t, "the lease to be renewed", func() bool { return r.target(p, id).LeaseUntil.After(first) })

	close(r.gw.block)
	s.Wait()

	if got := r.target(p, id); got.State != core.TargetPublished {
		t.Errorf("state = %s", got.State)
	}
}

func TestSchedulerLoopSleepsUntilTheNextTimeAndWakes(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	s := NewScheduler(r.m, SchedulerOptions{Clock: r.clock, Owner: "loop", Poll: time.Hour})

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- s.Run(ctx) }()

	waitFor(t, "the loop to sleep", func() bool { return r.clock.pending() > 0 })

	// A change wakes it: a post planned while it sleeps is seen at once, and it sleeps until its time.
	p, id := r.plan(10*time.Minute, nil)

	waitFor(t, "the loop to look at the new post", func() bool { return r.clock.pending() > 0 })

	r.clock.advance(10 * time.Minute)

	waitFor(t, "the post to be sent", func() bool { return r.gw.sent(id) == 1 })
	waitFor(t, "the result to be recorded", func() bool { return r.target(p, id).State == core.TargetPublished })

	stop()

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSchedulerNextWake(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	s := NewScheduler(r.m, SchedulerOptions{Clock: r.clock, Owner: "n", Poll: 30 * time.Second})

	if d, err := s.Next(context.Background()); err != nil || d != 30*time.Second {
		t.Errorf("empty calendar: %v, %v", d, err)
	}

	r.plan(10*time.Second, nil)

	if d, _ := s.Next(context.Background()); d != 10*time.Second {
		t.Errorf("one post in 10 s: %v", d)
	}

	r.plan(time.Hour, nil)

	if d, _ := s.Next(context.Background()); d != 10*time.Second {
		t.Errorf("the nearest wins: %v", d)
	}

	// Something due that nobody took is looked at again in a second, not in a tight loop.
	r.clock.advance(time.Minute)

	if d, _ := s.Next(context.Background()); d != time.Second {
		t.Errorf("a due post: %v", d)
	}
}

// A claimed post that waits for a free slot keeps its lease: left unrenewed in the queue, the lease would run
// out and the next pass, of this process or of another, would take the post and send it a second time.
func TestSchedulerQueuedClaimKeepsItsLease(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	r.gw.block = make(chan struct{})

	p, first := r.plan(time.Minute, nil)
	q, second := r.plan(time.Minute, nil)
	s := NewScheduler(r.m, SchedulerOptions{Clock: r.clock, Owner: "a", Parallel: 1})
	other := r.scheduler("b")

	r.clock.advance(time.Minute)

	if n, err := s.Tick(context.Background()); err != nil || n != 2 {
		t.Fatalf("Tick = %d, %v", n, err)
	}

	<-r.gw.started

	// The send in the one slot takes longer than a lease, and the other post waits behind it.
	for range 4 {
		waitFor(t, "both heartbeats to wait on the clock", func() bool { return r.clock.pending() >= 2 })
		r.clock.advance(core.Lease / 3)

		now := r.clock.Now()
		waitFor(t, "both leases to be renewed", func() bool {
			return r.target(p, first).LeaseUntil.After(now) && r.target(q, second).LeaseUntil.After(now)
		})
	}

	for name, sc := range map[string]*Scheduler{"the same": s, "another": other} {
		if n, err := sc.Tick(context.Background()); err != nil || n != 0 {
			t.Fatalf("%s scheduler took a post that is queued: %d, %v", name, n, err)
		}
	}

	close(r.gw.block)
	s.Wait()
	other.Wait()

	if n := r.tick(s); n != 0 {
		t.Fatalf("started %d sends after both went out", n)
	}

	for _, id := range []string{first, second} {
		if got := r.gw.sent(id); got != 1 {
			t.Errorf("target %s was sent %d times", id, got)
		}
	}

	if r.target(p, first).State != core.TargetPublished || r.target(q, second).State != core.TargetPublished {
		t.Errorf("states %s and %s", r.target(p, first).State, r.target(q, second).State)
	}
}

// What is sent is the post the target holds when it is claimed, not the one a pass read before: an edit that
// was approved again between the read and the claim sends the new post, never the former one.
func TestSchedulerSendsThePostApprovedAtTheClaim(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	p, id := r.plan(time.Minute, nil)
	s := r.scheduler("a")

	r.clock.advance(time.Minute)

	edited := "the edited title"
	r.env.store.beforeClaim = func() {
		if _, err := r.m.Update(context.Background(), UpdateInput{ID: p.ID, Title: &edited}); err != nil {
			t.Error(err)
		}
	}

	if n := r.tick(s); n != 1 {
		t.Fatalf("started %d", n)
	}

	if got := r.gw.posts[id]; !strings.Contains(got, edited) {
		t.Fatalf("sent %s, not the post approved after the edit", got)
	}

	if got := r.target(p, id); got.State != core.TargetPublished {
		t.Errorf("state = %s", got.State)
	}
}

func TestSchedulerPausedStartsNothingAndMissesWhatIsLate(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	s := r.scheduler("paused")
	p, id := r.plan(time.Hour, nil)

	if n, err := s.Waiting(context.Background()); err != nil || n != 1 {
		t.Fatalf("waiting = %d, %v; want 1", n, err)
	}

	s.SetPaused(true)

	if !s.Paused() {
		t.Fatal("not paused")
	}

	r.clock.advance(time.Hour)

	if n := r.tick(s); n != 0 || r.gw.total() != 0 {
		t.Fatalf("a paused pass started %d, sent %d", n, r.gw.total())
	}

	if tg := r.target(p, id); tg.State != core.TargetScheduled {
		t.Fatalf("state = %s while paused, want scheduled", tg.State)
	}

	if d, err := s.Next(context.Background()); err != nil || d != 30*time.Second {
		t.Fatalf("next = %v, %v; a paused loop sleeps at the poll", d, err)
	}

	// Resumed, the post that is still on time goes out once.
	s.SetPaused(false)

	if n := r.tick(s); n != 1 || r.gw.sent(id) != 1 {
		t.Fatalf("after resume started %d, sent %d", n, r.gw.sent(id))
	}
}

func TestSchedulerPausedPastTheWindowIsMissedNotSent(t *testing.T) {
	t.Parallel()

	r := newSchedRig(t)
	s := r.scheduler("paused-late")
	p, id := r.plan(time.Hour, nil)

	s.SetPaused(true)
	r.clock.advance(3 * time.Hour)
	r.tick(s)
	s.SetPaused(false)

	if n := r.tick(s); n != 0 || r.gw.total() != 0 {
		t.Fatalf("a late post was sent after the resume: started %d, sent %d", n, r.gw.total())
	}

	if tg := r.target(p, id); tg.State != core.TargetMissed {
		t.Fatalf("state = %s, want missed", tg.State)
	}
}
