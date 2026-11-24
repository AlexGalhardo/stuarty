package recording

import (
	"slices"
	"strings"
	"unicode"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Source kinds.
const (
	SourceDisplay = "display"
	SourceWindow  = "window"
)

// FrameRates is the closed set of frame rates.
var FrameRates = []int{24, 30, 60}

// DefaultFPS is the frame rate when the request names none.
const DefaultFPS = 30

// StartSpec is what a person asks for.
type StartSpec struct {
	ProjectID  string
	SourceKind string
	SourceID   string
	FPS        int
}

// FieldError is one refused field of a request.
type FieldError struct {
	Field  string
	Reason string
}

const maxSourceIDLen = 64

// ValidateStart checks a request and returns every refused field; an empty result means it is valid. FPS 0 is
// the default and is not an error.
func ValidateStart(s StartSpec) []FieldError {
	var out []FieldError

	if !timeline.ValidID("prj", s.ProjectID) {
		out = append(out, FieldError{"project_id", "invalid_id"})
	}

	if s.SourceKind != SourceDisplay && s.SourceKind != SourceWindow {
		out = append(out, FieldError{"source.kind", "unknown_kind"})
	}

	if !validSourceID(s.SourceID) {
		out = append(out, FieldError{"source.id", "invalid_format"})
	}

	if s.FPS != 0 && !slices.Contains(FrameRates, s.FPS) {
		out = append(out, FieldError{"fps", "not_allowed"})
	}

	return out
}

// FPSOrDefault is the frame rate of a request.
func FPSOrDefault(fps int) int {
	if fps == 0 {
		return DefaultFPS
	}

	return fps
}

// validSourceID accepts the opaque ids the helper makes: ASCII letters, digits, dash and underscore.
func validSourceID(id string) bool {
	if id == "" || len(id) > maxSourceIDLen {
		return false
	}

	for _, r := range id {
		ok := r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !ok {
			return false
		}
	}

	return true
}

// CleanLabel makes the name of a source printable and short: control and space characters become one space,
// and the text is cut to max runes. A window title is the user's own text and is never trusted.
func CleanLabel(s string, maxRunes int) string {
	var b strings.Builder

	prevSpace := false

	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.IsSpace(r) || r == unicode.ReplacementChar {
			r = ' '
		}

		if r == ' ' && prevSpace {
			continue
		}

		prevSpace = r == ' '

		b.WriteRune(r)
	}

	t := strings.TrimSpace(b.String())
	if r := []rune(t); len(r) > maxRunes {
		t = string(r[:maxRunes])
	}

	return t
}
