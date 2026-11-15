//go:build windows

package ffmpeg

import (
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// LimitsFor returns the limits of a run kind; contrib/engine applies them through a Job Object
// (SDD-007 6.5). AddressSpace becomes ProcessMemoryLimit and CPUTime the per-process user time
// limit, with the numbers of the Linux table. A Job Object has no file size limit and the engine
// refuses one, so the size watchdog alone bounds the output here.
func LimitsFor(kind RunKind, deadline time.Duration, cpus int, _ int64) engine.Limits {
	return limitsFor(kind, deadline, cpus, 0)
}

// LimitsForWhisper returns the limits of a whisper-cli run: the memory of its model.
func LimitsForWhisper(modelBytes int64, cpus int) engine.Limits {
	return limitsForWhisper(modelBytes, cpus)
}

// LimitsAvailable reports true: the Job Object is part of the OS and of contrib/engine.
func LimitsAvailable() (bool, string) { return true, "" }
