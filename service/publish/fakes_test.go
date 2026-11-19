package publish

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
)

// memStore is a Store in memory. Each call takes the lock of the store; the transaction of fakeEnv holds a
// second lock for its whole length, as BEGIN IMMEDIATE does, so writers are serialised.
type memStore struct {
	mu   sync.Mutex
	pubs map[string]Publication
}

func cloneOf(p Publication) Publication {
	p.Targets = slices.Clone(p.Targets)

	return p
}

func (s *memStore) Insert(_ context.Context, p *Publication) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pubs[p.ID] = cloneOf(*p)

	return nil
}

func (s *memStore) Get(_ context.Context, id string) (Publication, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.pubs[id]
	if !ok {
		return Publication{}, ErrNotFound
	}

	return cloneOf(p), nil
}

func (s *memStore) OfTarget(_ context.Context, id string) (Publication, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key := range s.pubs {
		p := s.pubs[key]

		if p.Target(id) != nil {
			return cloneOf(p), nil
		}
	}

	return Publication{}, ErrNotFound
}

func (s *memStore) Save(_ context.Context, p *Publication) error {
	defer activity.ring()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pubs[p.ID] = cloneOf(*p)

	return nil
}

func (s *memStore) List(_ context.Context, f Filter) ([]Publication, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Publication

	for key := range s.pubs {
		p := s.pubs[key]

		if f.ProjectID != "" && p.ProjectID != f.ProjectID {
			continue
		}

		match := f.State == "" && f.Network == "" && f.From == nil && f.To == nil

		for i := range p.Targets {
			t := &p.Targets[i]
			if (f.State == "" || t.State == f.State) && (f.Network == "" || t.Network == f.Network) &&
				(f.From == nil || t.ScheduledAt != nil && !t.ScheduledAt.Before(*f.From)) &&
				(f.To == nil || t.ScheduledAt != nil && t.ScheduledAt.Before(*f.To)) {
				match = true
			}
		}

		if match {
			out = append(out, cloneOf(p))
		}
	}

	slices.SortFunc(out, func(a, b Publication) int { return cmp.Compare(a.ID, b.ID) })

	return out, len(out), nil
}

func (s *memStore) OfProject(ctx context.Context, project string) ([]Publication, error) {
	list, _, err := s.List(ctx, Filter{ProjectID: project})

	return list, err
}

func (s *memStore) Open(_ context.Context) ([]Target, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Target

	for key := range s.pubs {
		p := s.pubs[key]

		for i := range p.Targets {
			if st := p.Targets[i].State; st == core.TargetApproved || st == core.TargetScheduled ||
				st == core.TargetPublishing {
				out = append(out, p.Targets[i])
			}
		}
	}

	slices.SortFunc(out, func(a, b Target) int { return cmp.Compare(a.ID, b.ID) })

	return out, nil
}

func (s *memStore) Claim(_ context.Context, id, owner string, now, until time.Time) (bool, error) {
	defer activity.ring()

	s.mu.Lock()
	defer s.mu.Unlock()

	for pid := range s.pubs {
		p := s.pubs[pid]
		t := p.Target(id)
		if t == nil {
			continue
		}

		due := false

		if t.State == core.TargetApproved || t.State == core.TargetScheduled {
			at := t.ScheduledAt
			if t.NextAttemptAt != nil {
				at = t.NextAttemptAt
			}

			due = at != nil && !at.After(now)
		}

		if t.State == core.TargetPublishing {
			due = t.LeaseUntil != nil && t.LeaseUntil.Before(now)
		}

		if !due {
			return false, nil
		}

		cp := cloneOf(p)
		ct := cp.Target(id)
		ct.State, ct.LeaseOwner, ct.Attempts = core.TargetPublishing, owner, ct.Attempts+1
		u := until
		ct.LeaseUntil = &u
		s.pubs[pid] = cp

		return true, nil
	}

	return false, nil
}

