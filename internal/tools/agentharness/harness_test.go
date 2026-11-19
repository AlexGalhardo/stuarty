package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// endTurn is the response of a model that has nothing more to do.
const endTurn = `{"content":[{"type":"text","text":"done"}],"stop_reason":"end_turn",` +
	`"usage":{"input_tokens":1,"output_tokens":1}}`

const testKey = "test-" + "credential"

// fakeAPI answers the Messages API with a script of responses, one per call, and records the
// requests. After the script it answers with an end_turn.
type fakeAPI struct {
	t        *testing.T
	script   []string
	calls    atomic.Int32
	requests [][]byte
	headers  []http.Header
	status   func(call int) int
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := int(f.calls.Add(1)) - 1

	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(r.Body)
	f.requests = append(f.requests, body.Bytes())
	f.headers = append(f.headers, r.Header.Clone())

	if f.status != nil {
		if code := f.status(n); code != http.StatusOK {
			http.Error(w, "overloaded", code)

			return
		}
	}

	w.Header().Set("content-type", "application/json")

	if n < len(f.script) {
		_, _ = w.Write([]byte(f.script[n]))

		return
	}

	_, _ = w.Write([]byte(endTurn))
}

func toolUse(t *testing.T, id, args, stdin string) string {
	t.Helper()

	in, err := json.Marshal(map[string]string{"args": args, "stdin": stdin})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(response{
		Content:    []block{{Type: "tool_use", ID: id, Name: toolName, Input: in}},
		StopReason: "tool_use",
		Usage:      usage{InputTokens: 100, OutputTokens: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

func newAgent(url string, doer Doer, turns int) *agent {
	return &agent{
		api:   &client{http: doer, url: url, key: testKey, sleep: func(time.Duration) {}, backoff: time.Millisecond},
		model: defaultModel, maxTokens: 100, maxTurns: turns, system: "system",
	}
}

// scriptedRunner records the commands and answers from a table keyed by the command name.
func scriptedRunner(failing ...string) (Runner, *[][]string) {
	var seen [][]string

	return func(_ context.Context, args []string, _ string) (string, int, error) {
		seen = append(seen, args)

		if slices.Contains(failing, args[0]) {
			return "E_FAILED: nope", 1, nil
		}

		return "ok " + args[0], 0, nil
	}, &seen
}

func TestSplitArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{in: "project:list --json", want: []string{"project:list", "--json"}},
		{
			in:   `project:create --name "Launch day" --id prj_1`,
			want: []string{"project:create", "--name", "Launch day", "--id", "prj_1"},
		},
		{in: `edit --note 'it said "hi"'`, want: []string{"edit", "--note", `it said "hi"`}},
		{in: `a\ b c`, want: []string{"a b", "c"}},
		{in: `x "" y`, want: []string{"x", "", "y"}},
		{in: "$(rm -rf) `id` ; x", want: []string{"$(rm", "-rf)", "`id`", ";", "x"}},
		{in: `edit "open`, wantErr: true},
		{in: `trailing\`, wantErr: true},
		{in: "   ", want: nil},
	}

	for _, tt := range tests {
		got, err := splitArgs(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("%q: err = %v", tt.in, err)

			continue
		}

		if !tt.wantErr && !slices.Equal(got, tt.want) {
			t.Errorf("%q: got %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAgentCompletesAScenarioAndTheClientSendsWhatTheAPIExpects(t *testing.T) {
	t.Parallel()

	fake := &fakeAPI{t: t, script: []string{
		toolUse(t, "t1", `project:create --name "Base" --id prj_1`, ""),
		toolUse(t, "t2", "edit --project prj_1 --ops -", `[{"op":"track.add"}]`),
		toolUse(t, "t3", "template:save --project prj_1 --name x", ""),
		toolUse(t, "t4", "template:new tpl_1 --id prj_2 --fill -", "{}"),
		toolUse(t, "t5", "export --project prj_2", ""),
	}}
	srv := httptest.NewServer(fake)

	t.Cleanup(srv.Close)

	run, seen := scriptedRunner()

	sc, _ := findScenarios("template")

	res, err := newAgent(srv.URL, srv.Client(), 20).play(t.Context(), sc[0], 1, run)
	if err != nil {
		t.Fatal(err)
	}

	if !res.Success || res.Reason != "" {
		t.Fatalf("not a success: %+v", res.Reason)
	}

	if len(*seen) != 5 || (*seen)[3][0] != "template:new" {
		t.Errorf("commands run: %q", *seen)
	}

	if res.InputTokens != 501 || res.Turns != 6 {
		t.Errorf("tokens %d turns %d", res.InputTokens, res.Turns)
	}

	h := fake.headers[0]
	if h.Get("x-api-key") != testKey || h.Get("anthropic-version") != anthropicVersion {
		t.Errorf("headers: %v", h)
	}

	var first request
	if err := json.Unmarshal(fake.requests[0], &first); err != nil {
		t.Fatal(err)
	}

	if first.Model != defaultModel || first.System != "system" || len(first.Tools) != 1 ||
		first.Tools[0].Name != toolName {
		t.Errorf("first request: %+v", first)
	}

	// The tool result of turn 1 comes back to the model on turn 2, linked to its call.
	var second request
	if err := json.Unmarshal(fake.requests[1], &second); err != nil {
		t.Fatal(err)
	}

	last := second.Messages[len(second.Messages)-1].Content[0]
	if last.Type != "tool_result" || last.ToolUseID != "t1" || !strings.Contains(last.Content, "exit code 0") {
		t.Errorf("tool result: %+v", last)
	}

	for _, req := range fake.requests {
		if strings.Contains(string(req), testKey) {
			t.Error("the key is in a request body")
		}
	}
}

func TestAgentFailsWhenARequiredCommandNeverSucceeds(t *testing.T) {
	t.Parallel()

	fake := &fakeAPI{t: t, script: []string{
		toolUse(t, "t1", "project:create --name x", ""),
		toolUse(t, "t2", "export --project x", ""),
	}}
	srv := httptest.NewServer(fake)

	t.Cleanup(srv.Close)

	run, _ := scriptedRunner("export")

	sc, _ := findScenarios("template")

	res, err := newAgent(srv.URL, srv.Client(), 20).play(t.Context(), sc[0], 1, run)
	if err != nil {
		t.Fatal(err)
	}

	if res.Success || !strings.Contains(res.Reason, "export") || !strings.Contains(res.Reason, "template:save") {
		t.Errorf("success=%t reason=%q", res.Success, res.Reason)
	}

	if res.Commands[1].ExitCode != 1 {
		t.Error("the failed command was not recorded as failed")
	}
}

func TestAgentStopsAtTheTurnLimit(t *testing.T) {
	t.Parallel()

	loop := toolUse(t, "t", "project:list", "")
	fake := &fakeAPI{t: t, script: []string{loop, loop, loop, loop, loop}}
	srv := httptest.NewServer(fake)

	t.Cleanup(srv.Close)

	run, seen := scriptedRunner()

	sc, _ := findScenarios("template")

	res, err := newAgent(srv.URL, srv.Client(), 3).play(t.Context(), sc[0], 1, run)
	if err != nil {
		t.Fatal(err)
	}

	if res.Success || res.Turns != 3 || len(*seen) != 3 || !strings.Contains(res.Reason, "turn limit") {
		t.Errorf("turns=%d commands=%d reason=%q", res.Turns, len(*seen), res.Reason)
	}

	if int(fake.calls.Load()) != 3 {
		t.Errorf("%d API calls for 3 turns", fake.calls.Load())
	}
}

func TestUnknownToolAndBadInputAreReportedToTheModel(t *testing.T) {
	t.Parallel()

	bad := `{"content":[{"type":"tool_use","id":"u1","name":"rm","input":{}},` +
		`{"type":"tool_use","id":"u2","name":"` + toolName + `","input":{"args":"edit \"open"}}],` +
		`"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`
	fake := &fakeAPI{t: t, script: []string{bad}}
	srv := httptest.NewServer(fake)

	t.Cleanup(srv.Close)

	run, seen := scriptedRunner()

	sc, _ := findScenarios("template")
	if _, err := newAgent(srv.URL, srv.Client(), 5).play(t.Context(), sc[0], 1, run); err != nil {
		t.Fatal(err)
	}

	if len(*seen) != 0 {
		t.Errorf("ran %q", *seen)
	}

	var second request
	if err := json.Unmarshal(fake.requests[1], &second); err != nil {
		t.Fatal(err)
	}

	results := second.Messages[len(second.Messages)-1].Content
	if len(results) != 2 || !results[0].IsError || !results[1].IsError {
		t.Errorf("results: %+v", results)
	}
}

func TestRetriesTransientErrorsOnly(t *testing.T) {
	t.Parallel()

	fake := &fakeAPI{t: t, status: func(call int) int {
		if call < 2 {
			return http.StatusTooManyRequests
		}

		return http.StatusOK
	}}
	srv := httptest.NewServer(fake)

	t.Cleanup(srv.Close)

	c := newAgent(srv.URL, srv.Client(), 1).api
	if _, err := c.send(t.Context(), request{}); err != nil {
		t.Fatalf("a retried call failed: %v", err)
	}

	if fake.calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", fake.calls.Load())
	}

	fake = &fakeAPI{t: t, status: func(int) int { return http.StatusUnauthorized }}
	srv2 := httptest.NewServer(fake)

	t.Cleanup(srv2.Close)

	c = newAgent(srv2.URL, srv2.Client(), 1).api
	if _, err := c.send(t.Context(), request{}); err == nil || fake.calls.Load() != 1 {
		t.Errorf("a 401 was retried or accepted: calls=%d err=%v", fake.calls.Load(), err)
	}
}

func TestAnAPIFailureEndsTheRunAsAFailure(t *testing.T) {
	t.Parallel()

	fake := &fakeAPI{t: t, status: func(int) int { return http.StatusBadRequest }}
	srv := httptest.NewServer(fake)

	t.Cleanup(srv.Close)

	run, _ := scriptedRunner()

	sc, _ := findScenarios("template")

	res, err := newAgent(srv.URL, srv.Client(), 5).play(t.Context(), sc[0], 1, run)
	if err != nil || res.Success || !strings.Contains(res.Reason, "400") {
		t.Errorf("err=%v success=%t reason=%q", err, res.Success, res.Reason)
	}
}

func TestRefusesToStartWithoutTheGuards(t *testing.T) {
	t.Parallel()

	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	full := map[string]string{"STUARTY_HARNESS": "1", "ANTHROPIC_API_KEY": testKey}

	tests := []struct {
		name string
		argv []string
		env  map[string]string
		want string
	}{
		{
			"no switch",
			[]string{"--runs", "1"},
			map[string]string{"ANTHROPIC_API_KEY": testKey},
			"STUARTY_HARNESS=1",
		},
		{
			"switch is not 1",
			[]string{"--runs", "1"},
			map[string]string{"STUARTY_HARNESS": "true", "ANTHROPIC_API_KEY": testKey},
			"STUARTY_HARNESS=1",
		},
		{"no runs", nil, full, "--runs"},
		{"zero runs", []string{"--runs", "0"}, full, "--runs"},
		{"no key", []string{"--runs", "1"}, map[string]string{"STUARTY_HARNESS": "1"}, "ANTHROPIC_API_KEY"},
		{"http endpoint", []string{"--runs", "1", "--endpoint", "http://example.com/v1/messages"}, full, "https"},
		{"bad scenario", []string{"--runs", "1", "--scenario", "x"}, full, "unknown scenario"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			var calls atomic.Int32

			api := doerFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)

				return nil, http.ErrNotSupported
			})

			code := run(t.Context(), tt.argv, env(tt.env), api, nil, &stdout, &stderr)
			if code != 2 || !strings.Contains(stderr.String(), tt.want) || calls.Load() != 0 {
				t.Errorf("code=%d stderr=%q calls=%d", code, stderr.String(), calls.Load())
			}
		})
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestEstimateOnlyPrintsTheCostAndCallsNothing(t *testing.T) {
	t.Parallel()

	skill := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(skill, []byte(strings.Repeat("word ", 2000)), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	api := doerFunc(func(*http.Request) (*http.Response, error) {
		t.Error("the API was called")

		return nil, http.ErrNotSupported
	})
	env := func(k string) string { return map[string]string{"STUARTY_HARNESS": "1"}[k] }

	code := run(t.Context(), []string{"--runs", "10", "--estimate", "--skill", skill}, env, api, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code %d: %s", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "10 runs of 2 scenario(s) on claude-sonnet-5-5") ||
		!strings.Contains(stdout.String(), "estimated cost up to USD") {
		t.Errorf("output: %s", stdout.String())
	}
}

func TestEstimateGrowsWithRunsAndTurns(t *testing.T) {
	t.Parallel()

	base := options{runs: 1, maxTurns: 10, maxTokens: 1000, priceIn: 3, priceOut: 15, scenarioList: scenarios}
	one := estimate(base, 40000)

	base.runs = 10
	ten := estimate(base, 40000)

	if ten.input != one.input*10 || ten.usd <= one.usd*9.99 || one.usd <= 0 {
		t.Errorf("one=%+v ten=%+v", one, ten)
	}

	base.maxTurns = 20
	if estimate(base, 40000).input <= ten.input {
		t.Error("more turns did not cost more")
	}
}

func TestFullRunWritesTranscriptsAndSummaryAndJudgesTheBar(t *testing.T) {
	t.Parallel()

	// The server serves one script across the runs: the first run completes the template recipe,
	// and the second finds the script used up, so it stops at once and fails.
	var call atomic.Int32

	commands := []string{
		"project:create --name b", "edit --project p --ops -", "template:save --project p",
		"template:new t", "export --project q",
	}
	steps := make([]string, len(commands))
	for i, c := range commands {
		steps[i] = toolUse(t, "t", c, "")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(call.Add(1)) - 1
		if n < len(steps) {
			_, _ = w.Write([]byte(steps[n]))

			return
		}

		_, _ = w.Write([]byte(endTurn))
	}))

	t.Cleanup(srv.Close)

	dir := t.TempDir()
	skill := filepath.Join(dir, "SKILL.md")

	if err := os.WriteFile(skill, []byte("skill"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "out")
	env := func(k string) string {
		return map[string]string{"STUARTY_HARNESS": "1", "ANTHROPIC_API_KEY": testKey}[k]
	}
	mk := func(string) func(home, work string) Runner {
		return func(_, _ string) Runner {
			r, _ := scriptedRunner()

			return r
		}
	}

	var stdout, stderr bytes.Buffer

	argv := []string{"--runs", "2", "--scenario", "template", "--endpoint", srv.URL, "--skill", skill, "--out", out}

	code := run(t.Context(), argv, env, srv.Client(), mk, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("1 of 2 runs passed but the exit code is %d (bar is 2): %s %s", code, stdout.String(), stderr.String())
	}

	for _, f := range []string{"template-01.json", "template-02.json", "summary.json"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}

	raw, err := os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(raw), `"success": 1`) || strings.Contains(string(raw), testKey) {
		t.Errorf("summary: %s", raw)
	}

	if !strings.Contains(stdout.String(), "1 of 2 runs succeeded (the bar is 2)") {
		t.Errorf("stdout: %s", stdout.String())
	}
}

func TestPrototypeScenarioNeedsAClip(t *testing.T) {
	t.Parallel()

	skill := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(skill, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}

	env := func(k string) string {
		return map[string]string{"STUARTY_HARNESS": "1", "ANTHROPIC_API_KEY": testKey}[k]
	}

	var stdout, stderr bytes.Buffer

	api := doerFunc(func(*http.Request) (*http.Response, error) {
		t.Error("the API was called")

		return nil, http.ErrNotSupported
	})

	code := run(
		t.Context(),
		[]string{"--runs", "1", "--scenario", "prototype", "--skill", skill},
		env,
		api,
		nil,
		&stdout,
		&stderr,
	)
	if code != 2 || !strings.Contains(stderr.String(), "--clip") {
		t.Errorf("code=%d stderr=%s", code, stderr.String())
	}
}

func TestJudgeIgnoresGlobalFlagsAndFailedCommands(t *testing.T) {
	t.Parallel()

	sc := scenario{Required: []string{"export"}}

	if ok, _ := judge(sc, []commandRecord{{Args: "--json export --project p", ExitCode: 0}}); !ok {
		t.Error("a global flag before the command hid it")
	}

	if ok, _ := judge(sc, []commandRecord{{Args: "export --project p", ExitCode: 1}}); ok {
		t.Error("a failed export counted")
	}
}

func TestScenariosAreWellFormed(t *testing.T) {
	t.Parallel()

	for _, sc := range scenarios {
		if sc.Name == "" || sc.Prompt == "" || len(sc.Required) == 0 {
			t.Errorf("incomplete scenario %+v", sc)
		}
	}

	if _, ok := findScenarios("all"); !ok {
		t.Error("all")
	}
}

func TestExecRunnerReportsExitCodesAndMissingBinaries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	out, code, err := execRunner("go")(dir, dir)(t.Context(), []string{"version"}, "")
	if err != nil || code != 0 || !strings.Contains(out, "go version") {
		t.Errorf("go version: %q %d %v", out, code, err)
	}

	if _, code, err = execRunner(
		"go",
	)(
		dir,
		dir,
	)(
		t.Context(),
		[]string{"no-such-subcommand"},
		"",
	); err != nil ||
		code == 0 {
		t.Errorf("a failing command: code=%d err=%v", code, err)
	}

	if _, _, err = execRunner("definitely-not-a-binary-xyz")(dir, dir)(t.Context(), []string{"x"}, ""); err == nil {
		t.Error("a missing binary was not an error")
	}
}
