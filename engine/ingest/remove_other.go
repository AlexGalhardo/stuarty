//go:build !windows

package ingest

import (
	"errors"
	"syscall"
)

// transientRemoveError is true for the answers a folder removal can give while a file is still being released:
// access denied and a folder that is not empty yet.
func transientRemoveError(err error) bool {
	return errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.ENOTEMPTY)
}
