//go:build !linux

package ffmpeg

import (
	"context"
	"errors"
)

// SandboxBwrap is the value of VUKEM_VIDEO_SANDBOX that wraps every tool in bubblewrap (Linux only).
const SandboxBwrap = "bwrap"

// ResolveSandbox accepts only "": bubblewrap exists only on Linux, so asking for it elsewhere is a
// boot error.
func ResolveSandbox(_ context.Context, mode string) (string, error) {
	if mode == "" {
		return "", nil
	}

	return "", errors.New("ffmpeg: VUKEM_VIDEO_SANDBOX needs Linux with bubblewrap")
}

// command is the tool itself: there is no sandbox outside Linux.
func (r *Runner) command(s execSpec) (string, []string, error) {
	return s.tool.Path(), s.argv, nil
}
