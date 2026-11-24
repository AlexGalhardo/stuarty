package ingest

import (
	"errors"
	"syscall"
)

// The Windows codes of ERROR_ACCESS_DENIED, ERROR_SHARING_VIOLATION and ERROR_DIR_NOT_EMPTY.
const (
	errAccessDenied     syscall.Errno = 5
	errSharingViolation syscall.Errno = 32
	errDirNotEmpty      syscall.Errno = 145
)

// transientRemoveError is true for the three answers Windows gives while a closed file is still held for a moment by
// a scanner or the indexer: access denied, a sharing violation and a folder that is not empty yet.
func transientRemoveError(err error) bool {
	var errno syscall.Errno

	return errors.As(err, &errno) &&
		(errno == errAccessDenied || errno == errSharingViolation || errno == errDirNotEmpty)
}
