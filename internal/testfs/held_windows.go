//go:build windows

package testfs

import "syscall"

// isHeld is true for the errors a removal gets while a handle is open on an entry: access denied and a
// sharing violation (which Go's own cleanup retries) and the directory that is not empty because one of its
// entries waits to be deleted (which it does not).
func isHeld(err error) bool {
	switch err {
	case syscall.ERROR_ACCESS_DENIED, syscall.Errno(32), syscall.Errno(145): // sharing violation, dir not empty
		return true
	}

	return false
}
