package servicetest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
)

// The fake agent of the hosted chat is built once for the package, on the first test that needs it,
// and removed when the tests end.
var (
	fakeOnce sync.Once
	fakeDir  string
	fakePath string
	errFake  error
)

func TestMain(m *testing.M) {
	code := m.Run()

	if fakeDir != "" {
		_ = os.RemoveAll(fakeDir)
	}

	os.Exit(code)
}

func fakeAgentBinary(t *testing.T) string {
	t.Helper()

	fakeOnce.Do(func() {
		fakeDir, errFake = os.MkdirTemp("", "stuarty-fakeagent-") //nolint:usetesting // built once for the package
		if errFake != nil {
			return
		}

		name := "fakeagent"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}

		fakePath = filepath.Join(fakeDir, name)

		out, err := exec.CommandContext(context.Background(), "go", "build", "-o", fakePath,
			"github.com/AlexGalhardo/stuarty/service/agenthost/fakeagent").CombinedOutput()
		if err != nil {
			errFake = &buildError{err: err, out: string(out)}
		}
	})

	if errFake != nil {
		t.Fatalf("building the fake agent: %v", errFake)
	}

	return fakePath
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

// chatWorld is a service with a host of the fake agent over the real store.
type chatWorld struct {
	*env
	host   *agenthost.Host
	tokens *agenthost.Tokens
}

func newChatWorld(t *testing.T) *chatWorld {
	t.Helper()

	w := &chatWorld{tokens: agenthost.NewTokens(nil)}

	w.env = newEnvWith(t, func(e *env, o *service.Options) {
		w.host = agenthost.New(agenthost.Options{
			Store: e.store.Chat(), Tokens: w.tokens, Workspace: string(e.dir), Version: "test",
			Limits: agenthost.Limits{InputClose: 200 * time.Millisecond},
		})
		o.Agents = w.host
	})

	// The host is stopped before the store closes: cleanups run last in, first out.
	t.Cleanup(w.host.Shutdown)

	return w
}

// person is a person in a browser session with every scope.
func person() context.Context {
	return service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})
}

func (w *chatWorld) profile(t *testing.T, name, mode string) {
	t.Helper()

	if _, err := w.svc.AgentProfileSet(person(), service.AgentProfileSetRequest{
		Name: name, Command: fakeAgentBinary(t), Args: []string{"--mode=" + mode},
	}); err != nil {
		t.Fatalf("AgentProfileSet = %v", err)
	}
}

func (w *chatWorld) start(t *testing.T, project, profile string) *service.ChatSession {
	t.Helper()

	s, err := w.svc.ChatSessionStart(person(), service.ChatSessionStartRequest{ProjectID: project, Profile: profile})
	if err != nil {
		t.Fatalf("ChatSessionStart = %v", err)
	}

	return s
}

