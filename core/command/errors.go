package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Codes of the core (SDD-002 §A.8). Service codes are in SDD-003.
const (
	CodeUnknownCommand      = "E_UNKNOWN_COMMAND"
	CodeValidation          = "E_VALIDATION"
	CodeInvalidTime         = "E_INVALID_TIME"
	CodeNotFound            = "E_NOT_FOUND"
	CodeIDExists            = "E_ID_EXISTS"
	CodeTypeMismatch        = "E_TYPE_MISMATCH"
	CodeOverlap             = "E_OVERLAP"
	CodeOutOfSource         = "E_OUT_OF_SOURCE"
	CodeInvalidState        = "E_INVALID_STATE"
	CodeInUse               = "E_IN_USE"
	CodeNotEmpty            = "E_NOT_EMPTY"
	CodeLimit               = "E_LIMIT"
	CodeFontUnknown         = "E_FONT_UNKNOWN"
	CodeSlot                = "E_SLOT"
	CodeRevisionConflict    = "E_REVISION_CONFLICT"
	CodeIdempotencyMismatch = "E_IDEMPOTENCY_MISMATCH"
	CodeInternal            = "E_INTERNAL"
)

// codeTable lists every core code and whether a retry can succeed.
var codeTable = map[string]bool{
	CodeUnknownCommand:      false,
	CodeValidation:          false,
	CodeInvalidTime:         false,
	CodeNotFound:            false,
	CodeIDExists:            false,
	CodeTypeMismatch:        false,
	CodeOverlap:             false,
	CodeOutOfSource:         false,
	CodeInvalidState:        false,
	CodeInUse:               false,
	CodeNotEmpty:            false,
	CodeLimit:               false,
	CodeFontUnknown:         false,
	CodeSlot:                false,
	CodeRevisionConflict:    true,
	CodeIdempotencyMismatch: false,
	CodeInternal:            false,
}

// Codes returns every code of the core, sorted.
func Codes() []string {
	out := make([]string, 0, len(codeTable))
	for c := range codeTable {
		out = append(out, c)
	}
	slices.Sort(out)
	return out
}

// KnownCode reports whether code is a code of the core.
func KnownCode(code string) bool {
	_, ok := codeTable[code]
	return ok
}

