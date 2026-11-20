//go:build unix

package owner

import "syscall"

// alive reports whether a process with this id is running.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)

	return err == nil || err == syscall.EPERM
}
