//go:build unix

package ffmpeg

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own process group, so cancelling a run reaches the
// processes it spawned.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup signals the whole group; the negative pid is what makes it the group. A
// failure falls back to the process, because a group that is already gone is the normal case.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}
