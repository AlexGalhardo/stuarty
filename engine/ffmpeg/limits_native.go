//go:build linux || windows

package ffmpeg

import (
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// whisperHeadroom and whisperModelFactor are the whisper row of SDD-007 6.5: 4 x model size + 2 GiB.
const (
	whisperHeadroom    = 2 << 30
	whisperModelFactor = 4
)

// limitsForWhisper bounds a whisper-cli run by the memory its model needs; the CPU backstop is the
// ceiling of the transcription deadline times the CPU count.
func limitsForWhisper(modelBytes int64, cpus int) engine.Limits {
	return engine.Limits{
		CPUTime:      TranscribeDeadline(24*time.Hour) * time.Duration(max(cpus, 1)),
		AddressSpace: whisperModelFactor*max(modelBytes, 0) + whisperHeadroom,
	}
}
