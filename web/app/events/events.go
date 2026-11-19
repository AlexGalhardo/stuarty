// Package events carries the event stream of SDD-006: the hub that fans the
// events of the service out per project, and the handler of
// GET /api/v1/projects/{id}/events with its replay from the log, its snapshot
// of jobs and assets, its heartbeat and its limits. What an event looks like
// is service/events; this package only moves it.
package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	wire "github.com/AlexGalhardo/stuarty/service/events"

	"github.com/AlexGalhardo/vukem/sse"
	"github.com/AlexGalhardo/vukem/web"
)

// Defaults of SDD-006 sections 3.1 and 9.
const (
	DefaultHeartbeat    = 15 * time.Second
	DefaultRevalidate   = 60 * time.Second
	DefaultWriteTimeout = 10 * time.Second
	MaxPerProject       = 32
	MaxPerProcess       = 128
	MaxPerKey           = 16
	snapshotWindow      = 60 * time.Second
)

// Store is what the stream reads: the log, the project, the jobs and the assets.
type Store interface {
	Projects() service.ProjectRepo
	Operations() service.OperationRepo
	Jobs() service.JobRepo
	Assets() service.AssetRepo
}

// Options tune the stream; the zero value is the protocol. The intervals are
// fields so that tests need not wait for them.
type Options struct {
	Heartbeat, Revalidate, WriteTimeout time.Duration
	MaxPerProject, MaxPerProcess        int
	// MaxPerKey limits the streams of one API key, in every project (network mode).
	MaxPerKey int
	Now       func() time.Time
	Log       *slog.Logger
	// Drop, when it returns true for a revision, keeps that batch event off the
	// hub: the gap-filling path of section 5.4 is tested with it.
	Drop func(revision int64) bool
	// RedactPaths drops the absolute path of an export from job events: in
	// network mode no caller may learn a server path (SDD-007 section 4.3).
	RedactPaths bool
}

// Events is the hub and the handler.
type Events struct {
	store Store
	hub   *sse.Hub
	o     Options

	mu      sync.Mutex
	perProj map[string]int
	perKey  map[string]int
	total   int
}

// New returns the event hub over a store.
func New(store Store, o Options) *Events {
	if o.Heartbeat <= 0 {
		o.Heartbeat = DefaultHeartbeat
	}

	if o.Revalidate <= 0 {
		o.Revalidate = DefaultRevalidate
	}

	if o.WriteTimeout <= 0 {
		o.WriteTimeout = DefaultWriteTimeout
	}

	if o.MaxPerProject <= 0 {
		o.MaxPerProject = MaxPerProject
	}

	if o.MaxPerProcess <= 0 {
		o.MaxPerProcess = MaxPerProcess
	}

	if o.MaxPerKey <= 0 {
		o.MaxPerKey = MaxPerKey
	}

	if o.Now == nil {
		o.Now = time.Now
	}

	if o.Log == nil {
		o.Log = slog.Default()
	}

	return &Events{store: store, hub: sse.NewHub(nil), o: o, perProj: map[string]int{}, perKey: map[string]int{}}
}

// Close ends every subscription; the owner calls it on shutdown.
func (e *Events) Close() { e.hub.Close() }

// Subscribers is the number of streams open on a project (diagnostics and tests).
func (e *Events) Subscribers(project string) int { return e.hub.Subscribers(topic(project)) }

func topic(project string) string { return "project:" + project }

