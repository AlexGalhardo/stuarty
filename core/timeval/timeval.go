// Package timeval parses time values and does frame arithmetic.
//
// A time in the timeline document is a non-negative integer number of frames at
// the project's rational frame rate (ADR-0004). On every external surface it is a
// string with an explicit unit: "12.5s", "375f" or "00:00:12.500". All arithmetic
// here is exact integer arithmetic; floats appear only in the reported seconds.
package timeval

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// CodeInvalidTime is the error code for a time string that cannot be read (SDD-000).
const CodeInvalidTime = "E_INVALID_TIME"

// MaxFrame is the largest frame number Parse accepts (about 1,000 years at 30 fps).
// It keeps every derived millisecond count inside int64 for frame rates up to 1,000.
const MaxFrame int64 = 1 << 40

const (
	maxInputLen = 64
	maxRate     = 1_000_000
	timeHint    = `use one of "12.5s" (seconds), "375f" (frames) or "00:00:12.500" (timecode)`
)

// Error is the error returned for an unreadable time. Code is always CodeInvalidTime.
type Error struct {
	Code    string
	Message string
	Hint    string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Message + " (" + e.Hint + ")"
}

func invalid(input, why string) *Error {
	return &Error{Code: CodeInvalidTime, Message: fmt.Sprintf("cannot read time %q: %s", input, why), Hint: timeHint}
}

// ErrInvalidFPS is returned when a frame rate is not usable.
var ErrInvalidFPS = errors.New("timeval: frame rate must be a rational num/den between 1 and 1000 frames per second")

// FPS is a rational frame rate, for example {30, 1} or {30000, 1001}.
type FPS struct {
	Num int64 `json:"num"`
	Den int64 `json:"den"`
}

// NewFPS returns the frame rate num/den, or ErrInvalidFPS.
func NewFPS(num, den int64) (FPS, error) {
	f := FPS{Num: num, Den: den}
	if !f.Valid() {
		return FPS{}, ErrInvalidFPS
	}
	return f, nil
}

// Valid reports whether the rate is between 1 and 1,000 frames per second with
// numerator and denominator no larger than 1,000,000.
func (f FPS) Valid() bool {
	return f.Num > 0 && f.Den > 0 && f.Num <= maxRate && f.Den <= maxRate && f.Num >= f.Den && f.Num <= 1000*f.Den
}

func (f FPS) String() string {
	if f.Den == 1 {
		return fmt.Sprintf("%d", f.Num)
	}
	return fmt.Sprintf("%d/%d", f.Num, f.Den)
}

// roundDiv returns n/d rounded to the nearest integer, halves away from zero. d > 0.
func roundDiv(n, d *big.Int) *big.Int {
	neg := n.Sign() < 0
	a := new(big.Int).Abs(n)
	twice := new(big.Int).Lsh(a, 1)
	twice.Add(twice, d)
	q := twice.Div(twice, new(big.Int).Lsh(d, 1))
	if neg {
		q.Neg(q)
	}
	return q
}

// Millis returns the time of the start of a frame in milliseconds, rounded to the
// nearest one (half away from zero).
func (f FPS) Millis(frame int64) int64 {
	n := new(big.Int).Mul(big.NewInt(frame), big.NewInt(1000*f.Den))
	return roundDiv(n, big.NewInt(f.Num)).Int64()
}

// FrameAtMillis returns the frame nearest to a time in milliseconds (half away from zero).
func (f FPS) FrameAtMillis(ms int64) int64 {
	n := new(big.Int).Mul(big.NewInt(ms), big.NewInt(f.Num))
	return roundDiv(n, big.NewInt(1000*f.Den)).Int64()
}

// Seconds returns the time of a frame in seconds with millisecond precision. It is
// for reporting only; never convert it back with float arithmetic.
func (f FPS) Seconds(frame int64) float64 {
	return float64(f.Millis(frame)) / 1000
}

// Effective is the triple every response reports for an applied time (SDD-002 A.4).
type Effective struct {
	Frame     int64   `json:"frame"`
	Seconds   float64 `json:"seconds"`
	Timecode  string  `json:"timecode"`
	Requested string  `json:"requested,omitempty"`
}

// Effective returns the frame, its seconds and its timecode.
func (f FPS) Effective(frame int64) Effective {
	return Effective{Frame: frame, Seconds: f.Seconds(frame), Timecode: f.Timecode(frame)}
}

