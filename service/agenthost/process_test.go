package agenthost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// testEnv is a host over a memory store, with the fake agent as the program.
type testEnv struct {
	host  *Host
	store *memStore
	dir   string
	n     atomic.Int64
}

// fakeTokens is a TokenIssuer that counts what it issued and revoked.
type fakeTokens struct {
	issued, revoked atomic.Int64
	last            atomic.Value
}

func (f *fakeTokens) Issue(g Grant, _ time.Time) (string, func(), error) {
	f.issued.Add(1)
	f.last.Store(g)

	return "stc_test_" + g.SessionID, func() { f.revoked.Add(1) }, nil
}

func newEnv(t *testing.T, lim Limits, mutate func(*Options)) *testEnv {
	t.Helper()

	e := &testEnv{store: newMemStore(), dir: t.TempDir()}

	o := Options{Store: e.store, Workspace: e.dir, Limits: lim, Version: "test"}
	if mutate != nil {
		mutate(&o)
	}

	e.host = New(o)

	// A test that leaves a session behind must not leave its child behind.
	t.Cleanup(e.host.Shutdown)

	return e
}

func (e *testEnv) profile(mode string, extra ...string) Profile {
	return Profile{Name: mode, Command: fakeAgentPath, Args: append([]string{"--mode=" + mode}, extra...)}
}

func (e *testEnv) request(project string, p Profile) StartRequest {
	id := "cht_" + strconv.FormatInt(1000000000+e.n.Add(1), 10)

	return StartRequest{SessionID: id, ProjectID: project, Profile: p, StartedBy: "local:ui"}
}

func (e *testEnv) start(t *testing.T, project string, p Profile) *Session {
	t.Helper()

	s, err := e.host.Start(context.Background(), e.request(project, p))
	if err != nil {
		t.Fatalf("Start(%s) = %v", p.Name, err)
	}

	return s
}

// events follows a session and finds the first event that matches.
type follower struct {
	t    *testing.T
	sub  *Subscription
	seen []chat.Event
	pos  int
}

func follow(t *testing.T, h *Host, id string) *follower {
	t.Helper()

	sub, err := h.Subscribe(context.Background(), id, 0)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(sub.Close)

	return &follower{t: t, sub: sub, seen: append([]chat.Event(nil), sub.Snapshot...)}
}

// next returns the first event, from where the follower stopped, that matches; it fails after a
// time limit that no healthy test comes near.
func (f *follower) next(match func(chat.Event) bool) chat.Event {
	f.t.Helper()

	deadline := time.After(30 * time.Second)

	for {
		for ; f.pos < len(f.seen); f.pos++ {
			if match(f.seen[f.pos]) {
				f.pos++
				return f.seen[f.pos-1]
			}
		}

		select {
		case ev, ok := <-f.sub.Events:
			if !ok {
				f.t.Fatalf(
					"the stream closed before the event came; %d events seen, the last %s",
					len(f.seen),
					tail(f.seen),
				)
			}

			f.seen = append(f.seen, ev)
		case <-deadline:
			f.t.Fatalf("the event never came; %d events seen, the last %s", len(f.seen), tail(f.seen))
		}
	}
}

// tail describes the last events, for a failure message.
func tail(evs []chat.Event) string {
	var b strings.Builder

	for _, e := range evs[max(0, len(evs)-3):] {
		b.WriteString(string(e.Kind) + " ")

		if e.State != nil {
			b.WriteString(string(e.State.State) + "/" + string(e.State.EndReason) + " ")
		}
	}

	return b.String()
}

// ended waits for the session to be over, whatever the stream did.
func (e *testEnv) ended(t *testing.T, s *Session) *Session {
	t.Helper()

	e.host.mu.Lock()
	live := e.host.live[s.ID]
	e.host.mu.Unlock()

	if live != nil {
		select {
		case <-live.done:
		case <-time.After(30 * time.Second):
			t.Fatal("the session did not end")
		}
	}

	row, err := e.store.SessionGet(context.Background(), s.ID)
	if err != nil {
		t.Fatal(err)
	}

	return row
}

func state(s chat.State) func(chat.Event) bool {
	return func(e chat.Event) bool { return e.Kind == chat.KindState && e.State.State == s }
}

func kind(k chat.Kind) func(chat.Event) bool { return func(e chat.Event) bool { return e.Kind == k } }