// Publish is service.Options.Publish: it turns an event of the service into its
// frame and hands it to the subscribers of the project. It never blocks the
// commit that called it (a full subscriber is dropped by the hub).
func (e *Events) Publish(ev service.Event) {
	var f wire.Frame

	switch ev.Type {
	case wire.BatchCommitted:
		if e.o.Drop != nil && e.o.Drop(ev.Revision) {
			return
		}

		row, err := e.store.Operations().Get(context.Background(), ev.ProjectID, ev.Revision)
		if err != nil {
			e.o.Log.Warn(
				"event: the row of a committed batch could not be read",
				"project",
				ev.ProjectID,
				"revision",
				ev.Revision,
			)

			return
		}

		if f, err = wire.BatchFrame(&row, ev.Description); err != nil {
			return
		}
	case wire.ProjectUpdated:
		f = wire.ProjectFrame(&ev)
	case wire.AssetUpdated:
		f = wire.AssetFrame(&ev)
	case wire.JobUpdated:
		if e.o.RedactPaths {
			ev = redacted(ev)
		}

		f = wire.JobFrame(&ev)
	default:
		return
	}

	_ = e.hub.Publish(context.Background(), topic(ev.ProjectID), sse.Event{ID: f.ID, Event: f.Event, Data: f.Data})
}

// ParseSince reads the revision a client holds: the Last-Event-ID header wins
// over the since query (section 2). nil means the client gave none.
func ParseSince(header, query string) (*int64, error) {
	raw, field := query, "since"
	if header != "" {
		raw, field = header, "Last-Event-ID"
	}

	if raw == "" {
		return nil, nil
	}

	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return nil, &service.Error{
			Code:    command.CodeValidation,
			Message: field + " must be a revision: a whole number of 0 or more.",
			Path:    field,
		}
	}

	return &n, nil
}

// keyOf is the actor whose streams are counted per key: an API key only.
func keyOf(actor string) string {
	if strings.HasPrefix(actor, "key:") {
		return actor
	}

	return ""
}

func (e *Events) acquire(project, key string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	limit, open := e.o.MaxPerProject, e.perProj[project]
	if e.total >= e.o.MaxPerProcess {
		limit, open = e.o.MaxPerProcess, e.total
	}

	if key != "" && e.perKey[key] >= e.o.MaxPerKey {
		limit, open = e.o.MaxPerKey, e.perKey[key]
	}

	if open >= limit {
		return &service.Error{
			Code: service.CodeQueueFull, Message: "too many event streams are open.", Retryable: true,
			Hint:    "close a stream and try again",
			Details: map[string]any{"limit": "streams", "value": open, "ceiling": limit},
		}
	}

	e.perProj[project]++
	e.total++

	if key != "" {
		e.perKey[key]++
	}

	return nil
}

func (e *Events) release(project, key string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if key != "" {
		e.perKey[key]--

		if e.perKey[key] <= 0 {
			delete(e.perKey, key)
		}
	}

	e.perProj[project]--

	if e.perProj[project] <= 0 {
		delete(e.perProj, project)
	}

	e.total--
}

// Serve is the handler of section 5.4 once the caller is authenticated and the
// project is known to be theirs. since is nil when the client gave no revision.
// authorized is asked every Revalidate; a false ends the stream.
func (e *Events) Serve(c *web.Ctx, project string, since *int64, authorized func() bool) error {
	var key string
	if caller, ok := service.CallerFrom(c.Context()); ok {
		key = keyOf(caller.Actor)
	}

	if err := e.acquire(project, key); err != nil {
		return err
	}

	defer e.release(project, key)

	// Subscribe before reading the head, so that no event falls between the
	// replay and the live events.
	live, unsub, err := e.hub.TrySubscribe(c.Context(), topic(project))
	if err != nil {
		return err
	}

	defer unsub()

	head, err := e.store.Projects().Head(c.Context(), project)
	if err != nil {
		return err
	}

	return c.SSE(func(s *sse.Stream) error {
		return e.run(s, project, since, &head, live, authorized)
	}, sse.Heartbeat(0), sse.WriteTimeout(e.o.WriteTimeout))
}

func send(s *sse.Stream, f wire.Frame) error {
	return s.SendEvent(sse.Event{ID: f.ID, Event: f.Event, Data: f.Data})
}

