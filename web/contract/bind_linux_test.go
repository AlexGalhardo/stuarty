//go:build linux

package contract

import (
	"os/exec"
	"syscall"
)

// prepareChild asks the kernel to kill the child when the test process dies, so a timeout or a
// kill of `go test` leaves no orphan server (VV-188).
func prepareChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}

func bindChild(*exec.Cmd) error { return nil }

func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }
