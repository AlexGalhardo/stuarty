//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach puts the server in its own session, so it outlives the shortcut that started it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
