package webhooks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/webhook"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
)

// The kinds of a delivery.
const (
	// KindEvent is a delivery made by an event: one per webhook and event id.
	KindEvent = "event"
	// KindTest is the "send test" delivery: one attempt.
	KindTest = "test"
	// KindRedelivery is a copy of a finished delivery, sent again at the request of a person.
	KindRedelivery = "redelivery"
)

// Options configure a Dispatcher. Store, Secrets and NewID are required.
type Options struct {
	Store   Store
	Secrets Secrets
	NewID   func(prefix string) string
	// Workspace is the path of the workspace: it makes the opaque id of the body, and never reaches it.
	Workspace string
	Clock     Clock
	Log       *slog.Logger
	// Parallel is the most deliveries sent at once; default 4.
	Parallel int
	// Poll is the longest the loop sleeps; default 30 s.
	Poll time.Duration
	// Timeout is the time one request may take; default core.Timeout.
	Timeout time.Duration
	// Trust is for tests with a name that must turn internal; production leaves it nil.
	Trust *outbound.TestOptions
	// Client builds the client of an address; default outbound.Webhook. Tests replace it.
	Client func(rawURL string) (Doer, error)
}

// Dispatcher turns events into deliveries and sends them (SDD-022 section 4). It never sends in the call of a person:
// Publish and the operations write rows and wake the loop, which Run drives.
type Dispatcher struct {
	o     Options
	wake  chan struct{}
	sem   chan struct{}
	mu    sync.Mutex
	times map[string]struct{}
}

// New makes a dispatcher.
func New(o Options) *Dispatcher {
	if o.Clock == nil {
		o.Clock = systemClock{}
	}

	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}

	if o.Parallel <= 0 {
		o.Parallel = 4
	}

	if o.Poll <= 0 {
		o.Poll = 30 * time.Second
	}

	if o.Timeout <= 0 {
		o.Timeout = core.Timeout
	}

	d := &Dispatcher{
		o: o, wake: make(chan struct{}, 1), sem: make(chan struct{}, o.Parallel), times: map[string]struct{}{},
	}

	if o.Client == nil {
		d.o.Client = func(rawURL string) (Doer, error) { return outbound.Webhook(rawURL, d.o.Timeout, d.o.Trust) }
	}

	return d
}

// Wake makes the loop look at the table now.
func (d *Dispatcher) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Run sends what is due until ctx ends. It sleeps until the next delivery is due, and at most Poll.
func (d *Dispatcher) Run(ctx context.Context) error {
	for {
		sent := d.RunOnce(ctx)

		wait := d.o.Poll

		if next, err := d.o.Store.NextDue(ctx); err == nil && next > 0 {
			if until := time.Duration(next-d.o.Clock.Now().UnixMilli()) * time.Millisecond; until < wait {
				wait = max(until, 0)
			}
		}

		if sent > 0 && wait > time.Second {
			wait = time.Second
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.wake:
		case <-d.o.Clock.After(wait):
		}
	}
}

// RunOnce sends every delivery that is due now, at most Parallel at once, and returns when they have all ended. It
// answers how many it took.
func (d *Dispatcher) RunOnce(ctx context.Context) int {
	due, err := d.o.Store.Due(ctx, d.o.Clock.Now().UnixMilli(), 100)
	if err != nil {
		d.o.Log.WarnContext(ctx, "the due webhook deliveries could not be read", "error", err)

		return 0
	}

	var (
		wg    sync.WaitGroup
		taken int
	)

	for i := range due {
		del := due[i]

		if ctx.Err() != nil {
			break
		}

		if !d.claim(del.ID) {
			continue
		}

		taken++

		wg.Add(1)

		d.sem <- struct{}{}

		go func() {
			defer func() {
				<-d.sem
				d.release(del.ID)
				wg.Done()
			}()

			d.attempt(ctx, del)
		}()
	}

	wg.Wait()

	return taken
}

func (d *Dispatcher) claim(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, busy := d.times[id]; busy {
		return false
	}

	d.times[id] = struct{}{}

	return true
}

func (d *Dispatcher) release(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.times, id)
}

// Publish makes a delivery of the event for every enabled webhook that chose it, and wakes the loop. It writes rows
// and sends nothing. With no webhook it reads one empty list and does no more.
func (d *Dispatcher) Publish(ctx context.Context, data core.Data, at time.Time) {
	ctx = context.WithoutCancel(ctx)

	rows, err := d.o.Store.List(ctx)
	if err != nil {
		d.o.Log.WarnContext(ctx, "the webhooks could not be read", "error", err)

		return
	}

	queued := false

	for i := range rows {
		if !core.Subscribed(rows[i].Events, rows[i].Enabled, data.EventName()) {
			continue
		}

		if _, ok, err := d.Enqueue(ctx, rows[i], data, at, KindEvent); err != nil {
			d.o.Log.WarnContext(ctx, "a webhook delivery could not be stored", "webhook", rows[i].ID, "error", err)
		} else if ok {
			queued = true
		}
	}

	if queued {
		d.Wake()
	}
}

// Enqueue stores a delivery of an event for one webhook. It answers false when the webhook already has this event.
func (d *Dispatcher) Enqueue(
	ctx context.Context, w Row, data core.Data, at time.Time, kind string,
) (Delivery, bool, error) {
	body, eventID, err := core.Build(data, at, core.WorkspaceID(d.o.Workspace))
	if err != nil {
		return Delivery{}, false, err
	}

	now := d.o.Clock.Now().UnixMilli()
	del := Delivery{
		ID: d.o.NewID("wdl"), WebhookID: w.ID, Event: data.EventName(), EventID: eventID, Kind: kind, Body: body,
		State: core.StatePending, MaxAttempts: core.MaxAttempts, NextMs: now, CreatedMs: now,
	}

	if kind == KindTest {
		del.MaxAttempts = 1
	}

	ok, err := d.o.Store.Enqueue(ctx, del)

	return del, ok, err
}

