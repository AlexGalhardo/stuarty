//go:build linux

package shell

import (
	"os/exec"
	"syscall"
)

// prepareCommand asks the kernel to send SIGTERM to the child when its parent dies, so a crash of
// the shell does not leave an orphan holding the workspace lock (VV-006). Pdeathsig fires when the
// thread that started the child exits, not the process; Start keeps that thread locked and alive
// for as long as the child runs.
func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}