// wait follows a live session until an event matches.
func (w *chatWorld) wait(t *testing.T, id string, match func(chat.Event) bool) chat.Event {
	t.Helper()

	sub, err := w.host.Subscribe(context.Background(), id, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	for _, ev := range sub.Snapshot {
		if match(ev) {
			return ev
		}
	}

	deadline := time.After(30 * time.Second)

	for {
		select {
		case ev, ok := <-sub.Events:
			if !ok {
				t.Fatal("the stream closed before the event came")
			}

			if match(ev) {
				return ev
			}
		case <-deadline:
			t.Fatal("the event never came")
		}
	}
}

func isKind(k chat.Kind) func(chat.Event) bool { return func(e chat.Event) bool { return e.Kind == k } }

func TestChatSessionFromStartToTranscript(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "echo", "echo")

	s := w.start(t, project, "echo")
	if s.State != chat.StateIdle || !s.Live || s.Profile != "echo" || s.ProjectID != project {
		t.Fatalf("session = %+v", s)
	}

	sent, err := w.svc.ChatSend(person(), service.ChatSendRequest{SessionID: s.ID, Text: "cut the intro"})
	if err != nil || sent.State != chat.StateRunning {
		t.Fatalf("ChatSend = %+v, %v", sent, err)
	}

	user := w.wait(t, s.ID, isKind(chat.KindUserMessage))
	w.wait(t, s.ID, func(e chat.Event) bool {
		return e.Kind == chat.KindState && e.State.State == chat.StateIdle && e.Seq > user.Seq
	})

	// The list and the single read agree.
	list, err := w.svc.ChatSessionList(w.as(ui), service.ChatSessionListRequest{ProjectID: project})
	if err != nil || len(list.Sessions) != 1 || list.Sessions[0].ID != s.ID ||
		list.Sessions[0].Title != "cut the intro" {
		t.Fatalf("ChatSessionList = %+v, %v", list, err)
	}

	got, err := w.svc.ChatSessionGet(w.as(ui), service.ChatRef{SessionID: s.ID})
	if err != nil || !got.Live || got.EventCount == 0 {
		t.Fatalf("ChatSessionGet = %+v, %v", got, err)
	}

	// A page of events, the first event the person's own message.
	page, err := w.svc.ChatTranscriptGet(w.as(ui), service.ChatTranscriptRequest{SessionID: s.ID, Limit: 3})
	if err != nil || len(page.Events) != 3 || page.NextAfter == nil || *page.NextAfter != page.Events[2].Seq {
		t.Fatalf("ChatTranscriptGet page = %+v, %v", page, err)
	}

	for i, ev := range page.Events {
		if ev.Seq != int64(i+1) {
			t.Errorf("event %d has seq %d", i, ev.Seq)
		}
	}

	rest, err := w.svc.ChatTranscriptGet(
		w.as(ui),
		service.ChatTranscriptRequest{SessionID: s.ID, After: *page.NextAfter},
	)
	if err != nil || len(rest.Events) == 0 || rest.Events[0].Seq != *page.NextAfter+1 {
		t.Fatalf("second page = %+v, %v", rest, err)
	}

	// The same transcript as Markdown: every text inside a fence.
	md, err := w.svc.ChatTranscriptGet(
		w.as(ui),
		service.ChatTranscriptRequest{SessionID: s.ID, Format: service.ChatFormatMarkdown},
	)
	if err != nil || !strings.Contains(md.Markdown, "# Chat session "+s.ID) ||
		!strings.Contains(
			md.Markdown,
			"cut the intro",
		) || !strings.Contains(md.Markdown, "echo: You are working with the user") {
		t.Fatalf("markdown = %q, %v", md.Markdown, err)
	}

	if _, err := w.svc.ChatTranscriptGet(
		w.as(ui),
		service.ChatTranscriptRequest{SessionID: s.ID, Format: "html"},
	); code(
		err,
	) != "E_VALIDATION" {
		t.Errorf("an unknown format = %v", err)
	}

	// Ending the session keeps the transcript.
	ended, err := w.svc.ChatCancel(person(), service.ChatCancelRequest{SessionID: s.ID, End: true})
	if err != nil || ended.State != chat.StateEnded || ended.Live || ended.EndReason != chat.EndUser {
		t.Fatalf("end = %+v, %v", ended, err)
	}

	if _, err := w.svc.ChatSend(
		person(),
		service.ChatSendRequest{SessionID: s.ID, Text: "again"},
	); code(
		err,
	) != "E_INVALID_STATE" {
		t.Errorf("ChatSend after the end = %v", err)
	}

	after, err := w.svc.ChatTranscriptGet(w.as(ui), service.ChatTranscriptRequest{SessionID: s.ID})
	if err != nil || len(after.Events) == 0 || after.Events[len(after.Events)-1].State.State != chat.StateEnded {
		t.Errorf("the transcript after the end = %+v, %v", after, err)
	}

	// Deleting removes it.
	if _, err := w.svc.ChatSessionDelete(person(), service.ChatRef{SessionID: s.ID}); err != nil {
		t.Fatal(err)
	}

	if _, err := w.svc.ChatSessionGet(w.as(ui), service.ChatRef{SessionID: s.ID}); code(err) != "E_CHAT_NOT_FOUND" {
		t.Errorf("a deleted session = %v", err)
	}
}

