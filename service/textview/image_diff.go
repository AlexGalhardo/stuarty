package textview

import (
	"fmt"
	"strings"
)

// The budget of the diff text of an image is that of an edit result (SDD-003 sections 3.7.1 and 3.10).
const (
	// CapImageDiff is the cap of the text, in characters.
	CapImageDiff = 4000
	// MaxImageDiffLines is the cap of the lines of changes.
	MaxImageDiffLines = 40
)

// ImageDiff renders the difference between two revisions of an image (SDD-003 section 3.7.1): the note line, a
// header with the count, one line per change that fits, most significant first, and, when some were left out, a
// last line with their exact number. lines are the lines of the changes in significance order; the caller passes
// all of them and ImageDiff cuts the text at a line boundary, at 40 lines and 4,000 characters in all.
func ImageDiff(imageID string, from, to int64, lines []string) string {
	var b strings.Builder

	b.WriteString(Note + "\n")

	if len(lines) == 0 {
		fmt.Fprintf(&b, "diff %s r%d -> r%d (no changes)", imageID, from, to)

		return b.String()
	}

	shown := FitImageDiff(lines)

	fmt.Fprintf(&b, "diff %s r%d -> r%d (%d changes)", imageID, from, to, len(lines))

	for _, l := range lines[:shown] {
		b.WriteString("\n" + l)
	}

	if left := len(lines) - shown; left > 0 {
		fmt.Fprintf(&b, "\n... and %d more changes: read the diff of a narrower range, or the history", left)
	}

	return b.String()
}

// FitImageDiff is how many lines fit the budget, at least one when any.
func FitImageDiff(lines []string) int {
	used := runeLen(Note) + 120 + diffTrailer
	n := 0

	for _, l := range lines {
		cost := runeLen(l) + 1
		if n > 0 && (n == MaxImageDiffLines || used+cost > CapImageDiff) {
			break
		}

		used += cost
		n++
	}

	return n
}
