//go:build !linux

package ffmpeg

import (
	"context"
	"errors"
	"io"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// SandboxBwrap is the value of STUARTY_SANDBOX that wraps every tool in bubblewrap (Linux only).
const SandboxBwrap = "bwrap"

// ResolveSandbox accepts only "": bubblewrap exists only on Linux, so asking for it elsewhere is a
// boot error.
func ResolveSandbox(_ context.Context, mode string) (string, error) {
	if mode == "" {
		return "", nil
	}

	return "", errors.New("ffmpeg: STUARTY_SANDBOX needs Linux with bubblewrap")
}

// runSandboxed is never called: ResolveSandbox accepts no mode outside Linux, so Runner.Sandbox is empty.
func (r *Runner) runSandboxed(context.Context, execSpec, io.Writer) (engine.Result, error) {
	return engine.Result{}, errors.New("ffmpeg: there is no sandbox outside Linux")
}
