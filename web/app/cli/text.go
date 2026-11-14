package cli

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/lint"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/textview"
)

// Caps of SDD-003 section 3.5.
const (
	editTextCap    = 6400
	diffLineCap    = 40
	diffCharCap    = 4000
	findingLineCap = 10
	skippedLineCap = 10
)

// editText renders the edit, undo and redo texts of SDD-003 sections 3.5 and
// 3.9 from the result. The service result carries no frame rate, so the ranges
// of findings are printed in frames only.
func editText(r *service.EditResult, state, suffix string, skipped []service.SkippedOp) string {
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

	return truncate(strings.TrimRight(b.String(), "\n"), editTextCap)
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

func lintLine(r *service.EditResult) string {
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

func describeSkipped(s service.SkippedOp) string {
	if len(s.Fields) > 0 {
		return strings.Join(s.Fields, ",")
	}

	return fmt.Sprintf("primitive %d", s.PrimitiveIndex)
}

// undoText renders an undo or a redo.
func undoText(r *service.UndoResult) string {
	state, suffix := "undone", ""
	if r.Kind == "redo" {
		state = "redone"
	}

	switch {
	case r.Undone != nil:
		suffix = ", undid r" + itoa(r.Undone.Revision)
	case r.Redone != nil:
		suffix = ", redid r" + itoa(r.Redone.Revision)
	}

	return editText(&r.EditResult, state, suffix, r.Skipped)
}

// editState is the <state> of the first line of an edit text.
func editState(r *service.EditResult) string {
	switch {
	case r.DryRun:
		return "dry run, would commit as"
	case r.Replayed:
		return "replayed"
	}

	return "committed"
}
