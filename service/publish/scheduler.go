package publish

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
)

// Clock is the time the scheduler reads and waits on; tests move it.
type Clock interface {
	Now() time.Time
	// After returns a channel that receives once d has passed.
	After(d time.Duration) <-chan time.Time
}

// SystemClock is the clock of the machine.
type SystemClock struct{}

// Now implements Clock.
func (SystemClock) Now() time.Time { return time.Now() }

// After implements Clock.
func (SystemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// SchedulerOptions tunes a scheduler; the zero value is the production setting.
type SchedulerOptions struct {
	Clock Clock
	// Lease is how long a claim holds without a renewal; default core.Lease.
	Lease time.Duration
	// Poll is the longest the loop sleeps: it also sees what another process changed; default 30 s.
	Poll time.Duration
	// Parallel is the most targets sent at once; default 3.
	Parallel int
	// Owner names this scheduler in the lease; default a random id.
	Owner string
	Log   *slog.Logger
	// Housekeeping runs at the start of every pass: the service uses it to expire the approval requests whose
	// week passed, so a post that waits for an answer nobody gave goes back to a draft by itself.
	Housekeeping func(ctx context.Context)
}

// Scheduler sends the approved targets at their time (SDD-020 section 19). It sleeps until the next time
// something is due, claims a target with a lease so that no other scheduler sends it too, sends it through
// the gateway and records the result.
type Scheduler struct {
	m     *Manager
	clock Clock
	lease time.Duration
	poll  time.Duration
	owner string
	log   *slog.Logger
	house func(ctx context.Context)

	wake chan struct{}
	sem  chan struct{}
	wg   sync.WaitGroup
}

// NewScheduler makes a scheduler over a manager and registers its wake-up with the manager.
func NewScheduler(m *Manager, o SchedulerOptions) *Scheduler {
	if o.Clock == nil {
		o.Clock = SystemClock{}
	}

	if o.Lease <= 0 {
		o.Lease = core.Lease
	}

	if o.Poll <= 0 {
		o.Poll = 30 * time.Second
	}

	if o.Parallel <= 0 {
		o.Parallel = 3
	}

	if o.Owner == "" {
		var b [8]byte

		_, _ = rand.Read(b[:])
		o.Owner = hex.EncodeToString(b[:])
	}

	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}

	s := &Scheduler{
		m: m, clock: o.Clock, lease: o.Lease, poll: o.Poll, owner: o.Owner, log: o.Log, house: o.Housekeeping,
		wake: make(chan struct{}, 1), sem: make(chan struct{}, o.Parallel),
	}

	m.OnChange(s.Wake)

	return s
}

// Owner is the id this scheduler holds leases under.
func (s *Scheduler) Owner() string { return s.owner }

// Wake brings the next pass forward, after a change that may have made something due.
func (s *Scheduler) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Wait returns when every send this scheduler started has ended.
func (s *Scheduler) Wait() { s.wg.Wait() }

// Tick is one pass: it marks the targets whose time passed beyond their window as missed, ends the ones
// that were interrupted too many times, and claims and starts the sends that are due. It returns the number
// of sends it started; Wait returns when they end.
func (s *Scheduler) Tick(ctx context.Context) (int, error) {
	if s.house != nil {
		s.house(ctx)
	}

	now := s.clock.Now()

	open, err := s.m.Open(ctx)
	if err != nil {
		return 0, err
	}

	byID := make(map[string]*Target, len(open))
	cands := make([]core.Candidate, 0, len(open))

	for i := range open {
		byID[open[i].ID] = &open[i]
		cands = append(cands, open[i].Candidate())
	}

	for _, c := range core.Missed(cands, now) {
		if _, err := s.m.Miss(ctx, c.ID); err != nil {
			s.log.WarnContext(ctx, "a missed post was not recorded", "target", c.ID, "error", err.Error())
		}
	}

	started := 0

	for _, c := range core.Due(cands, now) {
		t := byID[c.ID]

		// An attempt that was interrupted counts, so a post that keeps killing the process stops.
		if core.Decide(c, now) == core.ActionResume && t.Attempts >= core.MaxAttempts {
			if _, err := s.m.Interrupt(ctx, t.ID); err != nil {
				s.log.WarnContext(ctx, "an interrupted post was not recorded", "target", t.ID, "error", err.Error())
			}

			continue
		}

		ok, err := s.m.Claim(ctx, t.ID, s.owner, now, now.Add(s.lease))
		if err != nil {
			s.log.WarnContext(ctx, "a post could not be claimed", "target", t.ID, "error", err.Error())

			continue
		}

		if !ok {
			continue
		}

		t.Attempts++
		t.State, t.LeaseOwner = core.TargetPublishing, s.owner
		s.wg.Add(1)

		go s.send(ctx, *t)

		started++
	}

	return started, nil
}

