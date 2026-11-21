package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
)

// oneJSONDocument fails unless s is exactly one JSON document and a newline.
func oneJSONDocument(tb testing.TB, s string) map[string]any {
	tb.Helper()

	if !strings.HasSuffix(s, "\n") || strings.Count(s, "\n") != 1 {
		tb.Fatalf("stdout is not one line: %q", s)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		tb.Fatalf("stdout is not one JSON document: %v\n%s", err, s)
	}

	return doc
}

func TestProjectCreateJSONIsOneDocumentAndStderrIsEmpty(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.res.inProcess = true

	if code := h.run(
		"project:create",
		"--name",
		"Launch teaser",
		"--description",
		ctxDescription,
		"--specific",
		ctxSpecific,
		"--measurable",
		ctxMeasurable,
		"--achievable",
		ctxAchievable,
		"--relevant",
		ctxRelevant,
		"--time-bound",
		ctxTimeBound,
		"--due",
		ctxDue,
		"--json",
	); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, h.stderr.String())
	}

	doc := oneJSONDocument(t, h.stdout.String())
	if doc["project_id"] != "prj_7k2m9x4q1b" || doc["name"] != "Launch teaser" {
		t.Fatalf("unexpected document: %v", doc)
	}

	if h.stderr.Len() != 0 {
		t.Fatalf("stderr must be empty in --json mode, got %q", h.stderr.String())
	}

	got := h.svc.createReq
	if got.Name != "Launch teaser" || got.Description != ctxDescription || got.Goal.Due != ctxDue ||
		got.Goal.TimeBound != ctxTimeBound || got.Preset != "" || got.FPS != "" {
		t.Fatalf("the context flags of SDD-003 5.4 are not passed, or a deprecated default is: %+v", got)
	}
}

func TestHumanOutputIsTheTextAndTheInProcessNoteGoesToStderr(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.res.inProcess = true

	if code := h.run(
		"project:create",
		"--name",
		"Project x",
		"--description",
		ctxDescription,
		"--specific",
		ctxSpecific,
		"--measurable",
		ctxMeasurable,
		"--achievable",
		ctxAchievable,
		"--relevant",
		ctxRelevant,
		"--time-bound",
		ctxTimeBound,
		"--due",
		ctxDue,
	); code != 0 {
		t.Fatalf("exit %d", code)
	}

	if h.stdout.String() != "created prj_7k2m9x4q1b\n" {
		t.Fatalf("stdout = %q", h.stdout.String())
	}

	if !strings.HasPrefix(h.stderr.String(), "note: no server owns /ws; this ran in process.") {
		t.Fatalf("missing in-process note: %q", h.stderr.String())
	}

	h.res.inProcess = false

	h.run(
		"project:create",
		"--name",
		"Project x",
		"--description",
		ctxDescription,
		"--specific",
		ctxSpecific,
		"--measurable",
		ctxMeasurable,
		"--achievable",
		ctxAchievable,
		"--relevant",
		ctxRelevant,
		"--time-bound",
		ctxTimeBound,
		"--due",
		ctxDue,
	)

	if h.stderr.Len() != 0 {
		t.Fatalf("no note in client mode, got %q", h.stderr.String())
	}

	h.res.inProcess = true

	h.run("project:list")

	if h.stderr.Len() != 0 {
		t.Fatalf("no note for a read command, got %q", h.stderr.String())
	}
}

