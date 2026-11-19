package agenthost

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// subBuffer is how many frames a subscriber may lag by before it is dropped; it reconnects with
// Last-Event-ID and is sent what it missed.
const subBuffer = 512

// Subscription is a view of a live session: the events already written after a point, and the
// ones to come.
type Subscription struct {
	// Snapshot is the events with seq above the point asked for, oldest first. The event that
	// is still being written is in it with the text it has so far.
	Snapshot []chat.Event
	// Events carries what happens after the snapshot. A message that grows arrives as frames
	// with Append true that continue the event with the same seq. The channel is closed when the
	// session ends or when the subscriber is too slow.
	Events <-chan chat.Event
	cancel func()
}

// Close stops the subscription.
func (s *Subscription) Close() { s.cancel() }

// recorder turns what the agent says into the transcript of one session: it numbers the events,
// coalesces the chunks of a message into one event, keeps the store up to date and feeds the
// live subscribers. Everything an agent wrote is cleaned and clipped here, once.
type recorder struct {
	mu    sync.Mutex
	store Store
	sid   string
	now   func() time.Time
	log   *slog.Logger

	seq    int64
	open   *chat.Event
	openID string
	tools  map[string]*chat.Event
	subs   map[chan chat.Event]struct{}
	closed bool
	full   bool
}

func newRecorder(store Store, sid string, now func() time.Time, log *slog.Logger) *recorder {
	return &recorder{
		store: store, sid: sid, now: now, log: log,
		tools: map[string]*chat.Event{}, subs: map[chan chat.Event]struct{}{},
	}
}

// count is how many events the transcript holds.
func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return int(r.seq)
}

// next assigns the number of a new event; it reports false when the transcript is full, which
// ends the session as a flood (SDD-017 section 4).
func (r *recorder) next(ev *chat.Event) bool {
	if r.closed {
		return false
	}

	if r.seq >= chat.MaxEventsPerChat {
		r.full = true
		return false
	}

	r.seq++
	ev.Seq, ev.At = r.seq, r.now().UTC()

	return true
}

func (r *recorder) persist(ev *chat.Event) {
	if err := r.store.EventUpsert(context.Background(), r.sid, clone(*ev)); err != nil {
		r.log.Warn("a chat event was not stored", "session", r.sid, "error", err)
	}
}

// clone copies an event with what it points to: the recorder goes on changing a tool call after
// it was handed out.
func clone(ev chat.Event) chat.Event {
	if ev.Tool != nil {
		t := *ev.Tool
		ev.Tool = &t
	}

	if ev.Permission != nil {
		p := *ev.Permission
		p.Options = append([]chat.Option(nil), p.Options...)
		ev.Permission = &p
	}

	if ev.State != nil {
		st := *ev.State
		ev.State = &st
	}

	return ev
}

func (r *recorder) publish(ev chat.Event) {
	for ch := range r.subs {
		select {
		case ch <- clone(ev):
		default:
			// Too slow: close it. The client reconnects and is sent the events it missed.
			delete(r.subs, ch)
			close(ch)
		}
	}
}

// flushLocked writes the event that was still growing.
func (r *recorder) flushLocked() {
	if r.open == nil {
		return
	}

	r.persist(r.open)
	r.open, r.openID = nil, ""
}

// Flush writes the event that is still growing.
func (r *recorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.flushLocked()
}

// emitLocked records a complete event.
func (r *recorder) emitLocked(ev chat.Event) (chat.Event, bool) {
	r.flushLocked()

	if !r.next(&ev) {
		return ev, false
	}

	r.persist(&ev)
	r.publish(ev)

	return ev, true
}

// UserMessage records what the person sent.
func (r *recorder) UserMessage(text string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	text, _ = chat.Clip(chat.CleanText(text), chat.MaxEventText)
	_, ok := r.emitLocked(chat.Event{Kind: chat.KindUserMessage, Text: text})

	return ok
}

// Notice records a line the product wrote (a stop reason, an error of the agent).
func (r *recorder) Notice(text string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	text, _ = chat.Clip(chat.CleanText(text), chat.MaxEventText)
	_, ok := r.emitLocked(chat.Event{Kind: chat.KindNotice, Text: text})

	return ok
}

// State records a change of the state of the session.
func (r *recorder) State(info chat.StateInfo) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.emitLocked(chat.Event{Kind: chat.KindState, State: &info})

	return ok
}