func (e *Events) run(
	s *sse.Stream,
	project string,
	since *int64,
	head *service.ProjectHead,
	live <-chan sse.Event,
	authorized func() bool,
) error {
	if s.SendEvent(sse.Event{Retry: time.Second}) != nil { // the preamble
		return nil
	}

	c := &conn{e: e, s: s, project: project, fresh: since == nil}
	if since != nil {
		c.last = *since
	}

	action, reason := wire.Decide(since, head.Revision, head.ResyncFloor)

	switch action {
	case wire.Resync:
		_ = send(s, wire.ResyncFrame(project, head.Revision, reason))

		return nil
	case wire.Replay:
		var stop bool

		if c.last, stop = e.replay(s, project, c.last, head.Revision, head.Revision); stop {
			return nil
		}
	case wire.Live, wire.Nothing:
	}

	if e.snapshot(s, project) != nil {
		return nil
	}

	hb, rv := time.NewTicker(e.o.Heartbeat), time.NewTicker(e.o.Revalidate)
	defer hb.Stop()
	defer rv.Stop()

	for {
		select {
		case <-s.Context().Done():
			return nil
		case ev, ok := <-live:
			// A closed channel is a subscriber dropped as too slow, or a closed hub:
			// the client reconnects and replays.
			if !ok || !c.deliver(ev) {
				return nil
			}
		case <-hb.C:
			if s.SendEvent(sse.Event{}) != nil { // the visible heartbeat
				return nil
			}
		case <-rv.C:
			if !authorized() {
				return nil
			}
		}
	}
}

// conn is the position of one client: the last revision it was sent.
type conn struct {
	e       *Events
	s       *sse.Stream
	project string
	last    int64
	// fresh is a client that gave no revision: it takes the stream from where it joined.
	fresh bool
}

// deliver sends one live event and reports whether the stream goes on. A batch
// event is sent in order: one already sent is skipped, and a gap is filled from
// the log first (section 5.4).
func (c *conn) deliver(ev sse.Event) bool {
	f := wire.Frame{ID: ev.ID, Event: ev.Event, Data: ev.Data}
	if ev.Event != wire.BatchCommitted {
		return send(c.s, f) == nil
	}

	rev, _ := strconv.ParseInt(ev.ID, 10, 64)

	if c.fresh {
		c.fresh, c.last = false, rev-1
	}

	if rev <= c.last {
		return true
	}

	if rev > c.last+1 {
		last, stop := c.e.replay(c.s, c.project, c.last, rev-1, rev)
		c.last = last

		if stop {
			return false
		}
	}

	if send(c.s, f) != nil {
		return false
	}

	c.last = rev

	return true
}

// replay sends revisions after from up to to, in pages of 100. It returns the
// last revision sent and true when the stream must end (a resync was sent, or
// the client left).
func (e *Events) replay(s *sse.Stream, project string, from, to, headRev int64) (last int64, stop bool) {
	ctx := s.Context()
	last = from

	describe := e.describer(ctx, project)

	var total int

	for last < to {
		rows, err := e.store.Operations().Since(ctx, project, last, service.OperationFilter{Limit: wire.ReplayPage})
		if err != nil || len(rows) == 0 {
			return last, err != nil
		}

		for i := range rows {
			row := &rows[i]
			if row.Revision > to {
				return last, false
			}

			total += len(row.Ops) + len(row.Inverse) + len(row.Diff)

			switch {
			case !wire.Replayable(row):
				_ = send(s, wire.ResyncFrame(project, headRev, wire.ReasonBeforeFloor))

				return last, true
			case total > wire.MaxReplayBytes || to-from > wire.MaxReplayRevisions:
				_ = send(s, wire.ResyncFrame(project, headRev, wire.ReasonGapTooLarge))

				return last, true
			}

			f, err := wire.BatchFrame(row, describe(row))
			if err != nil || send(s, f) != nil {
				return last, true
			}

			last = row.Revision
		}
	}

	return last, false
}

// describer describes a row with the document as it is now: the log keeps the
// diff, not the text of it, and the frame rate and the order of the tracks are
// what the text needs.
func (e *Events) describer(ctx context.Context, project string) func(*service.OperationRow) string {
	var doc *timeline.Document

	if row, err := e.store.Projects().Get(ctx, project); err == nil {
		doc, _ = timeline.Decode(row.Document)
	}

	return func(r *service.OperationRow) string {
		if doc == nil {
			return ""
		}

		return wire.Describe(doc, r.Diff)
	}
}

