//go:build windows

package ffmpeg

import (
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestHideWindowSetsFlagsAndPreservesExisting(t *testing.T) {
	cmd := exec.Command("ffmpeg.exe", "-version")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}

	HideWindow(cmd)

	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow = false, want true")
	}

	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Errorf("CreationFlags = %#x, want CREATE_NO_WINDOW set", cmd.SysProcAttr.CreationFlags)
	}

	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Error("HideWindow cleared CREATE_NEW_PROCESS_GROUP")
	}
}
