package chat

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNext(t *testing.T) {
	t.Parallel()

	cases := []struct {
		from State
		in   Input
		want State
		err  error
	}{
		{StateStarting, InputStarted, StateIdle, nil},
		{StateStarting, InputEnd, StateEnded, nil},
		{StateStarting, InputSend, StateStarting, ErrInvalidTransition},
		{StateIdle, InputSend, StateRunning, nil},
		{StateIdle, InputEnd, StateEnded, nil},
		{StateIdle, InputCancel, StateIdle, nil},
		{StateIdle, InputTurnEnded, StateIdle, ErrInvalidTransition},
		{StateIdle, InputPermissionAnswered, StateIdle, ErrInvalidTransition},
		{StateRunning, InputSend, StateRunning, ErrBusy},
		{StateRunning, InputPermissionAsked, StateAwaitingPermission, nil},
		{StateRunning, InputTurnEnded, StateIdle, nil},
		{StateRunning, InputCancel, StateRunning, nil},
		{StateRunning, InputEnd, StateEnded, nil},
		{StateRunning, InputStarted, StateRunning, ErrInvalidTransition},
		{StateAwaitingPermission, InputPermissionAnswered, StateRunning, nil},
		{StateAwaitingPermission, InputSend, StateAwaitingPermission, ErrBusy},
		{StateAwaitingPermission, InputTurnEnded, StateIdle, nil},
		{StateAwaitingPermission, InputCancel, StateAwaitingPermission, nil},
		{StateAwaitingPermission, InputEnd, StateEnded, nil},
		{StateEnded, InputSend, StateEnded, ErrEnded},
		{StateEnded, InputEnd, StateEnded, ErrEnded},
		{StateEnded, InputCancel, StateEnded, ErrEnded},
		{State("nonsense"), InputEnd, State("nonsense"), ErrInvalidTransition},
	}

	for _, c := range cases {
		got, err := Next(c.from, c.in)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("Next(%s, %s) = %s, %v; want %s, %v", c.from, c.in, got, err, c.want, c.err)
		}
	}
}

func TestStateAndReasonValidity(t *testing.T) {
	t.Parallel()

	if !StateIdle.Live() || StateEnded.Live() || State("x").Live() {
		t.Error("Live is wrong")
	}

	for _, r := range []EndReason{
		"", EndUser, EndExited, EndProtocol, EndTimeout, EndIdle, EndFlood, EndShutdown, EndStartFailed,
	} {
		if !r.Valid() {
			t.Errorf("reason %q should be valid", r)
		}
	}

	if EndReason("because").Valid() {
		t.Error("an unknown reason is valid")
	}
}

func TestCheckProfile(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", MaxArgLength+1)
	manyArgs := make([]string, MaxArgs+1)

	cases := []struct {
		name    string
		profile string
		command string
		args    []string
		want    error
	}{
		{"bare name", "claude", "claude-agent-acp", nil, nil},
		{"unix path", "p", "/usr/local/bin/opencode", []string{"acp"}, nil},
		{"windows path", "p", `C:\Tools\agent.exe`, nil, nil},
		{"unc path", "p", `\\server\share\agent.exe`, nil, nil},
		{"upper case name", "Claude", "x", nil, ErrProfileName},
		{"empty name", "", "x", nil, ErrProfileName},
		{"name with a space", "a b", "x", nil, ErrProfileName},
		{"name too long", strings.Repeat("a", 33), "x", nil, ErrProfileName},
		{"name starting with a dash", "-a", "x", nil, ErrProfileName},
		{"empty command", "p", "", nil, ErrCommand},
		{"padded command", "p", " agent", nil, ErrCommand},
		{"relative path", "p", "./agent", nil, ErrCommand},
		{"relative dir path", "p", `bin\agent`, nil, ErrCommand},
		{"parent path", "p", "../agent", nil, ErrCommand},
		{"newline in command", "p", "agent\nrm", nil, ErrCommand},
		{"nul in command", "p", "agent\x00", nil, ErrCommand},
		{"command too long", "p", strings.Repeat("a", MaxCommandLength+1), nil, ErrCommand},
		{"invalid utf8", "p", "ag\xffent", nil, ErrCommand},
		{"argument too long", "p", "agent", []string{long}, ErrArgs},
		{"too many arguments", "p", "agent", manyArgs, ErrArgs},
		{"control in argument", "p", "agent", []string{"a\x1bb"}, ErrArgs},
		{"newline in argument", "p", "agent", []string{"a\nb"}, ErrArgs},
		{"a shell line is one odd name, not a command line", "p", "agent --flag; rm -rf /", nil, ErrCommand},
	}

	for _, c := range cases {
		err := CheckProfile(c.profile, c.command, c.args)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: CheckProfile = %v, want %v", c.name, err, c.want)
		}
	}
}

