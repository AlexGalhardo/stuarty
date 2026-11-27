//go:build windows

package shell

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// prepareCommand starts the local server without allocating or showing a console
// window on Windows, while keeping the job object in bindToShell responsible for
// tearing the process tree down when the desktop shell exits (epic 100).
func prepareCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
