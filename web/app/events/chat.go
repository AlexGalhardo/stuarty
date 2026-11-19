package events

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/sse"
	"github.com/AlexGalhardo/vukem/web"
)

// ChatEventName is the name of every frame of a chat stream; its data is a chat.Event.
const ChatEventName = "chat.event"

// ServeChat is GET /chat/sessions/{id}/events (SDD-017 section 8): the events already written, then,
// for a live session, the ones to come. replay is the snapshot, live is nil for a session that
// ended (the stream then ends once the replay is sent), and authorized is asked every Revalidate: a
// false ends the stream. The data of a frame is a chat.Event; a frame of a message that is still
// growing carries only the new text and append true, under the id of the event.
func (e *Events) ServeChat(
	c *web.Ctx, id string, replay []chat.Event, live <-chan chat.Event, authorized func() bool,
) error {
	var key string
	if caller, ok := service.CallerFrom(c.Context()); ok {
		key = keyOf(caller.Actor)
	}

	topic := "chat:" + id

	if err := e.acquire(topic, key); err != nil {
		return err
	}

	defer e.release(topic, key)

	return c.SSE(func(s *sse.Stream) error {
		return e.runChat(s, replay, live, authorized)
	}, sse.Heartbeat(0), sse.WriteTimeout(e.o.WriteTimeout))
}

func chatFrame(ev chat.Event) (sse.Event, error) {
	raw, err := json.Marshal(ev)
	if err != nil {
		return sse.Event{}, err
	}

	return sse.Event{ID: strconv.FormatInt(ev.Seq, 10), Event: ChatEventName, Data: string(raw)}, nil
}

func (e *Events) runChat(
	s *sse.Stream, replay []chat.Event, live <-chan chat.Event, authorized func() bool,
) error {
	if s.SendEvent(sse.Event{Retry: time.Second}) != nil { // the preamble
		return nil
	}

	for i := range replay {
		f, err := chatFrame(replay[i])
		if err != nil {
			continue
		}

		if s.SendEvent(f) != nil {
			return nil
		}
	}

	if live == nil {
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
			// A closed channel is the end of the session, or a subscriber dropped as too slow: the
			// client reconnects and is sent what it missed.
			if !ok {
				return nil
			}

			f, err := chatFrame(ev)
			if err != nil {
				continue
			}

			if s.SendEvent(f) != nil {
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
