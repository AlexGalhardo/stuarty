package textview

import (
	"fmt"
	"strings"
	"time"
)

// CapCheckpoints is the cap of the checkpoint list text, in characters (SDD-003 section 3.10).
const CapCheckpoints = 6400

// CheckpointLine is one checkpoint of the list.
type CheckpointLine struct {
	Revision int64
	Name     string
	Note     string
	Actor    string
	At       time.Time
	Auto     bool
}

// Checkpoints renders the checkpoint list of SDD-003 section 3.7.1: the note line, a header, and one
// line per checkpoint, newest revision first. total is the number of checkpoints of the timeline; the
// caller passes the lines that fit (FitCheckpoints) and the text says how many were left out.
func Checkpoints(projectID string, revision int64, lines []CheckpointLine, total int) string {
	var b strings.Builder

	b.WriteString(Note + "\n")
	fmt.Fprintf(&b, "checkpoints %s rev %d (showing %d of %d)", projectID, revision, len(lines), total)

	for _, l := range lines {
		b.WriteString("\n" + checkpointLine(l))
	}

	if left := total - len(lines); left > 0 {
		fmt.Fprintf(&b, "\n... and %d more checkpoints", left)
	}

	return b.String()
}

func checkpointLine(l CheckpointLine) string {
	s := fmt.Sprintf("r%d %s %s %s", l.Revision, Quote(l.Name, 60), l.Actor, l.At.UTC().Format("2006-01-02T15:04:05Z"))

	if l.Auto {
		s += " auto"
	}

	if l.Note != "" {
		s += " - " + Quote(l.Note, 300)
	}

	return s
}

// FitCheckpoints is how many lines fit the cap, at least one when any.
func FitCheckpoints(lines []CheckpointLine) int {
	used := runeLen(Note) + 160
	n := 0

	for _, l := range lines {
		cost := runeLen("\n" + checkpointLine(l))
		if n > 0 && used+cost > CapCheckpoints {
			break
		}

		used += cost
		n++
	}

	return n
}
