//go:build windows

package testfs

import (
	"errors"
	"syscall"
)

// isHeld is true for the errors a removal gets while a handle is open on an entry: access denied and a
// sharing violation (which Go's own cleanup retries) and the directory that is not empty because one of its
// entries waits to be deleted (which it does not).
func isHeld(err error) bool {
	for _, held := range []error{syscall.ERROR_ACCESS_DENIED, syscall.Errno(32), syscall.Errno(145)} { // 32: sharing violation, 145: directory not empty
		if errors.Is(err, held) {
			return true
		}
	}

	return false
}
