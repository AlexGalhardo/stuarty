//go:build windows

package testfs

import (
	"errors"
	"syscall"
)

const (
	errAccessDenied     = syscall.Errno(5)  // ERROR_ACCESS_DENIED: the file is waiting to be deleted
	errSharingViolation = syscall.Errno(32) // ERROR_SHARING_VIOLATION: another handle is open
)

// held says whether another handle is open on path: a share mode of 0 is refused while one is.
func held(path string) bool {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}

	h, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return errors.Is(err, errSharingViolation) || errors.Is(err, errAccessDenied)
	}

	_ = syscall.CloseHandle(h)

	return false
}
