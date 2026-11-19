package events

import (
	"context"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	wire "github.com/AlexGalhardo/stuarty/service/events"

	"github.com/AlexGalhardo/vukem/sse"
	"github.com/AlexGalhardo/vukem/web"
)

// publicationsTopic is the stream of the calendar (SDD-020 section 21): every publication, for local mode and
// for an administrator. A user of network mode who is not an administrator has a topic of their own that
// carries their publications only.
const publicationsTopic = "publications"

func publicationsTopicOf(owner string) string {
	if owner == "" {
		return publicationsTopic
	}

	return publicationsTopic + ":u:" + owner
}

// publishPublication hands publication.updated to the stream of everyone who may see it. It never blocks the
// commit that called it.
func (e *Events) publishPublication(ev *service.Event) {
	if ev.Publication == nil {
		return
	}

	f := wire.PublicationFrame(ev)
	frame := sse.Event{Event: f.Event, Data: f.Data}
	ctx := context.Background()

	_ = e.hub.Publish(ctx, publicationsTopic, frame)

	if ev.Owner != "" {
		_ = e.hub.Publish(ctx, publicationsTopicOf(ev.Owner), frame)
	}
}

// ServePublications is GET /publications/events: the live events of the calendar. It has no replay: a client
// lists the publications first, then listens. owner is the user whose publications the caller may see, empty for
// every one; authorized is asked every Revalidate and a false ends the stream.
func (e *Events) ServePublications(c *web.Ctx, owner string, authorized func() bool) error {
	var key string
	if caller, ok := service.CallerFrom(c.Context()); ok {
		key = keyOf(caller.Actor)
	}

	if err := e.acquire(publicationsTopic, key); err != nil {
		return err
	}

	defer e.release(publicationsTopic, key)

	live, unsub, err := e.hub.TrySubscribe(c.Context(), publicationsTopicOf(owner))
	if err != nil {
		return err
	}

	defer unsub()

	return c.SSE(func(s *sse.Stream) error {
		return e.runPublications(s, live, authorized)
	}, sse.Heartbeat(0), sse.WriteTimeout(e.o.WriteTimeout))
}

func (e *Events) runPublications(s *sse.Stream, live <-chan sse.Event, authorized func() bool) error {
	if s.SendEvent(sse.Event{Retry: time.Second}) != nil { // the preamble
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
			// A closed channel is a subscriber dropped as too slow, or a closed hub: the client reconnects.
			if !ok || s.SendEvent(ev) != nil {
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