func (s *memStore) Renew(_ context.Context, id, owner string, until time.Time) (bool, error) {
	defer activity.ring()

	s.mu.Lock()
	defer s.mu.Unlock()

	for pid := range s.pubs {
		p := s.pubs[pid]
		t := p.Target(id)
		if t == nil {
			continue
		}

		if t.State != core.TargetPublishing || t.LeaseOwner != owner {
			return false, nil
		}

		cp := cloneOf(p)
		u := until
		cp.Target(id).LeaseUntil = &u
		s.pubs[pid] = cp

		return true, nil
	}

	return false, nil
}

// fakeEnv is the service as the manager sees it: one store, a clock, and a record of what the transaction was
// asked to do about approvals and costs.
type fakeEnv struct {
	mu    sync.Mutex
	wmu   sync.Mutex
	store *memStore
	now   time.Time
	ids   int

	// auto approves a request at once, as a rule the person wrote would.
	auto bool
	m    *Manager

	asks      []Ask
	withdrawn []string
	released  []string
	settled   []string
	events    []Publication
	askErr    error
}

func newFakeEnv(now time.Time) *fakeEnv {
	return &fakeEnv{store: &memStore{pubs: map[string]Publication{}}, now: now}
}

func (e *fakeEnv) Store() Store { return e.store }

func (e *fakeEnv) Now() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.now
}

func (e *fakeEnv) set(t time.Time) {
	e.mu.Lock()
	e.now = t
	e.mu.Unlock()
}

func (e *fakeEnv) NewID(prefix string) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ids++

	return fmt.Sprintf("%s_%010d", prefix, e.ids)
}

func (e *fakeEnv) Actor(context.Context) Actor { return Actor{ID: "local:test"} }

func (e *fakeEnv) Write(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
	e.wmu.Lock()
	defer e.wmu.Unlock()

	e.store.mu.Lock()
	before := make(map[string]Publication, len(e.store.pubs))

	for k := range e.store.pubs {
		before[k] = cloneOf(e.store.pubs[k])
	}

	e.store.mu.Unlock()

	nasks, nwith, nrel, nset, nev := len(e.asks), len(e.withdrawn), len(e.released), len(e.settled), len(e.events)

	err := fn(ctx, &fakeTx{e: e})
	if err != nil {
		e.store.mu.Lock()
		e.store.pubs = before
		e.store.mu.Unlock()

		e.asks, e.withdrawn, e.released, e.settled, e.events = e.asks[:nasks], e.withdrawn[:nwith], e.released[:nrel],
			e.settled[:nset], e.events[:nev]
	}

	return err
}

type fakeTx struct{ e *fakeEnv }

func (t *fakeTx) Store() Store { return t.e.store }

func (t *fakeTx) Ask(ctx context.Context, a Ask) error {
	if t.e.askErr != nil {
		return t.e.askErr
	}

	t.e.asks = append(t.e.asks, a)

	if t.e.auto {
		return t.e.m.Approved(ctx, t, a.TargetID, a.ApprovalID)
	}

	return nil
}

// Withdraw rejects the request, as the service does, and the target hears of it: it may already point at a
// newer request, in which case nothing happens.
func (t *fakeTx) Withdraw(ctx context.Context, id, _ string) error {
	t.e.withdrawn = append(t.e.withdrawn, id)

	for _, a := range t.e.asks {
		if a.ApprovalID == id {
			return t.e.m.Declined(ctx, t, a.TargetID, id, CodeRejected, "withdrawn")
		}
	}

	return nil
}

func (t *fakeTx) Release(_ context.Context, id string) error {
	t.e.released = append(t.e.released, id)

	return nil
}

func (t *fakeTx) Settle(_ context.Context, id string) error {
	t.e.settled = append(t.e.settled, id)

	return nil
}

func (t *fakeTx) Emit(p Publication) { t.e.events = append(t.e.events, p) }

