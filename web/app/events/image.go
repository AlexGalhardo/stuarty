package events

import (
	"context"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	wire "github.com/AlexGalhardo/stuarty/service/events"

	"github.com/AlexGalhardo/vukem/sse"
	"github.com/AlexGalhardo/vukem/web"
)

// The stream of an image document (SDD-006 section 12): the same hub, one topic per image, and the log of the
// image, `image_operations`, for the replay.

func imageTopic(id string) string { return "image:" + id }

// SubscribersOfImage is the number of streams open on an image (diagnostics and tests).
func (e *Events) SubscribersOfImage(id string) int { return e.hub.Subscribers(imageTopic(id)) }

// publishImage hands image.updated to the subscribers of the image. It never blocks the commit that called it.
func (e *Events) publishImage(ev *service.Event) {
	if e.o.Drop != nil && e.o.Drop(ev.Revision) {
		return
	}

	f, err := wire.ImageEventFrame(ev)
	if err != nil {
		return
	}

	_ = e.hub.Publish(context.Background(), imageTopic(ev.ImageID), sse.Event{ID: f.ID, Event: f.Event, Data: f.Data})
}

// ServeImage is the handler of GET /images/{id}/events once the caller is authenticated and the image is known
// to be theirs. since is nil when the client gave no revision. authorized is asked every Revalidate; a false ends
// the stream.
func (e *Events) ServeImage(c *web.Ctx, id string, since *int64, authorized func() bool) error {
	var key string
	if caller, ok := service.CallerFrom(c.Context()); ok {
		key = keyOf(caller.Actor)
	}

	if err := e.acquire(id, key); err != nil {
		return err
	}

	defer e.release(id, key)

	// Subscribe before reading the head, so that no event falls between the replay and the live events.
	live, unsub, err := e.hub.TrySubscribe(c.Context(), imageTopic(id))
	if err != nil {
		return err
	}

	defer unsub()

	row, err := e.store.Images().Get(c.Context(), id)
	if err != nil {
		return err
	}

	return c.SSE(func(s *sse.Stream) error {
		return e.runImage(s, id, since, row.Revision, live, authorized)
	}, sse.Heartbeat(0), sse.WriteTimeout(e.o.WriteTimeout))
}

func (e *Events) runImage(
	s *sse.Stream,
	id string,
	since *int64,
	head int64,
	live <-chan sse.Event,
	authorized func() bool,
) error {
	if s.SendEvent(sse.Event{Retry: time.Second}) != nil { // the preamble
		return nil //nolint:nilerr // a client that cannot be written to has left: the stream ends quietly
	}

	c := &conn{e: e, s: s, project: id, fresh: since == nil, event: wire.ImageUpdated}
	c.fill = func(from, to, headRev int64) (int64, bool) { return e.replayImage(s, id, from, to, headRev) }

	if since != nil {
		c.last = *since
	}

	// An image has no migration or import row, so its log has no floor.
	action, reason := wire.Decide(since, head, 0)

	switch action {
	case wire.Resync:
		_ = send(s, wire.ImageResyncFrame(id, head, reason))

		return nil
	case wire.Replay:
		var stop bool

		if c.last, stop = e.replayImage(s, id, c.last, head, head); stop {
			return nil
		}
	case wire.Live, wire.Nothing:
	}

	return e.loop(s, c, live, authorized)
}

// replayImage sends the revisions after from up to to, in pages of 100, the same frames the live events are. A
// row with an empty diff (the creation of the document, an undo step whose primitives were all skipped) is passed
// over and not sent, as it is not published live. It returns the last revision it passed and true when the stream
// must end (a resync was sent, or the client left).
func (e *Events) replayImage(s *sse.Stream, id string, from, to, headRev int64) (last int64, stop bool) {
	ctx := s.Context()
	last = from

	var total int

	for last < to {
		rows, err := e.store.Images().OpsPage(ctx, id, last, wire.ReplayPage)
		if err != nil || len(rows) == 0 {
			return last, err != nil
		}

		for i := range rows {
			row := &rows[i]
			if row.Revision > to {
				return last, false
			}

			total += len(row.Ops) + len(row.Inverse) + len(row.Diff)

			if total > wire.MaxReplayBytes || to-from > wire.MaxReplayRevisions {
				_ = send(s, wire.ImageResyncFrame(id, headRev, wire.ReasonGapTooLarge))

				return last, true
			}

			last = row.Revision

			if !imageHasChange(row) {
				continue
			}

			f, err := wire.ImageRowFrame(row)
			if err != nil || send(s, f) != nil {
				return last, true
			}
		}
	}

	return last, false
}

// imageHasChange says whether a row of the log is one the stream publishes: its diff is not empty.
func imageHasChange(row *service.ImageOpRow) bool {
	d := row.Diff

	return len(d) > 0 && string(d) != "[]" && string(d) != "null"
}
