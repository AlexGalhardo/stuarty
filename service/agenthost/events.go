package agenthost

import (
	"context"
	"log/slog"
	"strings"
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
	// sent is how much of the text of open the subscribers have: a tail that could be the start of
	// a secret is held back until the next chunk shows whether it is one.
	sent   int
	tools  map[string]*chat.Event
	subs   map[chan chat.Event]struct{}
	closed bool
	full   bool
	// secrets are credentials that were handed to the agent: they are replaced in everything the
	// agent writes, so a transcript never holds one.
	secrets []string
}

func newRecorder(store Store, sid string, now func() time.Time, log *slog.Logger) *recorder {
	return &recorder{
		store: store, sid: sid, now: now, log: log,
		tools: map[string]*chat.Event{}, subs: map[chan chat.Event]struct{}{},
	}
}

// SecretMark replaces a credential in a text.
const SecretMark = "[token]"

// AddSecret makes the recorder keep a credential out of every text it stores or sends.
func (r *recorder) AddSecret(s string) {
	if s == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.secrets = append(r.secrets, s)
}

func (r *recorder) scrub(text string) string {
	for _, s := range r.secrets {
		text = strings.ReplaceAll(text, s, SecretMark)
	}

	return text
}

// Scrub replaces the secrets of the session in a text that is not an event (the diagnostic of a
// failed start).
func (r *recorder) Scrub(text string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.scrub(text)
}

// heldLocked is how many bytes at the end of text could be the start of a secret; the stream holds
// them back, so a secret that an agent splits across chunks never reaches a subscriber whole.
func (r *recorder) heldLocked(text string) int {
	held := 0

	for _, s := range r.secrets {
		for k := min(len(s)-1, len(text)); k > held; k-- {
			if strings.HasSuffix(text, s[:k]) {
				held = k
				break
			}
		}
	}

	return held
}

// scrubEventLocked replaces the secrets in every text of an event the recorder did not build from
// chunks: a notice with the agent's error, a permission request and its options.
func (r *recorder) scrubEventLocked(ev *chat.Event) {
	ev.Text = r.scrub(ev.Text)

	if ev.Permission != nil {
		p := *ev.Permission
		p.Title, p.ToolKind = r.scrub(p.Title), r.scrub(p.ToolKind)

		opts := make([]chat.Option, len(p.Options))
		for i, o := range p.Options {
			opts[i] = chat.Option{ID: r.scrub(o.ID), Name: r.scrub(o.Name), Kind: o.Kind}
		}

		p.Options = opts
		ev.Permission = &p
	}

	if ev.Tool != nil {
		t := *ev.Tool
		t.Title, t.Kind, t.Output = r.scrub(t.Title), r.scrub(t.Kind), r.scrub(t.Output)
		ev.Tool = &t
	}
}

// publishOpenLocked sends what subscribers do not have yet of the event still growing: the whole
// event the first time, then the new text as an append frame, short of a tail that could be the
// start of a secret. With all, the held tail goes too: the event is complete.
func (r *recorder) publishOpenLocked(first, all bool) {
	text := r.open.Text

	visible := len(text)
	if !all {
		visible -= r.heldLocked(text)
	}

	if first {
		ev := clone(*r.open)
		ev.Text = text[:visible]
		r.sent = visible
		r.publish(ev)

		return
	}

	if visible <= r.sent {
		return
	}

	r.publish(chat.Event{Seq: r.open.Seq, Kind: r.open.Kind, At: r.open.At, Text: text[r.sent:visible], Append: true})
	r.sent = visible
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

	// The event is complete: a tail held back as the possible start of a secret was not one.
	r.publishOpenLocked(false, true)
	r.persist(r.open)
	r.open, r.openID, r.sent = nil, "", 0
}

// Flush writes the event that is still growing.
func (r *recorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.flushLocked()
}

// Pending is the event still growing, whole, as a read of the transcript shows it; false when
// there is none. Reading it changes nothing: the next chunk still continues it.
func (r *recorder) Pending() (chat.Event, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.open == nil {
		return chat.Event{}, false
	}

	ev := clone(*r.open)
	ev.Text = ev.Text[:len(ev.Text)-r.heldLocked(ev.Text)]

	return ev, true
}

// emitLocked records a complete event.
func (r *recorder) emitLocked(ev chat.Event) (chat.Event, bool) {
	r.flushLocked()
	r.scrubEventLocked(&ev)

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

	text = r.scrub(chat.CleanText(text))
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

		// A secret may be split across chunks: the whole text is scrubbed again, and the stream
		// is sent only what can no longer turn out to be part of one.
		r.open.Text = r.scrub(r.open.Text + text)
		r.publishOpenLocked(false, false)

		return true
	}

	r.flushLocked()

	text, _ = chat.Clip(text, chat.MaxEventText)
	ev := chat.Event{Kind: kind, Text: text}

	if !r.next(&ev) {
		return false
	}

	r.open, r.openID = &ev, messageID
	r.publishOpenLocked(true, false)

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
			ev.Tool.Title = r.scrub(u.Tool.Title)
		}

		if u.HasTool["kind"] {
			ev.Tool.Kind = r.scrub(u.Tool.Kind)
		}

		if u.HasTool["status"] {
			ev.Tool.Status = u.Tool.Status
		}

		if u.HasTool["output"] {
			ev.Tool.Output = r.scrub(u.Tool.Output)
		}

		r.persist(ev)
		r.publish(*ev)

		return true
	}

	tool := *u.Tool
	tool.Title, tool.Kind, tool.Output = r.scrub(tool.Title), r.scrub(tool.Kind), r.scrub(tool.Output)
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

	// The open event as the other subscribers have it, so the append frames that follow continue
	// it exactly.
	if r.open != nil && (len(snap) == 0 || r.open.Seq > snap[len(snap)-1].Seq) {
		ev := clone(*r.open)
		ev.Text = ev.Text[:r.sent]
		snap = append(snap, ev)
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