func gone(t *testing.T, pid int) {
	t.Helper()

	// The operating system tells nothing when a process that is not ours dies, so it is
	// asked until the time limit.
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()

	limit := time.After(15 * time.Second)

	for alive(pid) {
		select {
		case <-tick.C:
		case <-limit:
			t.Fatalf("process %d is still running", pid)
		}
	}
}

func TestProcessStartTurnAndEnd(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("echo"))

	if s.State != chat.StateIdle {
		t.Fatalf("state = %s", s.State)
	}

	if _, err := os.Stat(e.host.Dir("prj_a")); err != nil {
		t.Errorf("the working directory was not made: %v", err)
	}

	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "make it shorter"); err != nil {
		t.Fatal(err)
	}

	f.next(kind(chat.KindUserMessage))
	msg := f.next(kind(chat.KindAgentMessage))

	if !strings.HasPrefix(msg.Text, "echo: ") {
		t.Errorf("first chunk = %q", msg.Text)
	}

	call := f.next(func(e chat.Event) bool {
		return e.Kind == chat.KindToolCall && e.Tool.Status == chat.ToolCompleted
	})
	if call.Tool.Title != "Reading the project" || call.Tool.Output != "done" {
		t.Errorf("tool = %+v", call.Tool)
	}

	f.next(state(chat.StateIdle))

	// The second turn is not allowed while the first runs; here the first ended.
	if err := e.host.Send(s.ID, "again"); err != nil {
		t.Fatal(err)
	}

	f.next(state(chat.StateIdle))

	e.host.End(s.ID, chat.EndUser)

	if e.host.Live(s.ID) {
		t.Error("the session is still live")
	}

	row, err := e.store.SessionGet(context.Background(), s.ID)
	if err != nil {
		t.Fatal(err)
	}

	if row.State != chat.StateEnded || row.EndReason != chat.EndUser || row.Title != "make it shorter" {
		t.Errorf("row = %+v", row)
	}

	stored, _ := e.store.EventList(context.Background(), s.ID, 0, 1000)

	var texts []string

	for _, ev := range stored {
		if ev.Kind == chat.KindAgentMessage {
			texts = append(texts, ev.Text)
		}
	}

	if len(texts) != 2 || texts[0] != "echo: make it shorter" || texts[1] != "echo: again" {
		t.Errorf("the stored messages = %q", texts)
	}

	last := stored[len(stored)-1]
	if last.Kind != chat.KindState || last.State.State != chat.StateEnded {
		t.Errorf("the last event = %+v", last)
	}

	for i, ev := range stored {
		if ev.Seq != int64(i+1) {
			t.Fatalf("event %d has seq %d", i, ev.Seq)
		}
	}
}

func TestProcessSendIsRefusedWhileATurnRuns(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("cancellable"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	f.next(kind(chat.KindAgentMessage))

	if err := e.host.Send(s.ID, "go again"); !errors.Is(err, chat.ErrBusy) {
		t.Errorf("Send during a turn = %v", err)
	}

	// Cancel is the way out: the agent confirms and the session is idle again.
	if err := e.host.Cancel(s.ID, false); err != nil {
		t.Fatal(err)
	}

	f.next(kind(chat.KindNotice))
	f.next(state(chat.StateIdle))

	if err := e.host.Send(s.ID, "after the cancel"); err != nil {
		t.Errorf("Send after a cancel = %v", err)
	}

	f.next(kind(chat.KindUserMessage))
}

func TestProcessCancelOnAnIdleSessionChangesNothing(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("echo"))

	if err := e.host.Cancel(s.ID, false); err != nil {
		t.Errorf("Cancel on an idle session = %v", err)
	}

	if !e.host.Live(s.ID) {
		t.Error("a cancel ended the session")
	}

	if err := e.host.Cancel(s.ID, true); err != nil {
		t.Fatal(err)
	}

	if e.host.Live(s.ID) {
		t.Error("an end left the session live")
	}

	if err := e.host.Send(s.ID, "x"); !errors.Is(err, ErrNoSession) {
		t.Errorf("Send after the end = %v", err)
	}
}

