package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

// The texts below are shared by the CLI and the MCP adapter, so that the two
// print the same words for the same result (SDD-003 sections 3.5 and 3.9).

// Caps of SDD-003 section 3.5.
const (
	editTextCap    = 6400
	diffLineCap    = 40
	diffCharCap    = 4000
	findingLineCap = 10
	skippedLineCap = 10
)

// renderEdit renders the edit, undo and redo texts of SDD-003 sections 3.5 and
// 3.9 from the result. The service result carries no frame rate, so the ranges
// of findings are printed in frames only.
func renderEdit(r *EditResult, state, suffix string, skipped []SkippedOp) string {
	var b strings.Builder

	if strings.Contains(r.Description, `"`) {
		b.WriteString(textview.Note + "\n")
	}

	fmt.Fprintf(
		&b,
		"%s r%d (base r%d) by %s: changes=%d%s\n",
		state,
		r.Revision,
		r.BaseRevision,
		r.Actor,
		len(r.Diff),
		suffix,
	)

	writeDiff(&b, r.Description)

	if len(r.Created) > 0 {
		fmt.Fprintf(&b, "created: %s\n", strings.Join(r.Created, ", "))
	}

	for i, sk := range skipped {
		if i == skippedLineCap {
			fmt.Fprintf(&b, "... and %d more skipped primitives\n", len(skipped)-i)
			break
		}

		fmt.Fprintf(&b, "skipped %s: %s\n", describeSkipped(sk), sk.Reason)
	}

	b.WriteString(lintLine(r))
	b.WriteString("\n")
	writeFindings(&b, r.Findings)

	return clip(strings.TrimRight(b.String(), "\n"), editTextCap)
}

// writeDiff prints the diff lines: at most 40 lines and 4,000 characters.
func writeDiff(b *strings.Builder, description string) {
	if description == "" {
		return
	}

	lines := strings.Split(description, "\n")
	used, shown := 0, 0

	for _, l := range lines {
		if shown == diffLineCap || used+len([]rune(l))+1 > diffCharCap {
			break
		}

		b.WriteString(l + "\n")

		used += len([]rune(l)) + 1
		shown++
	}

	if more := len(lines) - shown; more > 0 {
		fmt.Fprintf(b, "... and %d more changes: read them with history\n", more)
	}
}

func lintLine(r *EditResult) string {
	if len(r.Findings) == 0 {
		return fmt.Sprintf(
			"lint (fast rules): no new findings (errors=%d warnings=%d in total)",
			r.Lint.Errors,
			r.Lint.Warnings,
		)
	}

	return fmt.Sprintf(
		"lint (fast rules): %d new findings (errors=%d warnings=%d in total)",
		len(r.Findings),
		r.Lint.Errors,
		r.Lint.Warnings,
	)
}

func writeFindings(b *strings.Builder, findings []lint.Finding) {
	for i, f := range findings {
		if i == findingLineCap {
			fmt.Fprintf(b, "... and %d more findings: run lint\n", len(findings)-i)
			break
		}

		item, rng := "-", "-"
		if f.Item != "" {
			item = f.Item
		}

		if f.Range != nil {
			rng = fmt.Sprintf("%df-%df", f.Range.Start, f.Range.End)
		}

		fmt.Fprintf(b, "%s %s %s %s\n  %s\n  hint: %s\n", f.Severity, f.Code, item, rng, f.Message, f.Hint)
	}
}

func describeSkipped(s SkippedOp) string {
	if len(s.Fields) > 0 {
		return strings.Join(s.Fields, ",")
	}

	return fmt.Sprintf("primitive %d", s.PrimitiveIndex)
}

// UndoText is the text of an undo or a redo (SDD-003 section 3.9).
func UndoText(r *UndoResult) string {
	state, suffix := "undone", ""
	if r.Kind == "redo" {
		state = "redone"
	}

	switch {
	case r.Undone != nil:
		suffix = ", undid r" + strconv.FormatInt(r.Undone.Revision, 10)
	case r.Redone != nil:
		suffix = ", redid r" + strconv.FormatInt(r.Redone.Revision, 10)
	}

	return renderEdit(&r.EditResult, state, suffix, r.Skipped)
}

// EditText is the text of an edit result (SDD-003 section 3.5). The result carries
// no frame rate, so the ranges of findings are printed in frames only.
func EditText(r *EditResult) string { return renderEdit(r, editState(r), "", nil) }

// editState is the <state> of the first line of an edit text.
func editState(r *EditResult) string {
	switch {
	case r.DryRun:
		return "dry run, would commit as"
	case r.Replayed:
		return "replayed"
	}

	return "committed"
}

// retryableCodes are the codes SDD-000 section 8 marks retryable. The exit code
// follows the code, so a remote error that lost its flag still exits 2.
var retryableCodes = map[string]bool{
	command.CodeRevisionConflict: true,
	CodeWorkspaceLocked:          true,
	CodeQueueFull:                true,
	CodeRateLimited:              true,
	"E_ASSET_NOT_READY":          true,
}

// IsRetryable reports whether a failure is safe to retry: the flag it carries or
// a code SDD-000 section 8 marks retryable.
func (se *Error) IsRetryable() bool { return se.Retryable || retryableCodes[se.Code] }

// ErrorDocument is the error document of SDD-003 section 1.4 without the HTTP-only
// members instance and request_id.
type ErrorDocument struct {
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Status    int            `json:"status"`
	Detail    string         `json:"detail"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Hint      string         `json:"hint,omitempty"`
	Retryable bool           `json:"retryable"`
	OpIndex   *int           `json:"op_index,omitempty"`
	Path      string         `json:"path,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// Document is the error document of SDD-003 section 1.4 for this error.
func (se *Error) Document() ErrorDocument {
	return ErrorDocument{
		Type: "urn:stuarty:error:" + se.Code, Title: se.Code, Status: se.HTTPStatus(),
		Detail: se.Message, Code: se.Code, Message: se.Message, Hint: se.Hint,
		Retryable: se.IsRetryable(), OpIndex: se.OpIndex, Path: se.Path, Details: se.Details,
	}
}

const errorTextCap = 2000

// Text is the error text of SDD-003 section 3.9, capped at 2,000 characters with
// the details cut first. The MCP adapter leaves the details line out (withDetails
// false) because the envelope that follows it carries them.
func (se *Error) Text(withDetails bool) string {
	var b strings.Builder

	fmt.Fprintf(&b, "error %s: %s", se.Code, se.Message)

	if se.Hint != "" {
		fmt.Fprintf(&b, "\nhint: %s", se.Hint)
	}

	fmt.Fprintf(&b, "\nretryable: %t", se.IsRetryable())

	if se.OpIndex != nil {
		fmt.Fprintf(&b, "  op_index: %d", *se.OpIndex)
	}

	if se.Path != "" {
		fmt.Fprintf(&b, "  path: %s", se.Path)
	}

	if withDetails && len(se.Details) > 0 {
		if raw, err := json.Marshal(se.Details); err == nil {
			room := max(errorTextCap-b.Len()-len("\ndetails: "), 0)
			b.WriteString("\ndetails: ")
			b.WriteString(clip(string(raw), room))
		}
	}

	return clip(b.String(), errorTextCap)
}

func clip(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}

	return string(r[:max(limit-1, 0)]) + "…"
}