func TestChatPermissionRequestIsAnsweredByAPerson(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "ask", "permission")

	s := w.start(t, project, "ask")

	if _, err := w.svc.ChatSend(
		person(),
		service.ChatSendRequest{SessionID: s.ID, Text: "delete the clips"},
	); err != nil {
		t.Fatal(err)
	}

	ask := w.wait(t, s.ID, isKind(chat.KindPermissionAsked))

	// A standing answer does not exist.
	for _, bad := range []string{"always", "allow_always", ""} {
		_, err := w.svc.ChatAnswerPermission(person(), service.ChatAnswerRequest{
			SessionID: s.ID, RequestID: ask.Permission.RequestID, Answer: bad,
		})
		if code(err) != "E_VALIDATION" {
			t.Errorf("answer %q = %v", bad, err)
		}
	}

	if _, err := w.svc.ChatAnswerPermission(person(), service.ChatAnswerRequest{
		SessionID: s.ID, RequestID: "cpr_0000000000", Answer: "allow",
	}); code(err) != "E_CHAT_NOT_FOUND" {
		t.Errorf("an unknown request = %v", err)
	}

	if _, err := w.svc.ChatAnswerPermission(person(), service.ChatAnswerRequest{
		SessionID: s.ID, RequestID: ask.Permission.RequestID, Answer: "allow",
	}); err != nil {
		t.Fatal(err)
	}

	msg := w.wait(t, s.ID, isKind(chat.KindAgentMessage))
	if msg.Text != "permission: selected once" {
		t.Errorf("the agent was told %q", msg.Text)
	}

	if _, err := w.svc.ChatAnswerPermission(person(), service.ChatAnswerRequest{
		SessionID: s.ID, RequestID: ask.Permission.RequestID, Answer: "allow",
	}); code(err) != "E_INVALID_STATE" {
		t.Errorf("a second answer = %v", err)
	}
}

func TestChatAgentTextIsStoredAsText(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "markup", "markup")

	s := w.start(t, project, "markup")
	if _, err := w.svc.ChatSend(person(), service.ChatSendRequest{SessionID: s.ID, Text: "go"}); err != nil {
		t.Fatal(err)
	}

	user := w.wait(t, s.ID, isKind(chat.KindUserMessage))
	w.wait(t, s.ID, func(e chat.Event) bool {
		return e.Kind == chat.KindState && e.State.State == chat.StateIdle && e.Seq > user.Seq
	})

	page, err := w.svc.ChatTranscriptGet(w.as(ui), service.ChatTranscriptRequest{SessionID: s.ID})
	if err != nil {
		t.Fatal(err)
	}

	var text string

	for _, ev := range page.Events {
		if ev.Kind == chat.KindAgentMessage {
			text = ev.Text
		}
	}

	if !strings.Contains(text, "<img src=x onerror=alert(1)>") || strings.ContainsRune(text, rune(0x202e)) {
		t.Errorf("text = %q", text)
	}

	md, err := w.svc.ChatTranscriptGet(
		w.as(ui),
		service.ChatTranscriptRequest{SessionID: s.ID, Format: service.ChatFormatMarkdown},
	)
	if err != nil {
		t.Fatal(err)
	}

	// The hostile text sits inside a fence longer than its own backticks.
	if !strings.Contains(md.Markdown, "````\n<img src=x onerror=alert(1)>") {
		t.Errorf("markdown = %q", md.Markdown)
	}

	// No line of the agent is a heading, a link or markup outside a fence.
	fence := ""

	for line := range strings.SplitSeq(md.Markdown, "\n") {
		ticks := len(line) - len(strings.TrimLeft(line, "`"))

		switch {
		case fence == "" && ticks >= 3 && strings.Trim(line, "`") == "":
			fence = line
		case fence != "" && ticks >= len(fence) && strings.Trim(line, "`") == "":
			fence = ""
		case fence == "" && (strings.HasPrefix(line, "# not") || strings.Contains(line, "<img") ||
			strings.Contains(line, "javascript:")):
			t.Errorf("text of the agent is outside a fence: %q", line)
		}
	}
}