// fakeGateway resolves every post as a valid one on the fake network and counts what it sends.
type fakeGateway struct {
	mu     sync.Mutex
	issues []core.Issue
	cost   int64
	sends  map[string]int
	// fail, when set, fails the next failN sends with it.
	fail  *SendError
	failN int
	// block, when set, holds every send until it is closed.
	block   chan struct{}
	started chan string
}

func newFakeGateway() *fakeGateway {
	return &fakeGateway{sends: map[string]int{}, started: make(chan string, 64)}
}

func (g *fakeGateway) Resolve(_ context.Context, p Post) (*Resolved, error) {
	if p.ConnectionID == "con_missing" {
		return nil, errors.New("the connection does not exist")
	}

	r := &Resolved{
		Network: "fake", NetworkName: "Fake network", Account: "My channel", FileName: "video.mp4", FileBytes: 1234,
		CostMicros: g.cost,
	}
	if g.cost > 0 {
		r.Currency = "USD"
	}

	if p.Strict {
		r.Issues = g.issues

		snap, err := json.Marshal(map[string]string{
			"network": "fake", "title": p.Fields.Title, "description": p.Fields.Description, "export": p.Export,
			"connection": p.ConnectionID, "visibility": p.Fields.Visibility,
		})
		if err != nil {
			return nil, err
		}

		r.Snapshot = snap
	}

	return r, nil
}

func (g *fakeGateway) Send(ctx context.Context, t Target, _ func(float64, string)) (Sent, error) {
	defer activity.ring()

	g.started <- t.ID

	if g.block != nil {
		select {
		case <-g.block:
		case <-ctx.Done():
			return Sent{}, ctx.Err()
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	g.sends[t.ID]++

	if g.fail != nil && g.failN != 0 {
		g.failN--

		return Sent{}, g.fail
	}

	return Sent{RemoteID: "remote-" + t.ID, URL: "https://fake.example/" + t.ID, RemoteState: "published"}, nil
}

func (g *fakeGateway) sent(id string) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.sends[id]
}

func (g *fakeGateway) total() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	n := 0
	for _, c := range g.sends {
		n += c
	}

	return n
}

// fakeClock is a clock the test moves.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []fakeTimer
	env    *fakeEnv
}

type fakeTimer struct {
	at time.Time
	ch chan time.Time
}

func newFakeClock(now time.Time, env *fakeEnv) *fakeClock {
	env.set(now)

	return &fakeClock{now: now, env: env}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	defer activity.ring()

	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan time.Time, 1)
	c.timers = append(c.timers, fakeTimer{at: c.now.Add(d), ch: ch})

	return ch
}

func (c *fakeClock) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.timers)
}

// advance moves the clock, and the service clock with it, firing the timers that came due.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	rest := c.timers[:0]

	for _, tm := range c.timers {
		if tm.at.After(now) {
			rest = append(rest, tm)

			continue
		}

		tm.ch <- now
	}

	c.timers = rest
	c.mu.Unlock()
	c.env.set(now)
}

// bell rings whenever a fake does something a test may be waiting for: a timer armed, a store write, a
// send. It is a broadcast: ringing closes the channel waiters hold and makes a new one.
type bell struct {
	mu sync.Mutex
	ch chan struct{}
}

var activity = &bell{ch: make(chan struct{})}

func (b *bell) ring() {
	b.mu.Lock()
	defer b.mu.Unlock()

	close(b.ch)
	b.ch = make(chan struct{})
}

func (b *bell) channel() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.ch
}

// waitFor waits for a condition to hold, looking again each time the fakes ring, and fails the test when
// its patience ends. It waits for goroutines the test started and has no other way to see them finish.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.After(10 * time.Second)

	for {
		// The channel is taken before the condition is read, so a ring between the two is not lost.
		rung := activity.channel()
		if cond() {
			return
		}

		select {
		case <-rung:
		case <-deadline:
			t.Fatalf("gave up waiting for %s", what)
		}
	}
}
