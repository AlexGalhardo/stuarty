package textview

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeval"
)

// Caps of SDD-003 section 3.10 for the media and job texts, in characters.
const (
	CapMediaList = 12800
	CapJobList   = 6400
	CapShort     = 2000
)

// MediaLine is one asset of the media list.
type MediaLine struct {
	ID            string
	Kind          string
	State         string
	Name          string
	DurationFrame *int64 // nil until probed, and for an image
	Width, Height int
	HasAudio      bool
	Percent       int // shown while the asset is not ready
	ErrorCode     string
	ErrorMessage  string
}

func mediaLine(fps timeval.FPS, l *MediaLine) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s %s %s", l.ID, orDash(l.Kind), l.State, Quote(l.Name, 80))

	if l.DurationFrame != nil {
		b.WriteString(" " + Cell(fps, *l.DurationFrame))
	}

	if l.Width > 0 {
		fmt.Fprintf(&b, " %dx%d", l.Width, l.Height)
	}

	if l.HasAudio {
		b.WriteString(" audio")
	}

	if l.State == "pending" || l.State == "ingesting" {
		fmt.Fprintf(&b, " %d%%", l.Percent)
	}

	if l.State == "failed" {
		fmt.Fprintf(&b, " FAILED %s: %s", l.ErrorCode, oneLine(l.ErrorMessage, 160))
	}

	return b.String()
}

// MediaList renders the list mode of media_inspect (SDD-003 section 3.8). The
// caller appends next_cursor when more follow.
func MediaList(projectID string, fps timeval.FPS, lines []MediaLine, total int) string {
	var b strings.Builder

	b.WriteString(Note + "\n")
	fmt.Fprintf(&b, "media %s (showing %d of %d)", projectID, len(lines), total)

	for i := range lines {
		b.WriteString("\n" + mediaLine(fps, &lines[i]))
	}

	return b.String()
}

// FitMediaList is how many lines fit the cap, at least one when any.
func FitMediaList(fps timeval.FPS, lines []MediaLine) int {
	used := runeLen(Note) + 80 + cursorReserve
	n := 0

	for i := range lines {
		cost := runeLen("\n" + mediaLine(fps, &lines[i]))
		if n > 0 && used+cost > CapMediaList {
			break
		}

		used += cost
		n++
	}

	return n
}

// ImportLine is one file of an import.
type ImportLine struct {
	ID           string
	Kind         string
	State        string
	Name         string
	Deduplicated bool
	JobID        string
	ErrorCode    string
	ErrorMessage string
	ErrorHint    string
}

// Import renders the answer of an import: the note, the count of failures
// when above zero, and one line per file (SDD-003 section 3.9).
func Import(lines []ImportLine) string {
	var b strings.Builder

	b.WriteString(Note)

	failed := 0

	for i := range lines {
		if lines[i].ErrorCode != "" {
			failed++
		}
	}

	if failed > 0 {
		fmt.Fprintf(&b, "\n%d of %d files failed, %d imported", failed, len(lines), len(lines)-failed)
	}

	for i := range lines {
		l := &lines[i]

		if l.ErrorCode != "" {
			fmt.Fprintf(&b, "\nFAILED %s: %s %s", Quote(l.Name, 80), l.ErrorCode, oneLine(l.ErrorMessage, 160))

			if l.ErrorHint != "" {
				fmt.Fprintf(&b, "\n  hint: %s", oneLine(l.ErrorHint, 200))
			}

			continue
		}

		fmt.Fprintf(&b, "\n%s %s %s %s", l.ID, orDash(l.Kind), l.State, Quote(l.Name, 80))

		if l.Deduplicated {
			b.WriteString(" deduplicated")
		}

		if l.JobID != "" {
			b.WriteString(" job " + l.JobID)
		}
	}

	return b.String()
}

// JobView is what the job text needs.
type JobView struct {
	ID       string
	Kind     string
	State    string
	Percent  int
	Message  string
	File     string // set for a succeeded export, local callers only
	URL      string // the download path of a succeeded export, when File is empty
	Audio    bool   // the export has no picture: no dimensions
	Width    int
	Height   int
	Duration string // a time cell
	SizeText string
	Error    string   // the error text, for a failed job
	Warnings []string // notes about the job, one line each
	Waited   bool     // a wait elapsed without a final state
	Estimate string   // the estimated cost of a generation, before it runs
	Cost     string   // the real cost of a finished generation
	Output   string   // the text a model wrote, already delimited as untrusted text
	Files    []string // the assets a generation stored, one line each: id, kind and size
}

// Job renders one job (SDD-003 section 3.9).
func Job(j JobView) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s %s %d%%", j.ID, j.Kind, j.State, j.Percent)

	// A succeeded job keeps the last progress message ("Rendering (98%)"), which
	// would contradict the state.
	if j.Message != "" && j.State != "succeeded" {
		b.WriteString(" " + oneLine(j.Message, 120))
	}

	if j.State == "succeeded" && j.Kind == "export" {
		switch {
		case j.File != "":
			b.WriteString(" file=" + j.File)
		case j.URL != "":
			b.WriteString(" url=" + j.URL)
		}

		// A bundle has no picture and reports no size, like an audio export (VV-016).
		if !j.Audio && j.Width > 0 && j.Height > 0 {
			fmt.Fprintf(&b, " %dx%d", j.Width, j.Height)
		}

		if j.Duration != "" {
			b.WriteString(" " + j.Duration)
		}

		b.WriteString(" " + j.SizeText)
	}

	if j.Estimate != "" {
		b.WriteString("\nestimated cost " + j.Estimate)
	}

	if j.State == "succeeded" && j.Output != "" {
		b.WriteString("\ncost " + j.Cost + "\n" + j.Output)
	}

	if j.State == "succeeded" && len(j.Files) > 0 {
		b.WriteString("\ncost " + j.Cost)

		for _, f := range j.Files {
			b.WriteString("\nasset " + oneLine(f, 120))
		}
	}

	for _, w := range j.Warnings {
		b.WriteString("\n" + oneLine(w, 300))
	}

	if j.State == "failed" && j.Error != "" {
		b.WriteString("\n" + j.Error)
	}

	if j.Waited && (j.State == "queued" || j.State == "running") {
		fmt.Fprintf(&b, "\nstill %s: call job_get again", j.State)
	}

	return b.String()
}

// JobList renders the job list; lines are Job texts. The caller appends
// next_cursor when more follow.
func JobList(projectID string, lines []string, total int) string {
	var b strings.Builder

	fmt.Fprintf(&b, "jobs %s (showing %d of %d)", projectID, len(lines), total)

	for _, l := range lines {
		b.WriteString("\n" + l)
	}

	return b.String()
}

// Size is a byte count in GiB, MiB or KiB with one decimal; a whole number of
// GiB has no decimal ("8 GiB").
func Size(n int64) string {
	if n >= 1<<30 {
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/(1<<30)), ".0") + " GiB"
	}

	if n >= 1<<20 {
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	}

	return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}

	return s
}

// oneLine keeps a message on one line and cuts it to n characters.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")

	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}

	return s
}
