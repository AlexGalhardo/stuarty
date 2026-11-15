//go:build linux

package ffmpeg

import (
	"os/exec"
	"time"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// LimitsFor returns the limits of a run kind; prlimit(1) applies them (SDD-007 6.5).
func LimitsFor(kind RunKind, deadline time.Duration, cpus int, fileBytes int64) engine.Limits {
	return limitsFor(kind, deadline, cpus, fileBytes)
}

// LimitsAvailable reports whether prlimit(1) from util-linux is installed. When it is not, the owner
// process must start with a boot error that names the package, never with children that are
// silently unlimited; doctor reports "unavailable".
func LimitsAvailable() (bool, string) {
	if _, err := exec.LookPath("prlimit"); err != nil {
		return false, "prlimit(1) is missing: install util-linux"
	}

	return true, ""
}