func TestProcessTurnTimeLimitKillsAnAgentThatIgnoresTheCancel(t *testing.T) {
	t.Parallel()

	e := newEnv(
		t,
		Limits{Turn: 200 * time.Millisecond, CancelGrace: 200 * time.Millisecond, InputClose: 100 * time.Millisecond},
		nil,
	)
	s := e.start(t, "prj_a", e.profile("hang-turn"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	end := f.next(state(chat.StateEnded))
	if end.State.EndReason != chat.EndTimeout {
		t.Errorf("reason = %s", end.State.EndReason)
	}

	e.ended(t, s)

	if e.host.Live(s.ID) {
		t.Error("the session is still live")
	}
}

func TestProcessStartTimeLimit(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{Start: 400 * time.Millisecond, InputClose: 100 * time.Millisecond}, nil)

	_, err := e.host.Start(context.Background(), e.request("prj_a", e.profile("hang-start")))

	var se *StartError
	if !errors.As(err, &se) || se.Reason != ReasonTimeout {
		t.Fatalf("Start = %v", err)
	}

	if e.host.LiveCount() != 0 {
		t.Error("a failed start left a live session")
	}

	rows, _ := e.store.SessionList(context.Background(), "prj_a")
	if len(rows) != 1 || rows[0].State != chat.StateEnded || rows[0].EndReason != chat.EndStartFailed {
		t.Errorf("rows = %+v", rows)
	}
}

func TestProcessStartFailures(t *testing.T) {
	t.Parallel()

	tokens := &fakeTokens{}
	e := newEnv(t, Limits{}, func(o *Options) {
		o.Tokens = tokens
		o.MCPURL = func() string { return "http://127.0.0.1:7878/mcp" }
	})

	cases := []struct {
		name    string
		profile Profile
		reason  string
	}{
		{
			"the program does not exist",
			Profile{Name: "x", Command: filepath.Join(e.dir, "nothing-here")},
			ReasonNotFound,
		},
		{"the program exits at once", e.profile("exit-start"), ReasonExited},
		{"the protocol version is not 1", e.profile("bad-version"), ReasonProtocolVersion},
		{"no http mcp", e.profile("no-http"), ReasonNoHTTPMCP},
		{"a sign-in is needed", e.profile("sign-in"), ReasonSignIn},
		{"a line that is not json", e.profile("badline"), ReasonProtocol},
	}

	for _, c := range cases {
		_, err := e.host.Start(context.Background(), e.request("prj_a", c.profile))

		var se *StartError
		if !errors.As(err, &se) || se.Reason != c.reason {
			t.Errorf("%s: Start = %v, want reason %s", c.name, err, c.reason)
		}
	}

	if e.host.LiveCount() != 0 {
		t.Error("a failed start left a live session")
	}

	// Every token that was issued was revoked again.
	if tokens.issued.Load() != tokens.revoked.Load() {
		t.Errorf("issued %d tokens, revoked %d", tokens.issued.Load(), tokens.revoked.Load())
	}
}

func TestProcessAnAgentThatExitsDuringATurnEndsTheSession(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("exit-turn"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	end := f.next(state(chat.StateEnded))
	if end.State.EndReason != chat.EndExited {
		t.Errorf("reason = %s", end.State.EndReason)
	}
}

func TestProcessFloodEndsTheSession(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("flood"))

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	row := e.ended(t, s)
	if row.EndReason != chat.EndFlood {
		t.Errorf("reason = %s", row.EndReason)
	}

	if row.EventCount > chat.MaxEventsPerChat+1 {
		t.Errorf("the transcript holds %d events", row.EventCount)
	}
}

func TestFloodMeter(t *testing.T) {
	t.Parallel()

	now := time.Unix(1000, 0)
	m := floodMeter{now: func() time.Time { return now }}

	burst := func(n int) bool {
		var flooding bool

		for range n {
			flooding = m.Hit()
		}

		return flooding
	}

	// A burst inside one second is not a flood; neither are four seconds of it.
	for range chat.FloodSeconds - 1 {
		if burst(chat.FloodEventsPerSecond + 50) {
			t.Fatal("flooding before the seconds have passed")
		}

		now = now.Add(time.Second)
	}

	// The fifth second in a row is.
	if !burst(chat.FloodEventsPerSecond + 50) {
		t.Error("five busy seconds in a row are not a flood")
	}

	// A quiet second resets the streak.
	now = now.Add(time.Second)
	burst(5)
	now = now.Add(time.Second)

	if burst(chat.FloodEventsPerSecond + 50) {
		t.Error("the streak survived a quiet second")
	}

	// A gap resets it too.
	for range chat.FloodSeconds - 1 {
		now = now.Add(time.Second)
		burst(chat.FloodEventsPerSecond + 50)
	}

	now = now.Add(5 * time.Second)

	if burst(chat.FloodEventsPerSecond + 50) {
		t.Error("the streak survived a gap")
	}
}

func TestProcessIdleSessionEnds(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{Idle: 300 * time.Millisecond, InputClose: 100 * time.Millisecond}, nil)
	s := e.start(t, "prj_a", e.profile("echo"))
	f := follow(t, e.host, s.ID)

	end := f.next(state(chat.StateEnded))
	if end.State.EndReason != chat.EndIdle {
		t.Errorf("reason = %s", end.State.EndReason)
	}
}

