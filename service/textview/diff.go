package textview

import (
	"fmt"
	"strings"
)

// CapDiff is the cap of the diff text, in characters (SDD-003 section 3.10).
const CapDiff = 6400

// diffTrailer is the room kept for the line that says how many changes were left out.
const diffTrailer = 120

// Diff renders the difference between two revisions (SDD-003 section 3.7.1): the note line, a header with
// the count, one line per change that fits, most significant first, and, when some were left out, a last line
// with their exact number. lines are the DescribeDiff lines of the changes in significance order; the caller
// passes all of them and Diff cuts the text at a line boundary within CapDiff.
func Diff(projectID string, from, to int64, lines []string) string {
	var b strings.Builder

	b.WriteString(Note + "\n")

	if len(lines) == 0 {
		fmt.Fprintf(&b, "diff %s r%d -> r%d (no changes)", projectID, from, to)

		return b.String()
	}

	shown := FitDiff(lines)

	fmt.Fprintf(&b, "diff %s r%d -> r%d (%d changes)", projectID, from, to, len(lines))

	for _, l := range lines[:shown] {
		b.WriteString("\n" + l)
	}

	if left := len(lines) - shown; left > 0 {
		fmt.Fprintf(&b, "\n... and %d more changes: read the diff of a narrower range, or the history", left)
	}

	return b.String()
}

// FitDiff is how many lines fit the cap, at least one when any.
func FitDiff(lines []string) int {
	used := runeLen(Note) + 120 + diffTrailer
	n := 0

	for _, l := range lines {
		cost := runeLen(l) + 1
		if n > 0 && used+cost > CapDiff {
			break
		}

		used += cost
		n++
	}

	return n
}
