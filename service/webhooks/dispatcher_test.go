package webhooks

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/webhook"
)

type testClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.at
}

func (c *testClock) After(time.Duration) <-chan time.Time { return make(chan time.Time) }

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.at = c.at.Add(d)
}

// oneStore is the table with one webhook and one delivery.
type oneStore struct {
	Store

	mu        sync.Mutex
	del       Delivery
	listed    *Delivery // what Due answers, when it differs from the row
	recordErr error
}

func (s *oneStore) Due(_ context.Context, nowMs int64, _ int) ([]Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listed != nil {
		return []Delivery{*s.listed}, nil
	}

	if s.del.State != core.StatePending || s.del.NextMs > nowMs {
		return nil, nil
	}

	return []Delivery{s.del}, nil
}

func (s *oneStore) Get(context.Context, string) (Row, error) {
	return Row{ID: "whk_a", URL: "https://hooks.example.com/h", Enabled: true}, nil
}

func (s *oneStore) GetDelivery(context.Context, string, string) (Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.del, nil
}

func (s *oneStore) Record(_ context.Context, d Delivery, _ Attempt, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.recordErr != nil {
		return s.recordErr
	}

	s.del = d

	return nil
}

type oneSecret struct{}

func (oneSecret) Secret(context.Context, string) ([]byte, error) { return []byte("whsec_test"), nil }

// countingDoer answers 204 and counts the requests; before, when set, runs inside the request.
type countingDoer struct {
	mu     sync.Mutex
	n      int
	before func()
}

func (c *countingDoer) Do(*http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.n++
	before := c.before
	c.mu.Unlock()

	if before != nil {
		before()
	}

	return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
}

func (c *countingDoer) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.n
}

func newOne(t *testing.T, parallel int) (*Dispatcher, *oneStore, *countingDoer, *testClock) {
	t.Helper()

	clock := &testClock{at: time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)}
	st := &oneStore{del: Delivery{
		ID: "wdl_a", WebhookID: "whk_a", Event: core.ExportReady, Kind: KindEvent, Body: []byte(`{}`),
		State: core.StatePending, MaxAttempts: core.MaxAttempts, NextMs: clock.Now().UnixMilli(),
	}}
	doer := &countingDoer{}
	d := New(Options{
		Store: st, Secrets: oneSecret{}, NewID: func(p string) string { return p + "_x" }, Clock: clock,
		Parallel: parallel, Client: func(string) (Doer, error) { return doer, nil },
	})

	return d, st, doer, clock
}

// A result that cannot be stored leaves the row due. The receiver must not be called again at once, and again, for as
// long as the database cannot be written.
func TestARecordThatFailsDoesNotCallTheReceiverInALoop(t *testing.T) {
	t.Parallel()

	d, st, doer, clock := newOne(t, 4)
	st.recordErr = errors.New("disk full")

	ctx := context.Background()

	for range 5 {
		d.RunOnce(ctx)
	}

	if n := doer.count(); n != 1 {
		t.Fatalf("the receiver was called %d times while the result could not be stored", n)
	}

	clock.advance(unrecordedWait)
	st.mu.Lock()
	st.recordErr = nil
	st.mu.Unlock()

	d.RunOnce(ctx)

	if n := doer.count(); n != 2 {
		t.Fatalf("the delivery was not tried again after the wait: %d", n)
	}

	got, _ := st.GetDelivery(ctx, "whk_a", "wdl_a")
	if got.State != core.StateSucceeded || got.Attempts != 1 {
		t.Fatalf("delivery = %+v", got)
	}
}

// The loop lists a delivery, the test send finishes it, and the list is then stale: it is not sent a second time.
func TestAStaleRowIsNotSentAgain(t *testing.T) {
	t.Parallel()

	d, st, doer, _ := newOne(t, 4)
	stale := st.del
	st.listed = &stale
	st.del.State, st.del.Attempts = core.StateSucceeded, 1

	if d.RunOnce(context.Background()); doer.count() != 0 {
		t.Fatalf("a finished delivery was sent again: %d", doer.count())
	}
}

// The test send takes a slot of the loop, so the limit of sends at once holds for it too.
func TestSendWaitsForASlot(t *testing.T) {
	t.Parallel()

	d, _, doer, _ := newOne(t, 1)
	d.sem <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := d.Send(ctx, "whk_a", "wdl_a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send with every slot taken = %v", err)
	}

	if n := doer.count(); n != 0 {
		t.Fatalf("a send went past the limit: %d", n)
	}
}

// A person who closes the page does not leave a test pending, to be sent again by the loop.
func TestSendWhoseCallerLeftIsStillRecorded(t *testing.T) {
	t.Parallel()

	d, st, doer, _ := newOne(t, 4)
	st.del.Kind, st.del.MaxAttempts = KindTest, 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doer.before = cancel

	got, err := d.Send(ctx, "whk_a", "wdl_a")
	if err != nil || got.State != core.StateSucceeded || got.Attempts != 1 {
		t.Fatalf("Send = %+v, %v", got, err)
	}

	if d.RunOnce(context.Background()); doer.count() != 1 {
		t.Fatalf("the test was sent %d times", doer.count())
	}
}
