package agenthost

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// clientView joins the frames of a stream the way the panel does (chatState.ts): an append frame
// continues the text of its event, any other frame replaces it.
func clientView(frames []chat.Event) map[int64]chat.Event {
	view := map[int64]chat.Event{}

	for _, f := range frames {
		if was, ok := view[f.Seq]; ok && f.Append {
			was.Text += f.Text
			view[f.Seq] = was

			continue
		}

		f.Append = false
		view[f.Seq] = f
	}

	return view
}

// eventTexts is every text of an event a person can read.
func eventTexts(ev chat.Event) []string {
	out := []string{ev.Text}

	if ev.Tool != nil {
		out = append(out, ev.Tool.Title, ev.Tool.Kind, ev.Tool.Output)
	}

	if ev.Permission != nil {
		out = append(out, ev.Permission.Title, ev.Permission.ToolKind)

		for _, o := range ev.Permission.Options {
			out = append(out, o.ID, o.Name)
		}
	}

	return out
}

// SDD-017 section 6.1: the token an agent writes back never reaches a transcript or a stream, not
// in a message split across chunks, not in a permission request and not in the error of a turn.
func TestSecurity_T72_TheTokenIsScrubbedWhereverTheAgentWritesIt(t *testing.T) {
	t.Parallel()

	tokens := NewTokens(nil)
	spy := &capturingIssuer{Tokens: tokens}
	e := newEnv(t, Limits{}, func(o *Options) {
		o.Tokens = spy
		o.MCPURL = func() string { return "http://127.0.0.1:7878/mcp" }
	})

	s := e.start(t, "prj_a", e.profile("leak"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	ask := f.next(kind(chat.KindPermissionAsked))
	if !strings.Contains(ask.Permission.Title, SecretMark) {
		t.Errorf("the permission title is %q", ask.Permission.Title)
	}

	// While the request waits, a read of the transcript shows no part of the message it should not.
	if pending, ok := e.host.Pending(s.ID); ok && strings.Contains(pending.Text, spy.secret) {
		t.Error("the pending event holds the token")
	}

	if err := e.host.Answer(s.ID, ask.Permission.RequestID, chat.AnswerCancel); err != nil {
		t.Fatal(err)
	}

	notice := f.next(kind(chat.KindNotice))
	if !strings.Contains(notice.Text, SecretMark) {
		t.Errorf("the notice is %q", notice.Text)
	}

	f.next(state(chat.StateIdle))

	secret := spy.secret
	if secret == "" {
		t.Fatal("no token was issued")
	}

	half := secret[:len(secret)/2]

	view := clientView(f.seen)
	for seq, ev := range view {
		for _, text := range eventTexts(ev) {
			if strings.Contains(text, secret) || strings.Contains(text, half) {
				t.Errorf("event %d (%s) as the panel shows it holds the token: %q", seq, ev.Kind, text)
			}
		}
	}

	// The message the agent split reads whole, with the mark in place of the token.
	var message string

	for _, ev := range view {
		if ev.Kind == chat.KindAgentMessage {
			message = ev.Text
		}
	}

	if message != "my token is "+SecretMark+" and that is all" {
		t.Errorf("the message as the panel shows it is %q", message)
	}

	stored, _ := e.store.EventList(t.Context(), s.ID, 0, 1000)
	for _, ev := range stored {
		for _, text := range eventTexts(ev) {
			if strings.Contains(text, secret) || strings.Contains(text, half) {
				t.Errorf("the stored %s event %d holds the token: %q", ev.Kind, ev.Seq, text)
			}
		}

		if ev.Kind == chat.KindAgentMessage && ev.Text != message {
			t.Errorf("the stored message %q differs from the streamed one %q", ev.Text, message)
		}
	}
}

// An agent often writes right after it answers session/new; the reading goroutine then compares
// the session id while the start is still taking it in. Run with -race.
func TestProcessAnAgentThatSpeaksAtOnceIsReadSafely(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("eager"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	f.next(state(chat.StateIdle))
}

// SDD-017 section 4: the cap on permission requests is on the ones waiting for a person; a long
// session whose requests were all answered still asks the person.
func TestProcessAnsweredPermissionsDoNotCountAgainstTheCap(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{Permission: 10 * time.Millisecond}, nil)
	s := e.start(t, "prj_a", e.profile("permission-many"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	if msg := f.next(kind(chat.KindAgentMessage)); msg.Text != "asked" {
		t.Fatalf("the agent said %q", msg.Text)
	}

	asked := 0

	for _, ev := range f.seen {
		if ev.Kind == chat.KindPermissionAsked {
			asked++
		}
	}

	if asked != 101 {
		t.Errorf("%d of the 101 permission requests reached the person", asked)
	}
}

// SDD-017 section 4: an agent that sends requests faster than they are answered, and stops
// reading the answers, is ended as a protocol failure instead of growing without end.
func TestProcessAnAgentThatFloodsRequestsIsEnded(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{InputClose: 100 * time.Millisecond}, nil)
	s := e.start(t, "prj_a", e.profile("request-flood"))

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	row := e.ended(t, s)
	if row.EndReason != chat.EndProtocol {
		t.Errorf("the session ended with %q", row.EndReason)
	}
}

// gateStore holds the first SessionCreate until the test lets it go.
type gateStore struct {
	*memStore
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (g *gateStore) SessionCreate(ctx context.Context, s Session) error {
	g.once.Do(func() { close(g.entered) })
	<-g.release

	return g.memStore.SessionCreate(ctx, s)
}

// SDD-017 section 4: a session ended while it starts (a second start in its project, a delete, the
// shutdown) never launches its child, so none outlives it.
func TestProcessASessionEndedWhileItStartsLaunchesNoChild(t *testing.T) {
	t.Parallel()

	var launched atomic.Int64

	gate := &gateStore{entered: make(chan struct{}), release: make(chan struct{})}
	e := newEnv(t, Limits{}, func(o *Options) {
		gate.memStore = o.Store.(*memStore)
		o.Store = gate
		o.Environ = func() []string {
			launched.Add(1)

			return os.Environ()
		}
	})

	req := e.request("prj_a", e.profile("echo"))
	done := make(chan error, 1)

	go func() {
		_, err := e.host.Start(context.Background(), req)
		done <- err
	}()

	<-gate.entered
	e.host.End(req.SessionID, chat.EndUser)
	close(gate.release)

	if err := <-done; err == nil {
		t.Error("a session ended while it started reports a start")
	}

	if n := launched.Load(); n != 0 {
		t.Errorf("%d children were launched for a session that had ended", n)
	}

	if e.host.LiveCount() != 0 {
		t.Errorf("%d sessions are live", e.host.LiveCount())
	}

	row, err := e.store.SessionGet(context.Background(), req.SessionID)
	if err != nil || row.State != chat.StateEnded {
		t.Errorf("the row is %+v, %v", row, err)
	}
}