// Timecode formats a frame as HH:MM:SS.mmm.
func (f FPS) Timecode(frame int64) string {
	ms := f.Millis(frame)
	sign := ""
	if ms < 0 {
		sign, ms = "-", -ms
	}
	return fmt.Sprintf("%s%02d:%02d:%02d.%03d", sign, ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
}

// FormatSeconds formats a frame as decimal seconds with a trailing "s", for
// example "12.467s" or "2s", with at most three decimals.
func (f FPS) FormatSeconds(frame int64) string {
	ms := f.Millis(frame)
	sign := ""
	if ms < 0 {
		sign, ms = "-", -ms
	}
	s := fmt.Sprintf("%d.%03d", ms/1000, ms%1000)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return sign + s + "s"
}

// FormatFrames formats a frame as "375f".
func FormatFrames(frame int64) string {
	return fmt.Sprintf("%df", frame)
}

// Parse reads a time string and returns the nearest frame at fps, rounding half
// away from zero. It accepts seconds ("12.5s"), frames ("375f") and timecode
// ("00:00:12.500" or "01:02.5"). A bare number, a negative value, an unknown unit
// or any other text is rejected with an *Error whose Code is CodeInvalidTime.
func Parse(s string, fps FPS) (int64, error) {
	if !fps.Valid() {
		return 0, ErrInvalidFPS
	}
	if s == "" {
		return 0, invalid(s, "empty")
	}
	if len(s) > maxInputLen {
		return 0, invalid(s[:16]+"...", "too long")
	}
	var num, den *big.Int // the time in seconds, as num/den
	var err error
	switch {
	case strings.Contains(s, ":"):
		num, den, err = parseTimecode(s)
	case strings.HasSuffix(s, "f"):
		return parseFrames(s)
	case strings.HasSuffix(s, "s"):
		num, den, err = parseDecimal(s[:len(s)-1])
	case s[len(s)-1] >= '0' && s[len(s)-1] <= '9':
		return 0, invalid(s, "no unit")
	default:
		return 0, invalid(s, "unknown unit")
	}
	if err != nil {
		return 0, invalid(s, err.Error())
	}
	// frames = seconds * fps = num * fps.Num / (den * fps.Den)
	n := num.Mul(num, big.NewInt(fps.Num))
	d := den.Mul(den, big.NewInt(fps.Den))
	return checkFrame(s, roundDiv(n, d))
}

func checkFrame(s string, frame *big.Int) (int64, error) {
	if !frame.IsInt64() || frame.Int64() > MaxFrame {
		return 0, invalid(s, "too large")
	}
	return frame.Int64(), nil
}

func parseFrames(s string) (int64, error) {
	digits := s[:len(s)-1]
	if strings.HasPrefix(digits, "-") {
		return 0, invalid(s, "negative")
	}
	if !allDigits(digits) {
		return 0, invalid(s, "frames must be a whole number")
	}
	v, _ := new(big.Int).SetString(digits, 10)
	return checkFrame(s, v)
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// parseDecimal reads digits[.digits] as an exact fraction num/den.
func parseDecimal(s string) (num, den *big.Int, err error) {
	if strings.HasPrefix(s, "-") {
		return nil, nil, errors.New("negative")
	}
	whole, frac, hasPoint := strings.Cut(s, ".")
	if !allDigits(whole) || (hasPoint && !allDigits(frac)) {
		return nil, nil, errors.New("not a decimal number")
	}
	num, _ = new(big.Int).SetString(whole+frac, 10)
	den = new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(frac))), nil)
	return num, den, nil
}

// parseTimecode reads HH:MM:SS[.fff] or MM:SS[.fff]; minutes and seconds are below 60.
func parseTimecode(s string) (num, den *big.Int, err error) {
	if strings.HasPrefix(s, "-") {
		return nil, nil, errors.New("negative")
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return nil, nil, errors.New("timecode is HH:MM:SS.mmm or MM:SS.mmm")
	}
	last := len(parts) - 1
	sec, den, err := parseDecimal(parts[last])
	if err != nil {
		return nil, nil, errors.New("timecode seconds must be digits with an optional fraction")
	}
	if sec.Cmp(new(big.Int).Mul(big.NewInt(60), den)) >= 0 {
		return nil, nil, errors.New("seconds must be below 60")
	}
	var whole int64
	for i, p := range parts[:last] {
		if !allDigits(p) {
			return nil, nil, errors.New("timecode fields must be digits")
		}
		if len(p) > 9 {
			return nil, nil, errors.New("too large")
		}
		v, _ := strconv.ParseInt(p, 10, 64)
		if len(parts) == 3 && i == 0 {
			whole += v * 3600
			continue
		}
		if v >= 60 {
			return nil, nil, errors.New("minutes must be below 60")
		}
		whole += v * 60
	}
	num = new(big.Int).Mul(big.NewInt(whole), den)
	num.Add(num, sec)
	return num, den, nil
}
