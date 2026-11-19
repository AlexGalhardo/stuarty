package agenthost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// SDD-007 T-70: a profile is a program and an array of arguments; nothing is joined into a shell
// line, so metacharacters in the command or in an argument are only characters.
func TestSecurity_T70_ProfileCommandIsAnArgumentArray(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	planted := filepath.Join(t.TempDir(), "planted")

	attacks := []string{
		"; touch " + planted, "&& touch " + planted, "| touch " + planted, "$(touch " + planted + ")",
		"`touch " + planted + "`", "> " + planted, "\n touch " + planted,
	}

	// In an argument: the agent receives the text and nothing runs it.
	args := append([]string{"--mode=echo"}, attacks...)
	s := e.start(t, "prj_a", Profile{Name: "x", Command: fakeAgentPath, Args: args})
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	f.next(kind(chat.KindAgentMessage))
	e.host.End(s.ID, chat.EndUser)

	// In the command: a program with that name does not exist, and nothing runs it either.
	for _, a := range attacks {
		_, err := e.host.Start(
			context.Background(),
			e.request("prj_b", Profile{Name: "y", Command: fakeAgentPath + " " + a}),
		)

		var se *StartError
		if !errors.As(err, &se) || se.Reason != ReasonNotFound {
			t.Errorf("command %q: Start = %v", a, err)
		}
	}

	if _, err := os.Stat(planted); err == nil {
		t.Fatal("a shell ran what was written in a profile")
	}
}

// SDD-007 T-71: what an agent writes is stored and sent as a string; markup in it is kept as
// characters and the characters that reorder or hide text are dropped.
func TestSecurity_T71_AgentTextIsStoredAndSentAsText(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("markup"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	f.next(kind(chat.KindAgentMessage))
	f.next(func(ev chat.Event) bool {
		return ev.Kind == chat.KindState && ev.State.State == chat.StateIdle && ev.Seq > 3
	})

	stored, err := e.store.EventList(context.Background(), s.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}

	var text string

	for _, ev := range stored {
		if ev.Kind == chat.KindAgentMessage {
			text = ev.Text
		}
	}

	if !strings.Contains(text, "<img src=x onerror=alert(1)>") || !strings.Contains(text, "[a](javascript:alert(1))") {
		t.Errorf("the markup was not kept as it was written: %q", text)
	}

	if strings.ContainsRune(text, rune(0x202e)) {
		t.Errorf("a bidirectional override survived: %q", text)
	}

	// Every frame of the live stream carries the same text.
	for _, ev := range f.seen {
		if ev.Kind == chat.KindAgentMessage && strings.ContainsRune(ev.Text, rune(0x202e)) {
			t.Errorf("a frame carries an override: %q", ev.Text)
		}
	}
}

// SDD-007 T-72: the token of a session dies with it, whichever way it ends.
func TestSecurity_T72_SessionTokenDiesWithTheSession(t *testing.T) {
	t.Parallel()

	tokens := NewTokens(nil)
	spy := &capturingIssuer{Tokens: tokens}
	e := newEnv(t, Limits{InputClose: 200 * time.Millisecond}, func(o *Options) {
		o.Tokens = spy
		o.MCPURL = func() string { return "http://127.0.0.1:7878/mcp" }
	})

	ended := func(name string, finish func(id string)) {
		s := e.start(t, "prj_"+name, e.profile("echo"))
		secret := spy.secret

		if _, ok := tokens.Lookup(secret); !ok {
			t.Fatalf("%s: the token of a live session is not found", name)
		}

		finish(s.ID)
		e.ended(t, s)

		if _, ok := tokens.Lookup(secret); ok {
			t.Errorf("%s: the token outlived the session", name)
		}
	}

	ended("user", func(id string) { e.host.End(id, chat.EndUser) })
	ended("cancelend", func(id string) { _ = e.host.Cancel(id, true) })

	// A session that fails to start holds no token either.
	if _, err := e.host.Start(context.Background(), e.request("prj_nohttp", e.profile("no-http"))); err == nil {
		t.Fatal("an agent without http mcp started")
	}

	if _, ok := tokens.Lookup(spy.secret); ok {
		t.Error("a failed start left a token")
	}

	// The shutdown takes the rest.
	live := e.start(t, "prj_shutdown", e.profile("echo"))
	secret := spy.secret

	e.host.Shutdown()

	if _, ok := tokens.Lookup(secret); ok {
		t.Error("the token survived the shutdown")
	}

	_ = live
}

// SDD-007 T-73: an agent that never answers, or never ends a turn, is killed and its session ends.
func TestSecurity_T73_HangingAgentIsKilled(t *testing.T) {
	t.Parallel()

	// It never answers the first message.
	silent := newEnv(t, Limits{Start: 300 * time.Millisecond, InputClose: 100 * time.Millisecond}, nil)
	if _, err := silent.host.Start(
		context.Background(),
		silent.request("prj_a", silent.profile("hang-start")),
	); err == nil {
		t.Fatal("an agent that never answered started")
	}

	if silent.host.LiveCount() != 0 {
		t.Error("a hanging start left a session")
	}

	// It takes a turn and never ends it, ignoring the cancel and the close of its input.
	e := newEnv(
		t,
		Limits{Turn: 200 * time.Millisecond, CancelGrace: 200 * time.Millisecond, InputClose: 100 * time.Millisecond},
		nil,
	)
	s := e.start(t, "prj_b", e.profile("hang-turn"))
	pid := e.host.live[s.ID].proc.PID()

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	row := e.ended(t, s)
	if row.EndReason != chat.EndTimeout {
		t.Errorf("reason = %s", row.EndReason)
	}

	gone(t, pid)
}

// SDD-007 T-73: an agent that writes more than a session may hold ends the session.
func TestSecurity_T73_FloodEndsTheSession(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{InputClose: 100 * time.Millisecond}, nil)
	s := e.start(t, "prj_a", e.profile("flood"))
	pid := e.host.live[s.ID].proc.PID()

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	row := e.ended(t, s)
	if row.EndReason != chat.EndFlood || row.EventCount > chat.MaxEventsPerChat+1 {
		t.Errorf("row = %+v", row)
	}

	gone(t, pid)

	// A line over the limit is a protocol error that ends the session at the start.
	if _, err := e.host.Start(context.Background(), e.request("prj_b", e.profile("longline"))); err == nil {
		t.Error("an agent that wrote a line of 2 MiB started")
	}
}

// SDD-007 T-73: closing the host leaves no child of the host, nor of its agents.
func TestSecurity_T73_ShutdownLeavesNoChild(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{InputClose: 100 * time.Millisecond}, nil)

	var pids []int

	for _, project := range []string{"prj_a", "prj_b"} {
		pidfile := filepath.Join(t.TempDir(), "child.pid")
		s := e.start(t, project, e.profile("spawn", "--pidfile="+pidfile))
		f := follow(t, e.host, s.ID)

		if err := e.host.Send(s.ID, "go"); err != nil {
			t.Fatal(err)
		}

		f.next(kind(chat.KindAgentMessage))

		pids = append(pids, e.host.live[s.ID].proc.PID(), readPid(t, pidfile))
	}

	e.host.Shutdown()

	for _, pid := range pids {
		gone(t, pid)
	}
}
