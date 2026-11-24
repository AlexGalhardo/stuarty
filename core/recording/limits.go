package recording

import (
	"errors"
	"fmt"
)

// The bounds of SDD-025 section 5.
const (
	// MaxSecondsCeiling is the longest a recording may be allowed to run: the import limit, so a recording can
	// always be imported.
	MaxSecondsCeiling = 14400
	// DefaultMaxSeconds is the default longest recording.
	DefaultMaxSeconds = 14400
	// MaxSide and MaxPixels bound a frame (the import limits).
	MaxSide   = 8192
	MaxPixels = 33177600
	// MinSide is the smallest frame side an encoder takes.
	MinSide = 16
)

// StopReason is why the host must stop a recording that is still running.
type StopReason string

// The reasons to stop. None is the empty string.
const (
	StopNone     StopReason = ""
	StopDuration StopReason = "duration"
	StopDisk     StopReason = "disk"
	StopSize     StopReason = "size"
)

// Limits are the three ceilings of a recording.
type Limits struct {
	MaxSeconds   int
	MinFreeBytes int64
	MaxBytes     int64
}

// Validate refuses a limit outside its bounds.
func (l Limits) Validate() error {
	if l.MaxSeconds < 1 || l.MaxSeconds > MaxSecondsCeiling {
		return fmt.Errorf(
			"recording: the longest recording is %d s; it must be 1 to %d",
			l.MaxSeconds,
			MaxSecondsCeiling,
		)
	}

	if l.MinFreeBytes < 0 || l.MaxBytes < 1 {
		return errors.New("recording: the byte limits must be positive")
	}

	return nil
}

// ErrDiskLow means the free space is already under the limit.
var ErrDiskLow = errors.New("recording: the free disk space is under the limit")

// CheckStart refuses a start when the free space is already under the limit.
func (l Limits) CheckStart(freeBytes int64) error {
	if freeBytes < l.MinFreeBytes {
		return fmt.Errorf("%w: %d bytes free, %d needed", ErrDiskLow, freeBytes, l.MinFreeBytes)
	}

	return nil
}

// Check is asked every second while a recording runs. It returns the first limit reached, in the order duration,
// size, disk.
func (l Limits) Check(elapsedMS, bytes, freeBytes int64) StopReason {
	switch {
	case elapsedMS >= int64(l.MaxSeconds)*1000:
		return StopDuration
	case bytes >= l.MaxBytes:
		return StopSize
	case freeBytes < l.MinFreeBytes:
		return StopDisk
	}

	return StopNone
}

// FitSize is the size of the file for a source of w by h: both cut down to even numbers. A source outside the
// bounds of section 5 is an error.
func FitSize(w, h int) (int, int, error) {
	w, h = w&^1, h&^1
	if w < MinSide || h < MinSide || w > MaxSide || h > MaxSide || w*h > MaxPixels {
		return 0, 0, fmt.Errorf("recording: a source of %dx%d is outside the supported size", w, h)
	}

	return w, h, nil
}