// Chunk adds text to the message, thought or plan that is being written, or starts a new one.
func (r *recorder) Chunk(kind chat.Kind, messageID, text string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return false
	}

	text = chat.CleanText(text)
	if text == "" {
		return true
	}

	if r.open != nil && r.open.Kind == kind && r.openID == messageID {
		if len(r.open.Text) >= chat.MaxEventText {
			return true // the rest of an event over its limit is dropped; it ended with the marker
		}

		room := chat.MaxEventText - len(r.open.Text)

		if len(text) > room {
			text, _ = chat.Clip(text, room)
		}

		r.open.Text += text
		r.publish(chat.Event{Seq: r.open.Seq, Kind: kind, At: r.open.At, Text: text, Append: true})

		return true
	}

	r.flushLocked()

	text, _ = chat.Clip(text, chat.MaxEventText)
	ev := chat.Event{Kind: kind, Text: text}

	if !r.next(&ev) {
		return false
	}

	r.open, r.openID = &ev, messageID
	r.publish(ev)

	return true
}

// Tool records a tool call, or merges an update into the one it names. Only the members the
// update carried change.
func (r *recorder) Tool(u Update) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed || u.Tool == nil {
		return !r.closed
	}

	r.flushLocked()

	if ev, ok := r.tools[u.Tool.CallID]; ok {
		if u.HasTool["title"] {
			ev.Tool.Title = u.Tool.Title
		}

		if u.HasTool["kind"] {
			ev.Tool.Kind = u.Tool.Kind
		}

		if u.HasTool["status"] {
			ev.Tool.Status = u.Tool.Status
		}

		if u.HasTool["output"] {
			ev.Tool.Output = u.Tool.Output
		}

		r.persist(ev)
		r.publish(*ev)

		return true
	}

	tool := *u.Tool
	ev := chat.Event{Kind: chat.KindToolCall, Tool: &tool}

	if !r.next(&ev) {
		return false
	}

	r.tools[tool.CallID] = &ev
	r.persist(&ev)
	r.publish(ev)

	return true
}

// Permission records a permission request.
func (r *recorder) Permission(p chat.Permission) (int64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ev, ok := r.emitLocked(chat.Event{Kind: chat.KindPermissionAsked, Permission: &p})

	return ev.Seq, ok
}

// PermissionAnswer records the answer to a permission request.
func (r *recorder) PermissionAnswer(p chat.Permission) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.emitLocked(chat.Event{Kind: chat.KindPermissionAnswer, Permission: &p})

	return ok
}

// Subscribe returns the events after a point and the ones to come, as one step: no event falls
// between them.
func (r *recorder) Subscribe(ctx context.Context, after int64) (*Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Everything but the event still growing is in the store; the open one is added by hand.
	var snap []chat.Event

	for {
		page, err := r.store.EventList(ctx, r.sid, after, chat.MaxTranscriptPage)
		if err != nil {
			return nil, err
		}

		snap = append(snap, page...)

		if len(page) < chat.MaxTranscriptPage {
			break
		}

		after = page[len(page)-1].Seq
	}

	if r.open != nil && (len(snap) == 0 || r.open.Seq > snap[len(snap)-1].Seq) {
		snap = append(snap, clone(*r.open))
	}

	ch := make(chan chat.Event, subBuffer)

	if r.closed {
		close(ch)
	} else {
		r.subs[ch] = struct{}{}
	}

	return &Subscription{Snapshot: snap, Events: ch, cancel: func() { r.unsubscribe(ch) }}, nil
}

func (r *recorder) unsubscribe(ch chan chat.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.subs[ch]; ok {
		delete(r.subs, ch)
		close(ch)
	}
}

// Close writes what is left and ends every subscription.
func (r *recorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.flushLocked()

	if r.closed {
		return
	}

	r.closed = true

	for ch := range r.subs {
		delete(r.subs, ch)
		close(ch)
	}
}

// floodMeter applies the rate rule of SDD-017 section 4: more than chat.FloodEventsPerSecond
// notifications per second for chat.FloodSeconds seconds in a row.
type floodMeter struct {
	now    func() time.Time
	second int64
	count  int
	streak int
}

// Hit counts one notification and reports whether the session is flooding.
func (m *floodMeter) Hit() bool {
	sec := m.now().Unix()

	if sec != m.second {
		if m.second != 0 && sec == m.second+1 && m.count > chat.FloodEventsPerSecond {
			m.streak++
		} else {
			m.streak = 0
		}

		m.second, m.count = sec, 0
	}

	m.count++

	return m.streak >= chat.FloodSeconds-1 && m.count > chat.FloodEventsPerSecond
}
