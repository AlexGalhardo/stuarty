//go:build !windows

package shell

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

// startDetached runs the program at execPath in its own session, so it outlives the process that started
// it, and returns a channel closed when it exits. On macOS an application is opened with open(1), which
// returns at once, so the channel is nil: only the confirmation file tells that it started.
func startDetached(execPath string, args ...string) (<-chan struct{}, error) {
	if runtime.GOOS == "darwin" && len(args) == 0 {
		if app := bundleOf(execPath); app != "" {
			if err := exec.Command("/usr/bin/open", "-n", app).Start(); err != nil {
				return nil, fmt.Errorf("open %s: %w", app, err)
			}

			return nil, nil
		}
	}

	cmd := exec.Command(execPath, args...)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", execPath, err)
	}

	done := make(chan struct{})

	go func() {
		_ = cmd.Wait()

		close(done)
	}()

	return done, nil
}