func TestProcessLifetimeEndsTheSession(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{Lifetime: 300 * time.Millisecond, InputClose: 100 * time.Millisecond}, nil)
	s := e.start(t, "prj_a", e.profile("echo"))
	f := follow(t, e.host, s.ID)

	end := f.next(state(chat.StateEnded))
	if end.State.EndReason != chat.EndTimeout {
		t.Errorf("reason = %s", end.State.EndReason)
	}
}

func TestProcessOneSessionPerProjectAndALimitOfLiveSessions(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{MaxLive: 2}, nil)

	first := e.start(t, "prj_a", e.profile("echo"))
	second := e.start(t, "prj_a", e.profile("echo"))

	if e.host.Live(first.ID) || !e.host.Live(second.ID) {
		t.Error("the new session of a project did not replace the old one")
	}

	row, _ := e.store.SessionGet(context.Background(), first.ID)
	if row.EndReason != chat.EndUser {
		t.Errorf("the replaced session ended with %s", row.EndReason)
	}

	e.start(t, "prj_b", e.profile("echo"))

	_, err := e.host.Start(context.Background(), e.request("prj_c", e.profile("echo")))

	var se *StartError
	if !errors.As(err, &se) || se.Reason != ReasonLimit {
		t.Errorf("a third live session = %v", err)
	}
}