func TestSuggestionsAreText(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}

	for _, s := range Suggestions() {
		if !ValidProfileName(s.Name) || seen[s.Name] || s.Title == "" || len(s.Executables) == 0 || s.Install == "" {
			t.Errorf("bad suggestion %+v", s)
		}

		seen[s.Name] = true

		if err := CheckProfile(s.Name, s.Executables[0], s.Args); err != nil {
			t.Errorf("suggestion %s does not pass the profile rules: %v", s.Name, err)
		}
	}
}

func TestPickOption(t *testing.T) {
	t.Parallel()

	full := []Option{
		{
			"aa",
			"Always",
			AllowAlways,
		},
		{"ao", "Once", AllowOnce},
		{"ra", "Never", RejectAlways},
		{"ro", "No", RejectOnce},
	}
	onlyAlways := []Option{{"aa", "Always", AllowAlways}, {"ra", "Never", RejectAlways}}

	cases := []struct {
		name      string
		opts      []Option
		answer    Answer
		id        string
		cancelled bool
		err       error
	}{
		{"allow picks the one-time option", full, AnswerAllow, "ao", false, nil},
		{"reject picks the one-time option", full, AnswerReject, "ro", false, nil},
		{"cancel", full, AnswerCancel, "", true, nil},
		{"allow never picks a standing option", onlyAlways, AnswerAllow, "", false, ErrNoOneTimeOption},
		{"reject with only a standing option is a cancel", onlyAlways, AnswerReject, "", true, nil},
		{"no options: allow is refused", nil, AnswerAllow, "", false, ErrNoOneTimeOption},
		{"no options: reject is a cancel", nil, AnswerReject, "", true, nil},
		{"unknown answer", full, Answer("always"), "", false, ErrAnswer},
		{"empty answer", full, Answer(""), "", false, ErrAnswer},
	}

	for _, c := range cases {
		id, cancelled, err := PickOption(c.opts, c.answer)
		if id != c.id || cancelled != c.cancelled || !errors.Is(err, c.err) {
			t.Errorf("%s: got %q %v %v, want %q %v %v", c.name, id, cancelled, err, c.id, c.cancelled, c.err)
		}
	}

	if AnswerAllow.Valid() != true || Answer("allow_always").Valid() {
		t.Error("Valid is wrong")
	}
}

func TestCleanOptions(t *testing.T) {
	t.Parallel()

	in := []Option{
		{"a", "ok", AllowOnce},
		{"b", "bad kind", OptionKind("allow_forever")},
		{"", "no id", RejectOnce},
		{"c", "x\x00y\u202Ez", RejectAlways},
	}

	for range MaxOptions {
		in = append(in, Option{"z", "more", AllowOnce})
	}

	got := CleanOptions(in)
	if len(got) != MaxOptions {
		t.Fatalf("len = %d, want %d", len(got), MaxOptions)
	}

	if got[0].ID != "a" || got[1].ID != "c" || got[1].Name != "xyz" {
		t.Errorf("got %+v", got[:2])
	}
}

