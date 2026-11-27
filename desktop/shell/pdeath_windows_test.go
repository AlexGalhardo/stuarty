//go:build windows

package shell

import (
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrepareCommandHidesWindowsConsole(t *testing.T) {
	cmd := exec.Command("stuarty.exe", "serve")
	prepareCommand(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil, want HideWindow and CREATE_NO_WINDOW")
	}

	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow = false, want true")
	}

	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Errorf("CreationFlags = %#x, want CREATE_NO_WINDOW (%#x) set",
			cmd.SysProcAttr.CreationFlags, windows.CREATE_NO_WINDOW)
	}

	// Preserves pre-existing flags when SysProcAttr is already populated.
	existing := exec.Command("stuarty.exe", "serve")
	existing.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	prepareCommand(existing)

	if existing.SysProcAttr.CreationFlags&windows.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Error("prepareCommand cleared existing CREATE_NEW_PROCESS_GROUP flag")
	}

	if existing.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 || !existing.SysProcAttr.HideWindow {
		t.Errorf("existing SysProcAttr = %+v, want HideWindow and CREATE_NO_WINDOW", existing.SysProcAttr)
	}
}
