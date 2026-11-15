//go:build !linux && !windows

package ffmpeg

import (
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// LimitsFor returns no limits: macOS has no prlimit(1) and no Job Object, so contrib/engine
// refuses limits there (SDD-007 6.5). Resource control is the preflight ceilings, the timeouts,
// the size watchdog and the concurrency limits.
func LimitsFor(RunKind, time.Duration, int, int64) engine.Limits { return engine.Limits{} }

// LimitsForWhisper returns no limits, for the same reason.
func LimitsForWhisper(int64, int) engine.Limits { return engine.Limits{} }

// LimitsAvailable reports false: there is nothing to apply the limits with.
func LimitsAvailable() (bool, string) {
	return false, "resource limits are not available on this operating system"
}