func TestFlagsMayFollowPositionalArguments(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if code := h.run("project:show", "prj_7k2m9x4q1b", "--json", "--detail", "full"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	oneJSONDocument(t, h.stdout.String())

	if h.svc.getReq.ProjectID != "prj_7k2m9x4q1b" || h.svc.getReq.Detail != "full" {
		t.Fatalf("request = %+v", h.svc.getReq)
	}
}

// A flag value can be enormous. The validation error names the flag and the
// kind it expected, and it does not repeat the value (SDD-003 section 1.2 rule 9).
func TestInvalidFlagDoesNotEchoTheValue(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	huge := strings.Repeat("9", 8000)

	if code := h.run("project:list", "--limit", huge, "--json"); code != ExitFailure {
		t.Fatalf("exit %d: %s", code, h.stdout.String())
	}

	doc := oneJSONDocument(t, h.stdout.String())
	msg, _ := doc["message"].(string)

	if doc["code"] != command.CodeValidation || doc["path"] != "limit" || strings.Contains(msg, huge) {
		t.Fatalf("the error echoed the value or missed the code: %v", doc)
	}

	if !strings.Contains(msg, "integer") {
		t.Fatalf("the message does not name the expected kind: %q", msg)
	}

	hint, _ := doc["hint"].(string)
	if hint == "" {
		t.Fatalf("a bad flag needs a hint: %v", doc)
	}

	if h.res.resolved != 0 {
		t.Fatal("an invalid flag must not open the workspace")
	}
}

func TestProjectResolution(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if code := h.run("history", "--json"); code != ExitFailure {
		t.Fatalf("exit %d", code)
	}

	doc := oneJSONDocument(t, h.stdout.String())
	hint, _ := doc["hint"].(string)

	if doc["code"] != command.CodeValidation || doc["path"] != "project" || !strings.Contains(hint, EnvProject) {
		t.Fatalf("missing project must be E_VALIDATION with the hint: %v", doc)
	}

	if h.res.resolved != 0 {
		t.Fatal("the service must not be resolved for an invalid invocation")
	}

	h.rt.Getenv = func(k string) string {
		if k == EnvProject {
			return "prj_fromenv"
		}

		return ""
	}

	h.run("history")

	if h.svc.historyReq.ProjectID != "prj_fromenv" {
		t.Fatalf("environment project not used: %+v", h.svc.historyReq)
	}

	h.run("history", "--project", "prj_flag")

	if h.svc.historyReq.ProjectID != "prj_flag" {
		t.Fatalf("flag must win over the environment: %+v", h.svc.historyReq)
	}

	if code := h.run("project:show", "prj_a", "--project", "prj_b", "--json"); code != ExitFailure {
		t.Fatalf("a positional id that conflicts with --project must fail, exit %d", code)
	}
}

func TestActorValidation(t *testing.T) {
	t.Parallel()

	for actor, ok := range map[string]bool{
		"cli": true, "local:claude-code": true, "a-b-1": true,
		"ui": false, "user:u1": false, "key:k1": false, "UPPER": false, "": true,
		"abcdefghijklmnopqrstuvwxyz0123456789": false,
	} {
		h := newHarness(t)

		args := []string{"project:list", "--json"}
		if actor != "" {
			args = append(args, "--actor", actor)
		}

		code := h.run(args...)
		if ok != (code == 0) {
			t.Errorf("actor %q: exit %d, want ok=%t (%s)", actor, code, ok, h.stdout.String())
		}

		if ok && actor != "" {
			want := "local:" + strings.TrimPrefix(actor, "local:")
			if h.svc.caller.Actor != want {
				t.Errorf("actor %q: service saw %q, want %q", actor, h.svc.caller.Actor, want)
			}
		}
	}
}

func TestUnknownFlagAndCommandAreJSONErrorsWhenAsked(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if code := h.run("project:list", "--nope", "--json"); code != ExitFailure {
		t.Fatalf("exit %d", code)
	}

	if doc := oneJSONDocument(t, h.stdout.String()); doc["code"] != command.CodeValidation {
		t.Fatalf("doc = %v", doc)
	}

	if h.stderr.Len() != 0 {
		t.Fatalf("stderr = %q", h.stderr.String())
	}

	if code := h.run("nothing:here", "--json"); code != ExitFailure {
		t.Fatalf("exit %d", code)
	}

	if doc := oneJSONDocument(t, h.stdout.String()); doc["code"] != command.CodeUnknownCommand {
		t.Fatalf("doc = %v", doc)
	}
}

func TestHumanErrorGoesToStderr(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.svc.err = &service.Error{
		Code:      command.CodeRevisionConflict,
		Message:   "Revision 4 is behind revision 5.",
		Hint:      "send it again",
		Retryable: true,
	}

	if code := h.run("project:list"); code != ExitTransient {
		t.Fatalf("exit %d", code)
	}

	if h.stdout.Len() != 0 {
		t.Fatalf("stdout must be empty on a human error: %q", h.stdout.String())
	}

	want := "error E_REVISION_CONFLICT: Revision 4 is behind revision 5.\nhint: send it again\nretryable: true\n"
	if h.stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", h.stderr.String(), want)
	}
}

func TestLintStrict(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.svc.lintRes = &service.LintResult{
		ProjectID: "prj_a", Errors: 1, Findings: []lint.Finding{{Code: "L_MEDIA_MISSING", Severity: "error"}},
		Text: "lint r1 prj_a: errors=1 warnings=0",
	}

	if code := h.run("lint", "--project", "prj_a"); code != ExitOK {
		t.Fatalf("without --strict an error finding still exits 0, got %d", code)
	}

	if code := h.run("lint", "--project", "prj_a", "--strict", "--json"); code != ExitFailure {
		t.Fatalf("--strict with an error finding must exit 1, got %d", code)
	}

	oneJSONDocument(t, h.stdout.String())

	h.svc.lintRes = &service.LintResult{ProjectID: "prj_a", Warnings: 2, Text: "lint r1 prj_a: errors=0 warnings=2"}

	if code := h.run("lint", "--project", "prj_a", "--strict"); code != ExitOK {
		t.Fatalf("--strict with warnings only must exit 0, got %d", code)
	}

	if h.svc.lintReq.Limit != 25 || h.svc.lintReq.Severity != "warning" {
		t.Fatalf("lint defaults: %+v", h.svc.lintReq)
	}
}