func TestCleanText(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, in, want string }{
		{"plain", "hello", "hello"},
		{"markup stays as text", "<img src=x onerror=alert(1)>", "<img src=x onerror=alert(1)>"},
		{"nul and bell", "a\x00b\x07c", "abc"},
		{"carriage return", "a\r\nb\rc", "a\n\nb\nc"},
		{"tab and newline kept", "a\tb\nc", "a\tb\nc"},
		{"bidi override", "abc\u202Edef", "abcdef"},
		{"isolates", "a\u2066b\u2069c", "abc"},
		{"zero width", "a\u200Bb\u200Dc\uFEFFd", "abcd"},
		{"tag characters", "a\U000E0041b", "ab"},
		{"invalid utf8", "a\xffb", "a\uFFFDb"},
		{"empty", "", ""},
	}

	for _, c := range cases {
		if got := CleanText(c.in); got != c.want {
			t.Errorf("%s: CleanText(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestClip(t *testing.T) {
	t.Parallel()

	if got, cut := Clip("short", 10); got != "short" || cut {
		t.Errorf("Clip = %q, %v", got, cut)
	}

	got, cut := Clip(strings.Repeat("a", 100), 20)
	if !cut || len(got) != 20 || !strings.HasSuffix(got, ClipMarker) {
		t.Errorf("Clip = %q (%d), %v", got, len(got), cut)
	}

	// A cut never splits a character.
	got, _ = Clip(strings.Repeat("é", 50), 21)
	if !strings.HasSuffix(got, ClipMarker) || strings.ContainsRune(got, '\uFFFD') || len(got) > 21 {
		t.Errorf("Clip of a multi-byte text = %q (%d)", got, len(got))
	}

	if got, cut := Clip("abcdef", 3); !cut || len(got) > len(ClipMarker) {
		t.Errorf("Clip below the marker = %q, %v", got, cut)
	}
}

func TestTitle(t *testing.T) {
	t.Parallel()

	if got := Title("  make   the\n intro\tshorter \x00"); got != "make the intro shorter" {
		t.Errorf("Title = %q", got)
	}

	if got := Title(strings.Repeat("x", 500)); len(got) > MaxTitleLength {
		t.Errorf("Title is %d bytes", len(got))
	}
}

func TestKindValid(t *testing.T) {
	t.Parallel()

	for _, k := range []Kind{
		KindUserMessage, KindAgentMessage, KindAgentThought, KindToolCall, KindPermissionAsked,
		KindPermissionAnswer, KindPlan, KindNotice, KindState,
	} {
		if !k.Valid() {
			t.Errorf("%s should be valid", k)
		}
	}

	if Kind("html").Valid() {
		t.Error("an unknown kind is valid")
	}
}

func TestRenderMarkdownKeepsEveryTextInsideItsFence(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	end := at.Add(time.Minute)
	hostile := "# Heading\n```\n[click](javascript:alert(1)) <script>x</script>\n````\n## Fake section"

	md := RenderMarkdown(Transcript{
		SessionID: "cht_0123456789", ProjectID: "prj_0123456789", Profile: "claude\n# x", Title: hostile,
		StartedAt: at, EndedAt: &end, EndReason: EndUser,
		Events: []Event{
			{Seq: 1, Kind: KindUserMessage, At: at, Text: hostile},
			{Seq: 2, Kind: KindAgentMessage, At: at, Text: hostile},
			{Seq: 3, Kind: KindAgentThought, At: at, Text: "hm"},
			{Seq: 4, Kind: KindToolCall, At: at, Tool: &Tool{Title: hostile, Status: ToolCompleted, Output: hostile}},
			{Seq: 5, Kind: KindPermissionAsked, At: at, Permission: &Permission{Title: hostile}},
			{Seq: 6, Kind: KindPermissionAnswer, At: at, Permission: &Permission{Title: "t", Answer: AnswerAllow}},
			{Seq: 7, Kind: KindPlan, At: at, Text: "step"},
			{Seq: 8, Kind: KindNotice, At: at, Text: hostile},
			{Seq: 9, Kind: KindState, At: at, State: &StateInfo{State: StateEnded, EndReason: EndUser}},
		},
	})

	if !strings.HasPrefix(md, "# Chat session cht_0123456789\n") {
		t.Fatalf("heading: %q", md[:40])
	}

	// Outside a fence the only headings are the ones the renderer writes.
	inFence, fenceLen := false, 0

	for line := range strings.SplitSeq(md, "\n") {
		trim := strings.TrimRight(line, " ")
		n := 0

		for n < len(trim) && trim[n] == '`' {
			n++
		}

		switch {
		case !inFence && n >= 3 && strings.Trim(trim, "`") == "":
			inFence, fenceLen = true, n
		case inFence && n >= fenceLen && strings.Trim(trim, "`") == "":
			inFence = false
		case !inFence && strings.HasPrefix(line, "#"):
			if !strings.HasPrefix(line, "# Chat session ") && !strings.HasPrefix(line, "## You, ") &&
				!strings.HasPrefix(line, "## Agent") && !strings.HasPrefix(line, "## Tool call, ") &&
				!strings.HasPrefix(line, "## Permission, ") && !strings.HasPrefix(line, "## Notice, ") {
				t.Errorf("a heading outside a fence that the renderer did not write: %q", line)
			}
		case !inFence && (strings.Contains(line, "<script") || strings.Contains(line, "javascript:")):
			t.Errorf("markup outside a fence: %q", line)
		}
	}

	if inFence {
		t.Error("a fence was left open")
	}

	if strings.Contains(md, "claude\n") {
		t.Error("the profile name was written unreduced")
	}
}

func TestFenceIsLongerThanAnyRun(t *testing.T) {
	t.Parallel()

	got := fence("a ````` b")
	if !strings.HasPrefix(got, "``````\n") {
		t.Errorf("fence = %q", got)
	}

	if got := fence("plain"); !strings.HasPrefix(got, "```\n") {
		t.Errorf("fence = %q", got)
	}
}
