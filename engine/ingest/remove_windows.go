package ingest

import (
	"errors"
	"syscall"
)

const (
	errorAccessDenied   syscall.Errno = 5
	errorSharingViolate syscall.Errno = 32
	errorDirNotEmpty    syscall.Errno = 145
)

// transientRemoveError is true for the three answers Windows gives while a closed file is still held for a moment by
// a scanner or the indexer: access denied, a sharing violation and a folder that is not empty yet.
func transientRemoveError(err error) bool {
	var errno syscall.Errno

	return errors.As(err, &errno) &&
		(errno == errorAccessDenied || errno == errorSharingViolate || errno == errorDirNotEmpty)
}
