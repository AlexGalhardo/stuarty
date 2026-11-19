package service_test

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/service"
)

func TestEditTextCapsTheDiff(t *testing.T) {
	t.Parallel()

	lines := make([]string, 0, 60)
	for range 60 {
		lines = append(lines, "+ item itm_x clip")
	}

	text := service.EditText(&service.EditResult{
		Revision: 2, BaseRevision: 1, Actor: "local:cli", Description: strings.Join(lines, "\n"),
	})

	if got := strings.Count(text, "+ item"); got != 40 {
		t.Fatalf("diff lines shown = %d, want 40", got)
	}

	if !strings.Contains(text, "... and 20 more changes: read them with history") {
		t.Fatalf("missing overflow line:\n%s", text)
	}
}

func TestEditTextStatesAndFindings(t *testing.T) {
	t.Parallel()

	r := &service.EditResult{
		Revision:     5,
		BaseRevision: 4,
		Actor:        "local:cli",
		DryRun:       true,
		Created:      []string{"itm_a", "itm_b"},
		Description:  `~ item itm_a: name "x" -> "y"`,
		Lint:         service.LintTotals{Errors: 1},
		Findings: []lint.Finding{
			{
				Code:     "L_X",
				Severity: "error",
				Item:     "itm_a",
				Range:    &lint.Range{Start: 0, End: 60},
				Message:  "m",
				Hint:     "h",
			},
		},
	}

	text := service.EditText(r)
	for _, want := range []string{
		"note: text in double quotes", "dry run, would commit as r5 (base r4) by local:cli: changes=0",
		"created: itm_a, itm_b", "lint (fast rules): 1 new findings (errors=1 warnings=0 in total)",
		"error L_X itm_a 0f-60f\n  m\n  hint: h",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}

	r.DryRun, r.Replayed = false, true
	if !strings.HasPrefix(service.EditText(r), "note:") || !strings.Contains(service.EditText(r), "replayed r5") {
		t.Error("a replay must say so")
	}
}

func TestUndoText(t *testing.T) {
	t.Parallel()

	u := &service.UndoResult{
		Revision: 6, BaseRevision: 5, Actor: "local:cli",
		Kind: "redo", Redone: &service.HistoryRef{Revision: 3},
		Skipped: []service.SkippedOp{{PrimitiveIndex: 2, Reason: "gone"}},
	}

	text := service.UndoText(u)
	if !strings.Contains(text, "redone r6 (base r5) by local:cli: changes=0, redid r3") ||
		!strings.Contains(text, "skipped primitive 2: gone") {
		t.Fatalf("text:\n%s", text)
	}
}

func TestErrorTextFormat(t *testing.T) {
	t.Parallel()

	op := 1
	e := &service.Error{
		Code:    "E_OVERLAP",
		Message: "overlaps",
		Hint:    "use 3s",
		OpIndex: &op,
		Path:    "p",
		Details: map[string]any{"a": 1},
	}

	want := "error E_OVERLAP: overlaps\nhint: use 3s\nretryable: false  op_index: 1  path: p\ndetails: {\"a\":1}"
	if got := e.Text(true); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if got := e.Text(false); strings.Contains(got, "details") {
		t.Fatalf("the MCP form has no details line: %q", got)
	}

	long := (&service.Error{Code: "E_X", Message: "m", Details: map[string]any{"k": string(make([]rune, 5000))}}).Text(
		true,
	)
	if n := len([]rune(long)); n > 2000 {
		t.Fatalf("the text is %d characters, cap 2000", n)
	}
}

func TestRetryableByCodeAndDocument(t *testing.T) {
	t.Parallel()

	c := &service.Error{Code: command.CodeRevisionConflict, Message: "behind"}
	if !c.IsRetryable() {
		t.Fatal("a conflict is retryable even when the flag was lost")
	}

	d := c.Document()
	if d.Type != "urn:stuarty:error:E_REVISION_CONFLICT" || d.Status != 409 || !d.Retryable ||
		d.Detail != "behind" {
		t.Fatalf("document = %+v", d)
	}

	if (&service.Error{Code: "E_NOT_FOUND"}).IsRetryable() {
		t.Fatal("not found is permanent")
	}
}
