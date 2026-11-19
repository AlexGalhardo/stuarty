//go:build !windows

package workspace

import (
	"errors"
	"syscall"
)

// isCrossDevice reports that a rename failed because it crosses volumes.
func isCrossDevice(err error) bool { return errors.Is(err, errCrossDevice) }

// errCrossDevice is the error a rename across volumes returns on this system.
var errCrossDevice error = syscall.EXDEV
