// Package testfs holds the test helpers for folders that hold a workspace.
//
// On Windows a file or a folder that was closed is not always free at once: the system or a scanner keeps a
// handle on it for a short while (the database file for 50 to 140 ms, longer under load). Removing a folder
// that holds one fails with "The directory is not empty" because the entry waits to be deleted. Go's
// TempDir cleanup retries the sharing violations and the access denied errors of this kind for two seconds
// but not that one, so a test whose workspace is in a TempDir failed in its cleanup (VV-385).
package testfs

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// limit is how long the removal of a folder is retried: a folder still held after this is a handle that the
// test or the code leaks, not a scanner.
const limit = 30 * time.Second

// TempDir is tb.TempDir for a folder that holds a workspace. The folder is removed in a cleanup registered
// here, so it runs after the cleanups the test registers later (the closing of the store and of the server).
// The removal is tried again for as long as the system answers that an entry is still held, up to limit, and
// the test fails with that error after it.
func TempDir(tb testing.TB) string {
	tb.Helper()

	// Not tb.TempDir: its own removal does not retry the error this package exists for.
	dir, err := os.MkdirTemp("", folderName(tb.Name())+"-") //nolint:usetesting // see above
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := removeAll(dir); err != nil {
			tb.Errorf("TempDir RemoveAll cleanup: %v", err)
		}
	})

	return dir
}

// folderName is a test name as a folder name: letters, digits, "_" and "-" only, because the workspace path
// goes into a database URL that refuses "?", "#" and "%" (the name of a fuzz seed holds a "#"), and at most
// 40 characters, to keep the path short.
func folderName(name string) string {
	var b strings.Builder

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}

		if b.Len() >= 40 {
			break
		}
	}

	return b.String()
}

// removeAll removes path, trying again while the error is one that an entry still held by a handle gives.
func removeAll(path string) error {
	start := time.Now()
	sleep := time.Millisecond

	for {
		err := os.RemoveAll(path)
		if err == nil || !heldEntry(err) || time.Since(start)+sleep > limit {
			return err
		}

		time.Sleep(sleep)
		sleep = min(sleep*2, 100*time.Millisecond)
	}
}

// heldEntry says whether err is the answer of the system to the removal of an entry that a handle still holds.
func heldEntry(err error) bool {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			break
		}

		err = next
	}

	return isHeld(err)
}