// SDD-017 section 10: the project check of a session.
func TestChatSessionsBelongToTheirProject(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	mine, other := w.newProject("mine"), w.newProject("other")
	w.profile(t, "echo", "echo")

	s := w.start(t, mine, "echo")

	// A key limited to the other project cannot see the session: it is as missing as a made-up id.
	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:key_0000000001", Scopes: service.AllScopes, ProjectID: other,
	})

	for name, call := range map[string]func() error{
		"get": func() error { _, err := w.svc.ChatSessionGet(limited, service.ChatRef{SessionID: s.ID}); return err },
		"transcript": func() error {
			_, err := w.svc.ChatTranscriptGet(limited, service.ChatTranscriptRequest{SessionID: s.ID})
			return err
		},
	} {
		if c := code(call()); c != "E_CHAT_NOT_FOUND" {
			t.Errorf("%s = %q, want E_CHAT_NOT_FOUND", name, c)
		}
	}

	if _, err := w.svc.ChatSessionGet(
		limited,
		service.ChatRef{SessionID: "cht_0000000999"},
	); code(
		err,
	) != "E_CHAT_NOT_FOUND" {
		t.Errorf("a made-up id = %v", err)
	}

	if _, err := w.svc.ChatSessionList(
		limited,
		service.ChatSessionListRequest{ProjectID: mine},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("listing another project = %v", err)
	}

	// In network mode a user sees the sessions of their own projects only.
	stranger := service.WithCaller(context.Background(), service.Caller{
		Actor:       "user:usr_0000000009",
		Scopes:      service.RoleScopes(service.RoleMember),
		OwnerUserID: "usr_0000000009",
		Browser:     true,
	})

	if _, err := w.svc.ChatSessionStart(
		stranger,
		service.ChatSessionStartRequest{ProjectID: mine, Profile: "echo"},
	); code(
		err,
	) == "" {
		t.Error(
			"a user started a session on a project that is not theirs",
		) // owned by nobody in local data: not visible
	}
}

