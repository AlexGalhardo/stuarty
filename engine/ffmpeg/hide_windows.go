//go:build windows

package ffmpeg

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// HideWindow configures a child command started through contrib/engine to run
// without creating or displaying a console window on Windows (epic 100, VF-86).
func HideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
