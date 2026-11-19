package workspace

import (
	"errors"
	"syscall"
)

// errNotSameDevice is ERROR_NOT_SAME_DEVICE: MoveFile cannot move a folder to another volume.
const errNotSameDevice syscall.Errno = 17

// isCrossDevice reports that a rename failed because it crosses volumes.
func isCrossDevice(err error) bool { return errors.Is(err, errCrossDevice) }

// errCrossDevice is the error a rename across volumes returns on this system.
var errCrossDevice error = errNotSameDevice