// snapshot is section 4.4: the current state of what may have changed while the
// client was away. The events are ordinary ones; a duplicate of a live event is
// harmless because a client keeps the newest per entity.
func (e *Events) snapshot(s *sse.Stream, project string) error {
	recent := e.o.Now().Add(-snapshotWindow).UnixMilli()

	if err := e.snapshotJobs(s, project, recent); err != nil {
		return err
	}

	return e.snapshotAssets(s, project, recent)
}

// The snapshots are best effort: a store that cannot be read leaves the live
// events working, so a read error is not an error of the stream.
func (e *Events) snapshotJobs(s *sse.Stream, project string, recent int64) error {
	jobs, err := e.store.Jobs().List(s.Context(), project, nil, 100)
	if err != nil {
		return nil //nolint:nilerr // see above
	}

	for i := range jobs {
		j := &jobs[i]
		if j.State != service.JobQueued && j.State != service.JobRunning && j.UpdatedMs < recent {
			continue
		}

		if err := send(s, wire.JobFrame(e.jobEvent(project, j))); err != nil {
			return err
		}
	}

	return nil
}

func (e *Events) snapshotAssets(s *sse.Stream, project string, recent int64) error {
	ctx := s.Context()

	assets, err := e.store.Assets().ListProjectAssets(ctx, project)
	if err != nil {
		return nil //nolint:nilerr // see snapshotJobs
	}

	for i := range assets {
		pa := &assets[i]

		ar, err := e.store.Assets().Get(ctx, pa.AssetHash)
		if err != nil {
			continue
		}

		active := ar.IngestState == service.IngestPending || ar.IngestState == service.IngestIngesting
		if !active && ar.UpdatedMs < recent {
			continue
		}

		if active {
			if j, jerr := e.store.Jobs().ActiveIngestForHash(ctx, pa.AssetHash); jerr == nil {
				if err := send(s, wire.JobFrame(e.jobEvent(project, &j))); err != nil {
					return err
				}
			}
		}

		ev := service.Event{
			Type: wire.AssetUpdated, ProjectID: project, AssetID: pa.AssetID, State: string(ar.IngestState),
			Progress: ar.Progress, Error: errorOf(ar.IngestError), At: time.UnixMilli(ar.UpdatedMs),
		}
		if ar.IngestState == service.IngestReady {
			ev.Progress = 1
		}

		if err := send(s, wire.AssetFrame(&ev)); err != nil {
			return err
		}
	}

	return nil
}

// redacted returns ev with a copy of its result whose export path is empty.
func redacted(ev service.Event) service.Event {
	if ev.Result == nil || ev.Result.Export == nil || ev.Result.Export.Path == "" {
		return ev
	}

	res := *ev.Result
	exp := *res.Export
	exp.Path = ""
	res.Export = &exp
	ev.Result = &res

	return ev
}

func (e *Events) jobEvent(project string, j *service.JobRow) *service.Event {
	ev := &service.Event{
		Type: wire.JobUpdated, ProjectID: project, JobID: j.ID, Kind: j.Kind, State: j.State, Progress: j.Progress,
		Message: j.Message, Error: errorOf(j.Error), At: time.UnixMilli(j.UpdatedMs),
	}

	if j.Kind == service.JobExport {
		var res service.JobResult
		if json.Unmarshal(j.Result, &res) == nil && res.Export != nil {
			ev.Result = &res
		}
	}

	if e.o.RedactPaths {
		*ev = redacted(*ev)
	}

	return ev
}

func errorOf(raw json.RawMessage) *service.Error {
	var e service.Error
	if len(raw) == 0 {
		return nil
	}

	// A stored error that does not parse is no error to show.
	if err := json.Unmarshal(raw, &e); err != nil || e.Code == "" {
		return nil //nolint:nilerr // see above
	}

	return service.EventError(&e)
}