// Error is the error of every command, with the shape of SDD-002 §A.8. The
// message states what was wrong with the actual values; the hint states one
// concrete way to succeed; neither holds a stack trace, a file path of the server
// or SQL (CheckText).
type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Hint      string         `json:"hint,omitempty"`
	Retryable bool           `json:"retryable"`
	OpIndex   *int           `json:"op_index,omitempty"`
	Path      string         `json:"path,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

// Errorf returns an error with the given code; Retryable follows the code.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, quoted(args)...), Retryable: codeTable[code]}
}

// quotedMax is the longest string a message quotes back to the caller (SDD-003 section 1.2 rule 9):
// a value of 100 KB sent in an argument must not come back whole.
const quotedMax = 256

// quoted shortens the string arguments of a message, keeping its first runes and marking the cut.
func quoted(args []any) []any {
	var out []any

	for i, a := range args {
		s, ok := a.(string)
		if !ok {
			continue
		}

		short := Shortened(s)
		if short == s {
			continue
		}

		if out == nil {
			out = slices.Clone(args)
		}

		out[i] = short
	}

	if out == nil {
		return args
	}

	return out
}

// safeKeyShape is the shape of an argument key that an error may repeat: it cannot break a line, close a quote,
// reorder the text or fill the error block.
var safeKeyShape = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// IsSafeKey says whether a key chosen by the caller may be repeated in an error (SDD-003 section 1.2 rule 9).
func IsSafeKey(k string) bool { return safeKeyShape.MatchString(k) }

// SafeKey is k when an error may repeat it and "?" when it may not. Every message that names a key the caller chose
// (an argument, a style key, a briefing answer) goes through it, so no surface echoes raw caller text as a key.
func SafeKey(k string) string {
	if IsSafeKey(k) {
		return k
	}

	return "?"
}

// SafePath is a dotted path of members with every segment that an error may not repeat taken out.
func SafePath(p string) string {
	parts := strings.Split(p, ".")
	for i, s := range parts {
		parts[i] = SafeKey(s)
	}

	return strings.Join(parts, ".")
}

// UnknownMember is the name a decoder refused with DisallowUnknownFields, quoted and safe to repeat, and whether
// err is that refusal.
func UnknownMember(err error) (string, bool) {
	quotedName, found := strings.CutPrefix(err.Error(), "json: unknown field ")
	if !found {
		return "", false
	}

	name, uerr := strconv.Unquote(quotedName)
	if uerr != nil {
		return `"?"`, true
	}

	return strconv.Quote(SafeKey(name)), true
}

// JSONProblem says in a few words what is wrong with a decoded JSON argument, without repeating a key the caller
// chose: the decoder's own message quotes the member names.
func JSONProblem(err error) string {
	if ute, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		if ute.Field != "" {
			return "the member " + SafePath(ute.Field) + " has the wrong type"
		}

		return "a value has the wrong type"
	}

	if name, ok := UnknownMember(err); ok {
		return "the member " + name + " is not known"
	}

	if se, ok := errors.AsType[*json.SyntaxError](err); ok {
		return fmt.Sprintf("it is not valid JSON (at byte %d)", se.Offset)
	}

	return "they are not valid JSON"
}

// Shortened is s as a message quotes it: its first runes and the mark of the cut when it is longer than quotedMax.
func Shortened(s string) string {
	if len(s) <= quotedMax {
		return s
	}

	r := []rune(s)
	if len(r) <= quotedMax {
		return s
	}

	return string(r[:quotedMax]) + "... (" + strconv.Itoa(len(r)) + " characters)"
}

// WithHint sets the hint.
func (e *Error) WithHint(format string, args ...any) *Error {
	e.Hint = fmt.Sprintf(format, args...)
	return e
}

// WithPath sets the path of the offending value, for example "args.start".
func (e *Error) WithPath(path string) *Error {
	e.Path = path
	return e
}

// WithDetail adds one machine-readable detail.
func (e *Error) WithDetail(key string, value any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[key] = value
	return e
}

// AsError returns err as an *Error. Anything that is not one is a bug in a
// command and becomes E_INTERNAL, without the text of the original error.
func AsError(err error) *Error {
	if ce, ok := errors.AsType[*Error](err); ok {
		return ce
	}
	return Errorf(CodeInternal, "the operation failed because of an internal error; the batch was not applied.").
		WithHint("Report this as a bug with the operation that was sent.")
}

var pathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[A-Za-z]:[\\/]`),                   // C:\ or C:/
	regexp.MustCompile(`(^|[\s"'(=])/[\w.-]+/[\w.-]+`),     // /usr/lib
	regexp.MustCompile(`\\[\w.-]+\\[\w.-]+`),               // \Users\x
	regexp.MustCompile(`\.go:\d+`),                         // file.go:12
	regexp.MustCompile(`\b[\w-]+\.(go|ts|js|sql)\b`),       // a source file name
	regexp.MustCompile(`(^|\s)\.{1,2}/[\w.-]+`),            // ./x, ../x
	regexp.MustCompile(`goroutine \d+|\bpanic:|runtime\.`), // a stack trace
	regexp.MustCompile(`(?i)\bat [\w./*()-]+\(`),           // at pkg.Func(
	regexp.MustCompile(`(?i)\b(select .+ from|insert into|delete from|update \w+ set)\b`),
}

// CheckText returns an error when s contains a file path, a stack trace or SQL.
// Tests run it on every message and hint.
func CheckText(s string) error {
	for _, p := range pathPatterns {
		if m := p.FindString(s); m != "" {
			return fmt.Errorf("text contains a path, stack trace or SQL (%q)", strings.TrimSpace(m))
		}
	}
	return nil
}

// CheckError returns an error when e breaks the rules of SDD-002 §A.8: a known
// code, a message, a hint (except for E_INTERNAL), the retryable flag that goes
// with the code, and no path, stack trace or SQL in the texts.
func CheckError(e *Error) error {
	switch {
	case e == nil:
		return errors.New("nil error")
	case !KnownCode(e.Code):
		return fmt.Errorf("unknown code %q", e.Code)
	case strings.TrimSpace(e.Message) == "":
		return fmt.Errorf("%s has no message", e.Code)
	case e.Hint == "" && e.Code != CodeInternal:
		return fmt.Errorf("%s has no hint", e.Code)
	case e.Retryable != codeTable[e.Code]:
		return fmt.Errorf("%s has retryable=%v", e.Code, e.Retryable)
	case e.OpIndex != nil && *e.OpIndex < 0:
		return fmt.Errorf("%s has a negative op_index", e.Code)
	}
	if err := CheckText(e.Message); err != nil {
		return fmt.Errorf("%s message: %w", e.Code, err)
	}
	if err := CheckText(e.Hint); err != nil {
		return fmt.Errorf("%s hint: %w", e.Code, err)
	}
	return nil
}