// Redeliver stores a copy of a delivery, to be sent again with a fresh signature.
func (d *Dispatcher) Redeliver(ctx context.Context, src Delivery) (Delivery, error) {
	now := d.o.Clock.Now().UnixMilli()
	del := Delivery{
		ID: d.o.NewID("wdl"), WebhookID: src.WebhookID, Event: src.Event, EventID: src.EventID, Kind: KindRedelivery,
		Body: src.Body, State: core.StatePending, MaxAttempts: core.MaxAttempts, NextMs: now, CreatedMs: now,
	}

	if _, err := d.o.Store.Enqueue(ctx, del); err != nil {
		return Delivery{}, err
	}

	d.Wake()

	return del, nil
}

// Send makes the attempt of one delivery now and answers the delivery as it is after it. It is for the test send,
// where the person waits for the answer; the loop is not involved.
func (d *Dispatcher) Send(ctx context.Context, webhookID, deliveryID string) (Delivery, error) {
	del, err := d.o.Store.GetDelivery(ctx, webhookID, deliveryID)
	if err != nil {
		return Delivery{}, err
	}

	if d.claim(del.ID) {
		defer d.release(del.ID)

		d.attempt(ctx, del)
	}

	del, err = d.o.Store.GetDelivery(ctx, webhookID, deliveryID)
	del.Body = nil

	return del, err
}

// attempt makes one attempt of a delivery and records it.
func (d *Dispatcher) attempt(ctx context.Context, del Delivery) {
	w, err := d.o.Store.Get(ctx, del.WebhookID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			d.o.Log.WarnContext(ctx, "a webhook could not be read", "webhook", del.WebhookID, "error", err)
		}

		return
	}

	secret, err := d.o.Secrets.Secret(ctx, w.ID)
	if err != nil {
		// Not an attempt: the delivery waits and is sent after the unlock.
		d.o.Log.InfoContext(ctx, "a webhook delivery waits for the vault", "webhook", w.ID)

		_ = d.o.Store.Reschedule(
			ctx,
			del.ID,
			d.o.Clock.Now().Add(core.VaultLockedWait).UnixMilli(),
			core.ErrCodeVaultLocked,
		)

		return
	}

	defer clear(secret)

	status, errCode, answered, took := d.send(ctx, w, del, string(secret))
	if ctx.Err() != nil {
		return // shutting down: the delivery stays pending and is tried again at the next start
	}

	outcome := core.Classify(status)

	switch errCode {
	case core.ErrCodeRefusedAddress, core.ErrCodeInvalidURL:
		outcome = core.OutcomeRefused
	}

	if outcome != core.OutcomeDelivered && errCode == "" {
		errCode = core.ErrCodeStatus
	}

	now := d.o.Clock.Now()
	del.Attempts++
	del.LastStatus, del.LastError = status, errCode

	state, wait := core.Next(outcome, del.Attempts, del.MaxAttempts)
	del.State = state

	switch state {
	case core.StatePending:
		del.NextMs = now.Add(wait).UnixMilli()
	default:
		del.FinishedMs = now.UnixMilli()
	}

	att := Attempt{
		N: del.Attempts, AtMs: now.UnixMilli(), Status: status, DurationMs: took.Milliseconds(), Error: errCode,
		ResponseBytes: answered,
	}

	// A write that fails here is logged and the delivery is tried again: it is still pending in the table.
	if err := d.o.Store.Record(
		context.WithoutCancel(ctx),
		del,
		att,
		now.Add(-core.KeepFinished).UnixMilli(),
	); err != nil {
		d.o.Log.WarnContext(ctx, "a webhook attempt could not be recorded", "delivery", del.ID, "error", err)
	}
}

// send makes the request. It answers the HTTP status (0 when there was none), the short code of an error, the size of
// the answer and the time the request took. The body of the answer is read only to count it and is dropped.
func (d *Dispatcher) send(ctx context.Context, w Row, del Delivery, secret string) (int, string, int, time.Duration) {
	if _, ferr := core.CheckURL(w.URL, outbound.NotPublicAddr); ferr != nil {
		return 0, core.ErrCodeInvalidURL, 0, 0
	}

	client, err := d.o.Client(w.URL)
	if err != nil {
		return 0, core.ErrCodeInvalidURL, 0, 0
	}

	ctx, cancel := context.WithTimeout(ctx, d.o.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(del.Body))
	if err != nil {
		return 0, core.ErrCodeInvalidURL, 0, 0
	}

	start := time.Now()

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(core.HeaderSignature, core.Sign(secret, d.o.Clock.Now(), del.Body))
	req.Header.Set(core.HeaderEvent, del.Event)
	req.Header.Set(core.HeaderDelivery, del.ID)
	req.Header.Set("User-Agent", core.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return 0, errorCode(err), 0, time.Since(start)
	}

	n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, core.MaxAnswerBytes))
	_ = resp.Body.Close()

	return resp.StatusCode, "", int(n), time.Since(start)
}

// errorCode is the short code of a failure of the request. It never holds the text of the error: that can hold an
// address, and an address can hold a token.
func errorCode(err error) string {
	var netErr net.Error

	switch {
	case errors.Is(err, outbound.ErrNotPublic), errors.Is(err, outbound.ErrNotLoopback),
		errors.Is(err, outbound.ErrWebhookURL):
		return core.ErrCodeRefusedAddress
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return core.ErrCodeTimeout
	}

	return core.ErrCodeConnect
}