func TestUndoText(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.svc.undoRes = &service.UndoResult{
		Revision: 5, BaseRevision: 4, Actor: "local:cli", Diff: make([]command.Change, 1),
		Description: "~ item itm_a: duration 2s -> 5s",
		Kind:        "undo",
		Undone:      &service.HistoryRef{Revision: 4},
		Skipped:     []service.SkippedOp{{Fields: []string{"items itm_a start"}, Reason: "changed_by_other_actor"}},
	}

	if code := h.run("undo", "--project", "prj_a", "--dry-run"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if !h.svc.undoReq.DryRun {
		t.Fatal("--dry-run not forwarded")
	}

	want := "undone r5 (base r4) by local:cli: changes=1, undid r4\n" +
		"~ item itm_a: duration 2s -> 5s\n" +
		"skipped items itm_a start: changed_by_other_actor\n" +
		"lint (fast rules): no new findings (errors=0 warnings=0 in total)\n"
	if h.stdout.String() != want {
		t.Fatalf("stdout:\n%s\nwant:\n%s", h.stdout.String(), want)
	}
}

func TestPendingCommandsNameTheirTask(t *testing.T) {
	t.Parallel()

	for _, s := range registry() {
		if s.pending == "" {
			continue
		}

		h := newHarness(t)

		if code := h.run(s.name, "--anything", "--json"); code != ExitFailure {
			t.Errorf("%s: exit %d", s.name, code)
		}

		doc := oneJSONDocument(t, h.stdout.String())

		msg, _ := doc["message"].(string)
		if !strings.Contains(msg, s.pending) {
			t.Errorf("%s: message %q does not name %s", s.name, msg, s.pending)
		}

		if h.res.resolved != 0 {
			t.Errorf("%s: a stub must not resolve a service", s.name)
		}

		if code := h.run(s.name, "--help"); code != ExitOK || !strings.Contains(h.stdout.String(), s.pending) {
			t.Errorf("%s --help: exit %d, %q", s.name, code, h.stdout.String())
		}
	}
}

func TestHelpIsAtMost40Lines(t *testing.T) {
	t.Parallel()

	for _, s := range registry() {
		h := newHarness(t)

		if code := h.run(s.name, "--help"); code != ExitOK {
			t.Fatalf("%s --help exit %d", s.name, code)
		}

		if n := strings.Count(h.stdout.String(), "\n"); n > 40 {
			t.Errorf("%s --help has %d lines", s.name, n)
		}
	}
}

func TestCommandHelp(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if code := h.run(
		"command:help",
	); code != ExitOK || h.stdout.String() != "help "+string(rune(10)) ||
		h.svc.helpReq.Name != "" {
		t.Fatalf("list: exit %d, %q, %+v", code, h.stdout.String(), h.svc.helpReq)
	}

	if code := h.run("command:help", "item.move", "--json"); code != ExitOK || h.svc.helpReq.Name != "item.move" {
		t.Fatalf("one command: exit %d, %+v", code, h.svc.helpReq)
	}

	oneJSONDocument(t, h.stdout.String())

	if code := h.run("command:help", "a", "b"); code != ExitFailure {
		t.Fatalf("two names must fail, exit %d", code)
	}
}

func TestVersionNeedsNoService(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.res.err = errors.New("the workspace must not be opened")

	if code := h.run("version"); code != ExitOK || !strings.HasPrefix(h.stdout.String(), "stuarty ") {
		t.Fatalf("exit %d, %q, %q", code, h.stdout.String(), h.stderr.String())
	}

	if code := h.run("version", "--json"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}

	doc := oneJSONDocument(t, h.stdout.String())
	if doc["version"] == "" || doc["text"] != "stuarty "+agentdocs.Version {
		t.Fatalf("doc = %v", doc)
	}

	if h.res.resolved != 0 {
		t.Fatal("version must not resolve a service")
	}

	if code := h.run("version", "extra"); code != ExitFailure {
		t.Fatalf("version takes no argument, exit %d", code)
	}
}

// VV-151: a not-found answer of a command that ran in process names the workspace it read.
func TestNotFoundInProcessNamesTheWorkspace(t *testing.T) {
	h := newHarness(t)
	h.res.inProcess = true
	h.svc.err = &service.Error{Code: command.CodeNotFound, Message: "the project does not exist."}

	if code := h.run("project:show", "prj_0000000001"); code == 0 {
		t.Fatal("expected a failure")
	}

	got := h.stderr.String()
	for _, want := range []string{"/ws", "--workspace", "STUARTY_HOME"} {
		if !strings.Contains(got, want) {
			t.Errorf("stderr does not say %q: %q", want, got)
		}
	}

	h.res.inProcess = false

	h.run("project:show", "prj_0000000001")

	if strings.Contains(h.stderr.String(), "--workspace") {
		t.Errorf("a server answer must not carry the note: %q", h.stderr.String())
	}
}
