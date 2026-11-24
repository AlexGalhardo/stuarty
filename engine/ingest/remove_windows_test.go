package ingest

import (
	"io/fs"
	"syscall"
	"testing"
)

// VV-387: only the three answers of a file held for a moment are waited for; a missing path, a disk that is full or
// a bad name is returned at once, so the wait cannot hide them.
func TestTransientRemoveErrorOnWindowsIsTheThreeCodes(t *testing.T) {
	wrap := func(code uintptr) error {
		return &fs.PathError{Op: "unlinkat", Path: `C:\x`, Err: syscall.Errno(code)}
	}

	for _, code := range []uintptr{5, 32, 145} {
		if !transientRemoveError(wrap(code)) {
			t.Errorf("code %d is transient", code)
		}
	}

	for _, code := range []uintptr{2, 3, 112, 123} {
		if transientRemoveError(wrap(code)) {
			t.Errorf("code %d is not transient", code)
		}
	}
}
