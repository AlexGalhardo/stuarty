package recording

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The limits of the agent flow (SDD-025 section 19.2).
const (
	// MaxPreparedPerProject is the most prepared recordings a project holds at once.
	MaxPreparedPerProject = 20
	// MaxMarks is the most marks a recording holds.
	MaxMarks = 500
	// MaxPendingProposals is the most proposals of a recording that wait for a person.
	MaxPendingProposals = 5
	// MaxProposals is the most proposals a recording keeps, decided or not.
	MaxProposals = 50
	// MaxRanges is the most ranges of one proposal.
	MaxRanges = 200
	// MinRangeMS is the shortest range worth a decision.
	MinRangeMS = 100
	// MaxNoteRunes is the longest note of a mark and the longest reason of a range.
	MaxNoteRunes = 200
	// MaxPlanNotesRunes is the longest note of a prepared recording.
	MaxPlanNotesRunes = 2000
)

// The names of the limits of this file, as `details.limit` of E_LIMIT_EXCEEDED.
const (
	LimitPrepared  = "prepared_recordings"
	LimitMarks     = "recording_marks"
	LimitProposals = "recording_proposals"
)

// LimitError is a ceiling of the agent flow that was reached.
type LimitError struct {
	Limit   string
	Ceiling int
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("recording: the limit %s of %d is reached", e.Limit, e.Ceiling)
}

// ErrState is returned when the state of a recording does not allow the operation.
var ErrState = errors.New("recording: the state does not allow this")

// Plan is what a prepared recording stores: the request a person will start (SDD-025 section 19.1). It holds no
// window title, no device name and no pointer position.
type Plan struct {
	SourceKind  string `json:"source_kind"`
	SourceID    string `json:"source_id"`
	FPS         int    `json:"fps"`
	Microphone  string `json:"microphone,omitempty"`
	SystemAudio bool   `json:"system_audio,omitempty"`
	Camera      string `json:"camera,omitempty"`
	Events      bool   `json:"events,omitempty"`
	ScriptID    string `json:"script_id,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// Spec is the start request a plan stands for, for one project.
func (p Plan) Spec(projectID string) StartSpec {
	return StartSpec{
		ProjectID: projectID, SourceKind: p.SourceKind, SourceID: p.SourceID, FPS: p.FPS, Microphone: p.Microphone,
		SystemAudio: p.SystemAudio, Camera: p.Camera, Events: p.Events,
	}
}

// ValidatePrepare checks a plan like the start of a recording is checked, plus the script id and the notes. It
// returns every refused field.
func ValidatePrepare(s StartSpec, scriptID, notes string) []FieldError {
	out := ValidateStart(s)

	if s.ProjectID == "" {
		out = append([]FieldError{{"project_id", "invalid_id"}}, out...)
	}

	if scriptID != "" && !timeline.ValidID("pit", scriptID) {
		out = append(out, FieldError{"script_id", "invalid_id"})
	}

	if len([]rune(notes)) > MaxPlanNotesRunes {
		out = append(out, FieldError{"notes", "too_long"})
	}

	return out
}

// CleanNote makes the text of an agent safe to show: control and format characters other than a line break
// are dropped, runs of blank lines become one, and the text is cut to max runes. A note is never trusted.
func CleanNote(s string, maxRunes int) string {
	var lines []string

	for line := range strings.SplitSeq(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		var l strings.Builder

		for _, r := range line {
			switch {
			case r == '\t':
				l.WriteRune(' ')
			case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == unicode.ReplacementChar:
			default:
				l.WriteRune(r)
			}
		}

		text := strings.TrimRightFunc(l.String(), unicode.IsSpace)
		if strings.TrimSpace(text) == "" {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}

			continue
		}

		lines = append(lines, text)
	}

	t := strings.TrimSpace(strings.Join(lines, "\n"))
	if r := []rune(t); len(r) > maxRunes {
		t = strings.TrimSpace(string(r[:maxRunes]))
	}

	return t
}

// IsPrepared reports whether a row is a prepared recording that a person has still to start: the state
// `prepared` and a stored plan.
func IsPrepared(s State, plan string) bool { return s == Prepared && plan != "" }

// MarkKind is what a mark says about a stretch of a take.
type MarkKind string

// The kinds of a mark.
const (
	MarkGood    MarkKind = "good"
	MarkBad     MarkKind = "bad"
	MarkMistake MarkKind = "mistake"
)

// Valid reports whether k is one of the three kinds.
func (k MarkKind) Valid() bool { return k == MarkGood || k == MarkBad || k == MarkMistake }

// Mark is a point (EndMS 0) or a stretch of a take, in active milliseconds of the recording.
type Mark struct {
	N         int      `json:"n"`
	Kind      MarkKind `json:"kind"`
	AtMS      int64    `json:"at_ms"`
	EndMS     int64    `json:"end_ms,omitempty"`
	Note      string   `json:"note,omitempty"`
	Actor     string   `json:"actor"`
	CreatedMs int64    `json:"created_ms"`
}

// MarkAllowed reports whether a recording in the state can take a mark: it has time on its clock.
func MarkAllowed(s State) bool {
	return s == Recording || s == Paused || s == Finishing || s == Ready
}

// ValidateMark checks the position of a mark against the state and the duration of the recording and the marks
// it already has. durationMS is checked only for a ready recording; one still running has no end yet. It
// returns the refused fields, or an ErrState or a *LimitError.
func ValidateMark(kind MarkKind, atMS, endMS int64, s State, durationMS int64, have int) ([]FieldError, error) {
	if !MarkAllowed(s) {
		return nil, fmt.Errorf("%w: a mark on a %q recording", ErrState, s)
	}

	var out []FieldError

	if !kind.Valid() {
		out = append(out, FieldError{"kind", "unknown_kind"})
	}

	if atMS < 0 || s == Ready && atMS > durationMS {
		out = append(out, FieldError{"at_ms", "out_of_range"})
	}

	if endMS != 0 && (endMS <= atMS || s == Ready && endMS > durationMS) {
		out = append(out, FieldError{"end_ms", "out_of_range"})
	}

	if len(out) > 0 {
		return out, nil
	}

	if have >= MaxMarks {
		return nil, &LimitError{Limit: LimitMarks, Ceiling: MaxMarks}
	}

	return nil, nil
}