func TestProcessPermissionRequestWaitsForAPerson(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("permission"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	ask := f.next(kind(chat.KindPermissionAsked))
	f.next(state(chat.StateAwaitingPermission))

	p := ask.Permission
	if p.Title != "Delete every clip <b>now</b>" || len(p.Options) != 3 || !strings.HasPrefix(p.RequestID, "cpr_") {
		t.Errorf("permission = %+v", p)
	}

	// A message cannot be sent while the agent waits.
	if err := e.host.Send(s.ID, "x"); !errors.Is(err, chat.ErrBusy) {
		t.Errorf("Send while waiting = %v", err)
	}

	if err := e.host.Answer(s.ID, "cpr_unknown", chat.AnswerAllow); !errors.Is(err, ErrNoRequest) {
		t.Errorf("Answer to an unknown request = %v", err)
	}

	if err := e.host.Answer(s.ID, p.RequestID, chat.Answer("always")); !errors.Is(err, chat.ErrAnswer) {
		t.Errorf("Answer with a standing choice = %v", err)
	}

	if err := e.host.Answer(s.ID, p.RequestID, chat.AnswerAllow); err != nil {
		t.Fatal(err)
	}

	if err := e.host.Answer(s.ID, p.RequestID, chat.AnswerAllow); !errors.Is(err, ErrAnswered) {
		t.Errorf("a second answer = %v", err)
	}

	f.next(kind(chat.KindPermissionAnswer))

	// The agent saw the one-time option, never the standing one.
	msg := f.next(kind(chat.KindAgentMessage))
	if msg.Text != "permission: selected once" {
		t.Errorf("the agent was told %q", msg.Text)
	}

	f.next(state(chat.StateIdle))
}

func TestProcessUnansweredPermissionIsCancelled(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{Permission: 200 * time.Millisecond}, nil)
	s := e.start(t, "prj_a", e.profile("permission"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	msg := f.next(kind(chat.KindAgentMessage))
	if msg.Text != "permission: cancelled " {
		t.Errorf("the agent was told %q", msg.Text)
	}
}

func TestProcessCancellingATurnCancelsItsPermissionRequests(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("permission"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	f.next(kind(chat.KindPermissionAsked))

	if err := e.host.Cancel(s.ID, false); err != nil {
		t.Fatal(err)
	}

	msg := f.next(kind(chat.KindAgentMessage))
	if msg.Text != "permission: cancelled " {
		t.Errorf("the agent was told %q", msg.Text)
	}
}

func TestProcessEnvironmentIsAnAllowlist(t *testing.T) {
	t.Parallel()

	environ := []string{
		"PATH=" + os.Getenv(
			"PATH",
		),
		"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"),
		"HOME=/home/x",
		"LC_ALL=C",
		"XDG_DATA_HOME=/x",
		"Temp=" + os.TempDir(),
		"STUARTY_HOME=/secret",
		"STUARTY_MASTER_KEY=aa",
		"STUARTY_KEY=vvk_x",
		"APP_KEY=k",
		"ANTHROPIC_API_KEY=sk-x",
		"OPENAI_API_KEY=sk-y",
		"AWS_SECRET_ACCESS_KEY=z",
		"GITHUB_TOKEN=t",
		"VUKEM_VIDEO_HOME=/old",
		"MY_PASSWORD=p",
	}

	e := newEnv(t, Limits{}, func(o *Options) { o.Environ = func() []string { return environ } })
	s := e.start(t, "prj_a", e.profile("env"))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	msg := f.next(kind(chat.KindAgentMessage))
	names := strings.Split(strings.TrimPrefix(msg.Text, "env: "), ",")

	for _, n := range names {
		up := strings.ToUpper(n)

		// A Windows child may be given a few variables by the system itself; none of them may
		// be one of ours.
		if strings.HasPrefix(up, "STUARTY") || strings.Contains(up, "KEY") || strings.Contains(up, "TOKEN") ||
			strings.Contains(up, "SECRET") || strings.Contains(up, "PASSWORD") || strings.HasPrefix(up, "VUKEM") {
			t.Errorf("the agent received %s", n)
		}
	}

	for _, want := range []string{"PATH", "HOME", "LC_ALL", "XDG_DATA_HOME"} {
		found := false

		for _, n := range names {
			found = found || strings.EqualFold(n, want)
		}

		if !found {
			t.Errorf("the agent did not receive %s: %v", want, names)
		}
	}
}

func TestEnvironmentFunction(t *testing.T) {
	t.Parallel()

	got := Environment([]string{
		"Path=/bin",
		"path=/other",
		"STUARTY_ADDR=x",
		"NOEQUALS",
		"=x",
		"ProgramFiles(x86)=C:\\p",
		"LC_CTYPE=C",
		"xdg_cache_home=/c",
		"APP_KEY=k",
	})

	want := []string{"Path=/bin", "path=/other", "ProgramFiles(x86)=C:\\p", "LC_CTYPE=C", "xdg_cache_home=/c"}

	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Environment = %q, want %q", got, want)
	}
}

func TestProcessEndKillsTheWholeTree(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{InputClose: 200 * time.Millisecond}, nil)
	pidfile := filepath.Join(t.TempDir(), "child.pid")
	s := e.start(t, "prj_a", e.profile("spawn", "--pidfile="+pidfile))
	f := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "go"); err != nil {
		t.Fatal(err)
	}

	f.next(kind(chat.KindAgentMessage))

	pid := readPid(t, pidfile)
	if !alive(pid) {
		t.Fatal("the grandchild is not running before the end")
	}

	e.host.End(s.ID, chat.EndUser)
	gone(t, pid)
}

func TestProcessShutdownLeavesNoChild(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{InputClose: 200 * time.Millisecond}, nil)

	var pids []int

	for _, project := range []string{"prj_a", "prj_b", "prj_c"} {
		pidfile := filepath.Join(t.TempDir(), "child.pid")
		s := e.start(t, project, e.profile("spawn", "--pidfile="+pidfile))
		f := follow(t, e.host, s.ID)

		if err := e.host.Send(s.ID, "go"); err != nil {
			t.Fatal(err)
		}

		f.next(kind(chat.KindAgentMessage))

		pids = append(pids, readPid(t, pidfile))
	}

	// One more that never finishes its turn.
	hung := e.start(t, "prj_d", e.profile("hang-turn"))
	_ = e.host.Send(hung.ID, "go")

	e.host.Shutdown()

	if e.host.LiveCount() != 0 {
		t.Errorf("%d sessions are live after the shutdown", e.host.LiveCount())
	}

	for _, pid := range pids {
		gone(t, pid)
	}

	// The sessions are recorded as ended by the shutdown.
	for _, project := range []string{"prj_a", "prj_b", "prj_c", "prj_d"} {
		rows, _ := e.store.SessionList(context.Background(), project)
		if len(rows) != 1 || rows[0].State != chat.StateEnded || rows[0].EndReason != chat.EndShutdown {
			t.Errorf("%s: rows = %+v", project, rows)
		}
	}

	// A host that is shutting down starts nothing.
	if _, err := e.host.Start(context.Background(), e.request("prj_e", e.profile("echo"))); err == nil {
		t.Error("a closed host started a session")
	}
}

func TestProcessAnAgentThatIgnoresTheCloseOfItsInputIsKilled(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{InputClose: 150 * time.Millisecond}, nil)
	s := e.start(t, "prj_a", e.profile("deaf"))

	proc := e.host.live[s.ID].proc
	pid := proc.PID()

	started := time.Now()
	e.host.End(s.ID, chat.EndUser)

	if time.Since(started) > 20*time.Second {
		t.Error("ending took too long")
	}

	gone(t, pid)
}

func TestProcessAStartedHostClosesTheSessionsOfADeadServer(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	_ = store.SessionCreate(
		context.Background(),
		Session{ID: "cht_0000000001", ProjectID: "prj_a", State: chat.StateRunning},
	)
	_ = store.SessionCreate(
		context.Background(),
		Session{ID: "cht_0000000002", ProjectID: "prj_a", State: chat.StateEnded, EndReason: chat.EndUser},
	)

	New(Options{Store: store, Workspace: t.TempDir()})

	a, _ := store.SessionGet(context.Background(), "cht_0000000001")
	b, _ := store.SessionGet(context.Background(), "cht_0000000002")

	if a.State != chat.StateEnded || a.EndReason != chat.EndShutdown || b.EndReason != chat.EndUser {
		t.Errorf("a = %+v, b = %+v", a, b)
	}
}

func TestProcessSubscriptionIsAnExactReplayAndThenLive(t *testing.T) {
	t.Parallel()

	e := newEnv(t, Limits{}, nil)
	s := e.start(t, "prj_a", e.profile("echo"))
	first := follow(t, e.host, s.ID)

	if err := e.host.Send(s.ID, "one"); err != nil {
		t.Fatal(err)
	}

	first.next(state(chat.StateIdle))
	first.next(state(chat.StateIdle))

	// A client that joins late is given the same events as one that was there from the start.
	late, err := e.host.Subscribe(context.Background(), s.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer late.Close()

	var wantSeq []int64

	for _, ev := range first.seen {
		if ev.Append {
			continue
		}

		wantSeq = append(wantSeq, ev.Seq)
	}

	var gotSeq []int64
	for _, ev := range late.Snapshot {
		gotSeq = append(gotSeq, ev.Seq)
	}

	if len(gotSeq) == 0 || gotSeq[len(gotSeq)-1] != wantSeq[len(wantSeq)-1] {
		t.Errorf("late snapshot = %v, live = %v", gotSeq, wantSeq)
	}

	// A client that reconnects after event 3 gets 4 and on, and nothing it already has.
	after, err := e.host.Subscribe(context.Background(), s.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()

	for _, ev := range after.Snapshot {
		if ev.Seq <= 3 {
			t.Errorf("event %d was sent again", ev.Seq)
		}
	}

	// The text of the message in the snapshot is whole.
	var text string

	for _, ev := range late.Snapshot {
		if ev.Kind == chat.KindAgentMessage {
			text = ev.Text
		}
	}

	if text != "echo: one" {
		t.Errorf("the text in the snapshot = %q", text)
	}

	e.host.End(s.ID, chat.EndUser)

	// The end closes the stream.
	select {
	case _, ok := <-late.Events:
		for ok {
			_, ok = <-late.Events
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the stream did not close")
	}
}

func readPid(t *testing.T, path string) int {
	t.Helper()

	// The fake agent writes the file before it ends the turn, and the tests read it after the
	// turn ended.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the pid file %s was not written: %v", path, err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("the pid file %s is not a pid: %v", path, err)
	}

	return pid
}
