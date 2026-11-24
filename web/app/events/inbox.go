package events

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/sse"
	"github.com/AlexGalhardo/vukem/web"
)

// inboxTopic carries no data, only the news that a request that waits for a person was added or settled, in the chat
// or in the approval queue. The stream of the unified read (SDD-015 section 4.1) takes the news as a reason to ask
// the service again, as the caller; nothing a member may not see is ever on the topic.
const inboxTopic = "inbox"

// InboxEventName is the name of every frame of the inbox stream; its data is a service.InboxListResult.
const InboxEventName = "inbox.snapshot"

var inboxHint = sse.Event{Event: "inbox.changed", Data: "{}"}

// PublishInbox tells the inbox streams that something changed. It never blocks the caller.
func (e *Events) PublishInbox() {
	_ = e.hub.Publish(context.Background(), inboxTopic, inboxHint)
}

// ServeInbox is GET /inbox/events: the list as the caller reads it, then the list again, whole, each time a request
// is added or settled and the caller's own list changed. snapshot asks the service for the list of the caller;
// authorized is asked every Revalidate and a false ends the stream. A snapshot that fails ends the stream, so a
// caller that lost the right to read is not left on a stream that goes silent.
func (e *Events) ServeInbox(
	c *web.Ctx, snapshot func(context.Context) (*service.InboxListResult, error), authorized func() bool,
) error {
	var key string
	if caller, ok := service.CallerFrom(c.Context()); ok {
		key = keyOf(caller.Actor)
	}

	if err := e.acquire(inboxTopic, key); err != nil {
		return err
	}

	defer e.release(inboxTopic, key)

	// Subscribe before the first read, so that no change falls between the snapshot and the live news.
	live, unsub, err := e.hub.TrySubscribe(c.Context(), inboxTopic)
	if err != nil {
		return err
	}

	defer unsub()

	first, err := snapshot(c.Context())
	if err != nil {
		return err
	}

	return c.SSE(func(s *sse.Stream) error {
		return e.runInbox(c.Context(), s, first, snapshot, live, authorized)
	}, sse.Heartbeat(0), sse.WriteTimeout(e.o.WriteTimeout))
}

func inboxFrame(res *service.InboxListResult) (sse.Event, []byte, error) {
	raw, err := json.Marshal(res)
	if err != nil {
		return sse.Event{}, nil, err
	}

	return sse.Event{Event: InboxEventName, Data: string(raw)}, raw, nil
}

func (e *Events) runInbox(
	ctx context.Context, s *sse.Stream, first *service.InboxListResult,
	snapshot func(context.Context) (*service.InboxListResult, error), live <-chan sse.Event, authorized func() bool,
) error {
	if s.SendEvent(sse.Event{Retry: time.Second}) != nil { // the preamble
		return nil
	}

	frame, sent, err := inboxFrame(first)
	if err != nil || s.SendEvent(frame) != nil {
		return nil
	}

	hb, rv := time.NewTicker(e.o.Heartbeat), time.NewTicker(e.o.Revalidate)
	defer hb.Stop()
	defer rv.Stop()

	for {
		select {
		case <-s.Context().Done():
			return nil
		case _, ok := <-live:
			// A closed channel is a subscriber dropped as too slow, or a closed hub: the client reconnects and is
			// sent the list again.
			if !ok {
				return nil
			}

			// Several changes that arrived together are one read.
			for drained := false; !drained; {
				select {
				case <-live:
				default:
					drained = true
				}
			}

			res, err := snapshot(ctx)
			if err != nil {
				return nil
			}

			next, raw, err := inboxFrame(res)
			if err != nil {
				return nil
			}

			if bytes.Equal(raw, sent) {
				continue
			}

			if s.SendEvent(next) != nil {
				return nil
			}

			sent = raw
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
