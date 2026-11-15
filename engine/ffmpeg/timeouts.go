package ffmpeg

import "time"

// Timeouts and limits of SDD-007 6.5 (defaults; the multipliers may change after the
// ALPHA measurements). Every run gets a context deadline from this table, so the 5 minute
// default of contrib/engine never widens it.
const (
	ProbeTimeout     = 30 * time.Second
	FrameTimeout     = 30 * time.Second
	StallTimeout     = 120 * time.Second
	MaxProbeJSON     = 4 << 20
	maxProgressLine  = 4 << 10
	detectionTimeout = 20 * time.Second
	// OutputPollInterval is how often the size watchdog looks at the output file.
	OutputPollInterval = 2 * time.Second
	// CancelGrace is how long a cancelled run may take to end before the job is failed with
	// details.reason "cancel_timeout" (SDD-007 6.6).
	CancelGrace = 5 * time.Second
)

func capped(d, ceiling time.Duration) time.Duration {
	if d > ceiling {
		return ceiling
	}

	return d
}

// IngestDeadline is 60 s + 3 x media duration, at most 4 h.
func IngestDeadline(media time.Duration) time.Duration {
	return capped(60*time.Second+3*media, 4*time.Hour)
}

// ExportDeadline is 120 s + 6 x timeline duration, at most 2 h.
func ExportDeadline(timeline time.Duration) time.Duration {
	return capped(120*time.Second+6*timeline, 2*time.Hour)
}

// AudioExtractDeadline is 60 s + 1 x audio duration, at most 1 h.
func AudioExtractDeadline(audio time.Duration) time.Duration {
	return capped(60*time.Second+audio, time.Hour)
}

// TranscribeDeadline is 120 s + 3 x audio duration, at most 6 h.
func TranscribeDeadline(audio time.Duration) time.Duration {
	return capped(120*time.Second+3*audio, 6*time.Hour)
}
