package service

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/textview"
)

// withNote starts a text that quotes a name or a text of the document with the note of SDD-007 section 8.2.
func withNote(text string) string {
	if strings.Contains(text, `"`) && !strings.HasPrefix(text, textview.Note) {
		return textview.Note + "\n" + text
	}

	return text
}

// ImageEditText is the text of the result of an image batch, the form the CLI prints and the MCP tool returns:
// the state and the revision, what changed, what was created and the findings the batch introduced.
func ImageEditText(r *ImageEditResult) string { return withNote(imageEditBody(r)) }

func imageEditBody(r *ImageEditResult) string {
	state := "committed"

	switch {
	case !r.Applied && !r.DryRun:
		state = "nothing to commit"
	case r.DryRun:
		state = "dry run, would commit as"
	case r.Replayed:
		state = "replayed"
	}

	var b strings.Builder

	fmt.Fprintf(&b, "image %s r%d %s (base r%d, %d operations) lint errors=%d warnings=%d",
		r.ImageID, r.Revision, state, r.BaseRevision, len(r.Effective), r.Lint.Errors, r.Lint.Warnings)

	if r.Description != "" {
		b.WriteString("\n" + r.Description)
	}

	if len(r.Created) > 0 {
		b.WriteString("\ncreated: " + strings.Join(r.Created, " "))
	}

	for _, f := range r.Findings {
		fmt.Fprintf(&b, "\n%s %s %s %s", f.Severity, f.Code, f.Item, f.Message)
	}

	return b.String()
}

// ImageStepText is the text of an undo or a redo of an image.
func ImageStepText(r *ImageStepResult) string {
	head := fmt.Sprintf("%s of r%d: ", r.Kind, r.Step)
	text := imageEditBody(&r.ImageEditResult)

	if len(r.Skipped) > 0 {
		var skipped []string
		for i := range r.Skipped {
			s := &r.Skipped[i]
			skipped = append(skipped, fmt.Sprintf("%s %s %s", s.Reason, s.Primitive.Collection, s.Primitive.ID))
		}

		text += "\nskipped: " + strings.Join(skipped, "; ")
	}

	return withNote(head + text)
}

// ImageExportText is the text of an image export.
func ImageExportText(r *ImageExportResult) string { return r.Text + "\n" + r.Path }