// send sends one claimed target, renewing its lease meanwhile, and records how it ended.
func (s *Scheduler) send(ctx context.Context, t Target) {
	defer s.wg.Done()

	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return
	}

	sendCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan struct{})

	go s.heartbeat(sendCtx, cancel, t.ID, done)

	sent, err := s.m.Send(sendCtx, t, nil)

	close(done)

	// A shutdown interrupts the send: the target stays claimed, its lease runs out and the next process
	// resumes it with the same upload key.
	if ctx.Err() != nil {
		return
	}

	var serr *SendError

	switch {
	case err == nil:
	case errors.As(err, &serr):
	default:
		s.log.ErrorContext(ctx, "a post could not be sent", "target", t.ID, "error", err.Error())

		serr = &SendError{
			Kind: "internal", Code: "internal", Message: "the post could not be sent.",
			Hint: "try again; if it fails again, read the log of the server",
		}
	}

	if ferr := s.m.Finish(context.WithoutCancel(ctx), s.owner, t.ID, &sent, serr); ferr != nil {
		s.log.ErrorContext(ctx, "the result of a post was not recorded", "target", t.ID, "error", ferr.Error())
	}
}

// heartbeat renews the lease while the send runs. A lease that cannot be renewed cancels the send: another
// scheduler may hold the target by now.
func (s *Scheduler) heartbeat(ctx context.Context, cancel context.CancelFunc, id string, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-s.clock.After(s.lease / 3):
			ok, err := s.m.Renew(ctx, id, s.owner, s.clock.Now().Add(s.lease))
			if err == nil && !ok {
				s.log.WarnContext(ctx, "the lease of a post was lost", "target", id)
				cancel()

				return
			}
		}
	}
}

// Next is how long the loop may sleep: until the next time something is due, at most Poll.
func (s *Scheduler) Next(ctx context.Context) (time.Duration, error) {
	open, err := s.m.Open(ctx)
	if err != nil {
		return 0, err
	}

	now, wait := s.clock.Now(), s.poll

	for i := range open {
		if at, ok := core.Wake(open[i].Candidate()); ok && at.Sub(now) < wait {
			wait = at.Sub(now)
		}
	}

	// Something that is due and was not started (a claim another scheduler won) is looked at again soon, not
	// in a tight loop.
	if wait <= 0 {
		wait = time.Second
	}

	return wait, nil
}

// Run is the loop: tick, sleep until the next due time or a wake-up, until ctx ends. It waits for the sends
// it started before it returns.
func (s *Scheduler) Run(ctx context.Context) error {
	defer s.Wait()

	for {
		if _, err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			s.log.ErrorContext(ctx, "the scheduler could not read the calendar", "error", err.Error())
		}

		wait, err := s.Next(ctx)
		if err != nil {
			wait = s.poll
		}

		select {
		case <-ctx.Done():
			return nil
		case <-s.wake:
		case <-s.clock.After(wait):
		}
	}
}
