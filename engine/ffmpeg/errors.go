package ffmpeg

import (
	"errors"
	"regexp"
	"strings"
)

// Error codes of SDD-000 section 8 that this package raises.
const (
	CodeFFmpegMissing    = "E_FFMPEG_MISSING"
	CodeNoEncoder        = "E_NO_ENCODER"
	CodeUnsupportedMedia = "E_UNSUPPORTED_MEDIA"
	CodeLimitExceeded    = "E_LIMIT_EXCEEDED"
	CodeInternal         = "E_INTERNAL"
)

// Error carries an SDD-000 error code, a machine readable reason, what to do about it
// and the details of SDD-011 section 5.3, so an adapter maps it without parsing text.
type Error struct {
	Code    string
	Reason  string // details.reason
	Message string
	Hint    string
	Details map[string]any
	Err     error
}

func (e *Error) Error() string {
	s := e.Code + ": " + e.Message
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// Sentinels a caller tells apart with errors.Is.
var (
	// ErrStalled means -progress showed no advance for the stall window and the run was killed.
	ErrStalled = errors.New("ffmpeg: no progress, run stalled")
	// ErrDeadline means the run exceeded its deadline and was killed.
	ErrDeadline = errors.New("ffmpeg: deadline exceeded")
)

var (
	// Windows drive paths, UNC paths and absolute Unix paths in tool output.
	pathRe = regexp.MustCompile(`(^|[\s'"(=:])(?:[A-Za-z]:[\\/]|\\\\|/)[^\s'",)]*`)
	ctrlRe = regexp.MustCompile(`[\x00-\x08\x0b-\x1f\x7f]`)
)

// scrub removes paths and control characters from tool output and cuts it to limit
// characters. Messages and details never carry a server path (SDD-002 A.8).
func scrub(s string, limit int) string {
	s = ctrlRe.ReplaceAllString(s, "")
	s = pathRe.ReplaceAllString(s, "${1}<path>")
	s = strings.TrimSpace(s)
	if len(s) > limit {
		s = s[len(s)-limit:]
	}
	return s
}

func internalErr(msg string, err error) *Error {
	return &Error{Code: CodeInternal, Reason: "ffmpeg_internal", Message: msg, Err: err}
}
