//go:build !windows

package workspace

import (
	"errors"
	"syscall"
)

// ProcessAlive reports whether a process with this pid exists. A permission
// error means it exists and belongs to someone else.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