func TestChatProfileRules(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	good := fakeAgentBinary(t)

	for name, req := range map[string]service.AgentProfileSetRequest{
		"upper case name":   {Name: "Claude", Command: good},
		"a name with a dot": {Name: "a.b", Command: good},
		"empty command":     {Name: "x", Command: ""},
		"relative path":     {Name: "x", Command: "./agent"},
		"a command line":    {Name: "x", Command: good + " --flag; rm -rf /"},
		"missing program":   {Name: "x", Command: filepath.Join(os.TempDir(), "stuarty-no-such-program")},
		"a directory":       {Name: "x", Command: os.TempDir()},
		"a newline":         {Name: "x", Command: good, Args: []string{"a\nb"}},
		"too many args":     {Name: "x", Command: good, Args: make([]string, chat.MaxArgs+1)},
	} {
		if c := code(callSet(w, req)); c != "E_VALIDATION" {
			t.Errorf("%s = %q, want E_VALIDATION", name, c)
		}
	}

	// A bare name is looked up once and the absolute path is what is stored. The go tool is on the
	// search path of every machine that runs this test.
	p, err := w.svc.AgentProfileSet(person(), service.AgentProfileSetRequest{Name: "bare", Command: "go"})
	if err != nil || !filepath.IsAbs(p.Command) {
		t.Fatalf("a bare name = %+v, %v", p, err)
	}

	list, err := w.svc.AgentProfileList(person(), service.AgentProfileListRequest{})
	if err != nil || len(list.Profiles) != 1 || list.Profiles[0].Command != p.Command || !list.CanWrite ||
		!list.Available {
		t.Fatalf("list = %+v, %v", list, err)
	}

	if len(list.Suggestions) == 0 {
		t.Error("no suggestions")
	}

	// The limit of profiles.
	for i := range chat.MaxProfiles - 1 {
		name := "p" + strings.Repeat("a", i+1)
		if _, err := w.svc.AgentProfileSet(
			person(),
			service.AgentProfileSetRequest{Name: name, Command: good},
		); err != nil {
			t.Fatalf("profile %s: %v", name, err)
		}
	}

	if c := code(callSet(w, service.AgentProfileSetRequest{Name: "onetoomany", Command: good})); c != "E_VALIDATION" {
		t.Errorf("the 17th profile = %q", c)
	}

	// Replacing one is not adding one.
	if _, err := w.svc.AgentProfileSet(
		person(),
		service.AgentProfileSetRequest{Name: "bare", Command: good, Args: []string{"x"}},
	); err != nil {
		t.Errorf("replacing a profile at the limit = %v", err)
	}

	if _, err := w.svc.AgentProfileRemove(person(), service.AgentProfileRemoveRequest{Name: "bare"}); err != nil {
		t.Fatal(err)
	}

	if _, err := w.svc.AgentProfileRemove(
		person(),
		service.AgentProfileRemoveRequest{Name: "bare"},
	); code(
		err,
	) != "E_AGENT_NOT_FOUND" {
		t.Errorf("removing it again = %v", err)
	}

	// The audit holds the full command of the profile that was written.
	audit, err := w.svc.AuditList(w.as(ui), service.AuditListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	var found bool

	for _, e := range audit.Entries {
		if e.Action == "agent.profile.set" && e.SubjectID == "bare" {
			found = true
		}
	}

	if !found {
		t.Error("writing a profile is not in the audit")
	}
}

func callSet(w *chatWorld, req service.AgentProfileSetRequest) error {
	_, err := w.svc.AgentProfileSet(person(), req)

	return err
}

func TestChatStartRefusals(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "echo", "echo")
	w.profile(t, "gone", "sign-in")

	for name, tc := range map[string]struct {
		req  service.ChatSessionStartRequest
		code string
	}{
		"an unknown profile":  {service.ChatSessionStartRequest{ProjectID: project, Profile: "nothing"}, "E_AGENT_NOT_FOUND"},
		"a bad profile name":  {service.ChatSessionStartRequest{ProjectID: project, Profile: "No Way"}, "E_VALIDATION"},
		"an unknown project":  {service.ChatSessionStartRequest{ProjectID: "prj_0000000999", Profile: "echo"}, "E_NOT_FOUND"},
		"an agent that needs": {service.ChatSessionStartRequest{ProjectID: project, Profile: "gone"}, "E_AGENT_UNAVAILABLE"},
	} {
		if _, err := w.svc.ChatSessionStart(person(), tc.req); code(err) != tc.code {
			t.Errorf("%s = %v, want %s", name, err, tc.code)
		}
	}

	se := serviceError(t, func() error {
		_, err := w.svc.ChatSessionStart(person(), service.ChatSessionStartRequest{ProjectID: project, Profile: "gone"})
		return err
	}())
	if se.Details["reason"] != agenthost.ReasonSignIn {
		t.Errorf("details = %v", se.Details)
	}

	// A project in the trash starts nothing.
	if _, err := w.svc.ProjectTrash(w.as(ui), service.ProjectTrashRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	if _, err := w.svc.ChatSessionStart(
		person(),
		service.ChatSessionStartRequest{ProjectID: project, Profile: "echo"},
	); code(
		err,
	) != "E_INVALID_STATE" {
		t.Errorf("a trashed project = %v", err)
	}
}

func TestChatMessageRules(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")
	w.profile(t, "echo", "echo")
	s := w.start(t, project, "echo")

	for name, text := range map[string]string{
		"empty":      "",
		"blank":      " \n\t ",
		"too long":   strings.Repeat("x", chat.MaxMessageLength+1),
		"only noise": "\x00\x07",
	} {
		if _, err := w.svc.ChatSend(
			person(),
			service.ChatSendRequest{SessionID: s.ID, Text: text},
		); code(
			err,
		) != "E_VALIDATION" {
			t.Errorf("%s = %v", name, err)
		}
	}

	if _, err := w.svc.ChatSend(
		person(),
		service.ChatSendRequest{SessionID: "nope", Text: "x"},
	); code(
		err,
	) != "E_CHAT_NOT_FOUND" {
		t.Errorf("a malformed id = %v", err)
	}
}

func TestChatWithoutAHost(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	project := e.newProject("Launch video")

	list, err := e.svc.AgentProfileList(person(), service.AgentProfileListRequest{})
	if err != nil || list.Available {
		t.Fatalf("AgentProfileList = %+v, %v", list, err)
	}

	if _, err := e.svc.AgentProfileSet(
		person(),
		service.AgentProfileSetRequest{Name: "x", Command: os.Args[0]},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ChatSessionStart(
		person(),
		service.ChatSessionStartRequest{ProjectID: project, Profile: "x"},
	); code(
		err,
	) != "E_AGENT_UNAVAILABLE" {
		t.Errorf("ChatSessionStart without a host = %v", err)
	}

	// The reads work.
	if res, err := e.svc.ChatSessionList(
		e.as(ui),
		service.ChatSessionListRequest{ProjectID: project},
	); err != nil ||
		len(res.Sessions) != 0 {
		t.Errorf("ChatSessionList = %+v, %v", res, err)
	}
}

// In network mode a caller who is not an administrator sees the names of the profiles and nothing about the
// machine of the server (SDD-007 T-29).
func TestChatProfileListHidesThePathsFromAMember(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	w.profile(t, "echo", "echo")

	member := service.WithCaller(context.Background(), service.Caller{
		Actor:       "user:usr_0000000001",
		Scopes:      service.RoleScopes(service.RoleMember),
		OwnerUserID: "usr_0000000001",
		Browser:     true,
	})

	list, err := w.svc.AgentProfileList(member, service.AgentProfileListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Profiles) != 1 || list.Profiles[0].Name != "echo" || list.Profiles[0].Command != "" ||
		len(list.Profiles[0].Args) != 0 ||
		list.CanWrite {
		t.Errorf("a member sees %+v", list)
	}

	for _, s := range list.Suggestions {
		if s.FoundPath != "" {
			t.Errorf("a member sees the path of %s", s.Name)
		}
	}

	// The same member cannot write a profile: the command runs on the server.
	if _, err := w.svc.AgentProfileSet(
		member,
		service.AgentProfileSetRequest{Name: "x", Command: fakeAgentBinary(t)},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("a member writing a profile = %v", err)
	}
}

func TestChatPruneKeepsTheNewestEndedSessionsAndEveryLiveOne(t *testing.T) {
	t.Parallel()

	w := newChatWorld(t)
	project := w.newProject("Launch video")
	repo := w.store.Chat()
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

	for i, state := range []chat.State{chat.StateEnded, chat.StateEnded, chat.StateEnded, chat.StateIdle} {
		err := repo.SessionCreate(context.Background(), agenthost.Session{
			ID: fmt.Sprintf("cht_%010d", i+1), ProjectID: project, Profile: "p", StartedBy: "local:ui", State: state,
			CreatedAt: base.Add(time.Duration(i) * time.Hour), UpdatedAt: base.Add(time.Duration(i) * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := repo.SessionsPrune(context.Background(), project, 1); err != nil {
		t.Fatal(err)
	}

	left, err := repo.SessionList(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}

	var ids []string
	for _, s := range left {
		ids = append(ids, s.ID)
	}

	// The newest ended one and the live one stay; the two older ended ones go.
	if len(ids) != 2 || !slices.Contains(ids, "cht_0000000003") || !slices.Contains(ids, "cht_0000000004") {
		t.Errorf("sessions left = %v", ids)
	}
}
