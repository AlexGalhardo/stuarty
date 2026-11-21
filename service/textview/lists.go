package textview

import (
	"fmt"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/timeval"
)

// ProjectLine is one project of the listing.
type ProjectLine struct {
	ID            string
	Name          string
	Revision      int64
	Width, Height int
	FPS           timeval.FPS
	DurationFrame int64
	Items         int
	// Timelines is how many timelines the project holds; it is printed only when there is more than one, so the
	// line of a project that has one is what it always was.
	Timelines int
	Trashed   bool
}

// ProjectList renders the project list of SDD-003 section 3.9: the note, a
// header with the count shown and the total, and one line per project. total
// is the number of projects in the state; state is "active", "trashed" or
// "all". The caller appends next_cursor when more follow.
func ProjectList(lines []ProjectLine, total int, state string) string {
	var b strings.Builder

	b.WriteString(Note + "\n")
	fmt.Fprintf(&b, "projects (%d of %d, %s)", len(lines), total, state)

	for _, l := range lines {
		if l.FPS.Num == 0 {
			// A project made since epic 56 has no timeline until one is added: it has no size and no duration.
			fmt.Fprintf(&b, "\n%s %s no timeline", l.ID, Quote(l.Name, 60))
		} else {
			fmt.Fprintf(&b, "\n%s %s r%d %dx%d %sfps %s items=%d", l.ID, Quote(l.Name, 60), l.Revision,
				l.Width, l.Height, l.FPS, Cell(l.FPS, l.DurationFrame), l.Items)
		}

		if l.Timelines > 1 {
			fmt.Fprintf(&b, " timelines=%d", l.Timelines)
		}

		if l.Trashed {
			b.WriteString(" trashed")
		}
	}

	return b.String()
}

// FitProjectList is how many of the lines fit the cap, at least one when any.
func FitProjectList(lines []ProjectLine) int {
	used := runeLen(Note) + 80 + cursorReserve
	n := 0

	for _, l := range lines {
		cost := runeLen(fmt.Sprintf("\n%s %s no timeline trashed", l.ID, Quote(l.Name, 60)))
		if l.FPS.Num != 0 {
			cost = runeLen(
				fmt.Sprintf("\n%s %s r%d %dx%d %sfps %s items=%d trashed", l.ID, Quote(l.Name, 60), l.Revision,
					l.Width, l.Height, l.FPS, Cell(l.FPS, l.DurationFrame), l.Items),
			)
		}

		cost += len(" timelines=99")
		if n > 0 && used+cost > CapProjectList {
			break
		}

		used += cost
		n++
	}

	return n
}

// Created is the text of a created project (SDD-003 section 3.9).
func Created(id, name string, width, height int, fps timeval.FPS) string {
	return fmt.Sprintf(
		"%s\ncreated %s %s r0 %dx%d %sfps; add tracks with edit op track.add",
		Note,
		id,
		Quote(name, 60),
		width,
		height,
		fps,
	)
}

// CreatedProject is the text of a created project, which has no timeline until one is added (SDD-005 section
// 5.12.2).
func CreatedProject(id, name string) string {
	return fmt.Sprintf(
		"%s\ncreated %s %s; it has no timeline yet: add one with project_item action add kind timeline",
		Note,
		id,
		Quote(name, 60),
	)
}

// UpdatedProject is the text of a project whose name, description or goal changed.
func UpdatedProject(id, name string, complete bool) string {
	state := "complete"
	if !complete {
		state = "incomplete: the description and the goal are not all set"
	}

	return fmt.Sprintf("%s\nupdated %s %s; context %s", Note, id, Quote(name, 60), state)
}

// HistoryLine is one step of the history.
type HistoryLine struct {
	Revision int64
	Kind     string // names an undo, redo, migrate or import step, which has no operations
	At       time.Time
	Actor    string
	Ops      []string
	UndoOf   *int64
	UndoneBy *int64
}

// History renders the history list of SDD-003 section 3.7. total is the number
// of steps of the project (or of the actor).
func History(projectID string, revision int64, lines []HistoryLine, total int64) string {
	var b strings.Builder

	fmt.Fprintf(&b, "history %s rev %d (showing %d of %d, newest first)", projectID, revision, len(lines), total)

	for _, l := range lines {
		fmt.Fprintf(
			&b,
			"\nr%d %s %s ops=%d %s",
			l.Revision,
			l.At.UTC().Format("2006-01-02T15:04:05Z"),
			l.Actor,
			len(l.Ops),
			names(l.Kind, l.Ops),
		)

		if l.UndoOf != nil {
			fmt.Fprintf(&b, " undo-of:r%d", *l.UndoOf)
		}

		if l.UndoneBy != nil {
			fmt.Fprintf(&b, " undone-by:r%d", *l.UndoneBy)
		}
	}

	return b.String()
}

// FitHistory is how many lines fit the cap, at least one when any.
func FitHistory(lines []HistoryLine) int {
	used := 120 + cursorReserve
	n := 0

	for _, l := range lines {
		cost := runeLen(
			fmt.Sprintf(
				"\nr%d %s %s ops=%d %s undo-of:r%d undone-by:r%d",
				l.Revision,
				"2006-01-02T15:04:05Z",
				l.Actor,
				len(l.Ops),
				names(l.Kind, l.Ops),
				l.Revision,
				l.Revision,
			),
		)
		if n > 0 && used+cost > CapHistory {
			break
		}

		used += cost
		n++
	}

	return n
}

// names are at most three command names joined by comma, then +<k>. A step
// with no operations is named by its kind.
func names(kind string, ops []string) string {
	if len(ops) == 0 {
		return kind
	}

	if len(ops) <= 3 {
		return strings.Join(ops, ",")
	}

	return fmt.Sprintf("%s,+%d", strings.Join(ops[:3], ","), len(ops)-3)
}
