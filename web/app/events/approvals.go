package events

import (
	"context"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	wire "github.com/AlexGalhardo/stuarty/service/events"

	"github.com/AlexGalhardo/vukem/sse"
	"github.com/AlexGalhardo/vukem/web"
)

// approvalsTopic is the stream of the approval queue (SDD-015 section 7): every request, for local
// mode and for an administrator. A user of network mode who is not an administrator has a topic of
// their own that carries their requests only.
const approvalsTopic = "approvals"

func approvalsTopicOf(owner string) string {
	if owner == "" {
		return approvalsTopic
	}

	return approvalsTopic + ":u:" + owner
}

// publishApproval hands an approval event to the stream of everyone who may see it. It never blocks the
// commit that called it.
func (e *Events) publishApproval(ev *service.Event) {
	if ev.Approval == nil {
		return
	}

	f := wire.ApprovalFrame(ev)
	frame := sse.Event{Event: f.Event, Data: f.Data}
	ctx := context.Background()

	_ = e.hub.Publish(ctx, approvalsTopic, frame)

	if ev.Owner != "" {
		_ = e.hub.Publish(ctx, approvalsTopicOf(ev.Owner), frame)
	}
}

// ServeApprovals is GET /approvals/events: the requests that are pending as approval.requested, then
// the live events. owner is the user whose requests the caller may see, empty for every request.
// pending lists the requests that wait; authorized is asked every Revalidate and a false ends the stream.
func (e *Events) ServeApprovals(
	c *web.Ctx, owner string, pending func(context.Context) ([]approval.Request, error), authorized func() bool,
) error {
	var key string
	if caller, ok := service.CallerFrom(c.Context()); ok {
		key = keyOf(caller.Actor)
	}

	if err := e.acquire(approvalsTopic, key); err != nil {
		return err
	}

	defer e.release(approvalsTopic, key)

	// Subscribe before reading the queue, so that no event falls between the snapshot and the live events.
	live, unsub, err := e.hub.TrySubscribe(c.Context(), approvalsTopicOf(owner))
	if err != nil {
		return err
	}

	defer unsub()

	rows, err := pending(c.Context())
	if err != nil {
		return err
	}

	return c.SSE(func(s *sse.Stream) error {
		return e.runApprovals(s, rows, live, authorized)
	}, sse.Heartbeat(0), sse.WriteTimeout(e.o.WriteTimeout))
}

func (e *Events) runApprovals(
	s *sse.Stream, rows []approval.Request, live <-chan sse.Event, authorized func() bool,
) error {
	if s.SendEvent(sse.Event{Retry: time.Second}) != nil { // the preamble
		return nil
	}

	for i := range rows {
		f := wire.ApprovalFrame(&service.Event{Type: wire.ApprovalRequested, Approval: &rows[i]})
		if send(s, f) != nil {
			return nil
		}
	}

	hb, rv := time.NewTicker(e.o.Heartbeat), time.NewTicker(e.o.Revalidate)
	defer hb.Stop()
	defer rv.Stop()

	for {
		select {
		case <-s.Context().Done():
			return nil
		case ev, ok := <-live:
			// A closed channel is a subscriber dropped as too slow, or a closed hub: the client
			// reconnects and is sent the queue again.
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
