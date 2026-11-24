// Package testfs holds the test helpers that deal with files the operating system still holds.
//
// On Windows a file that was closed is not always free at once: the system or a scanner may keep a handle
// for a short while, and a directory that holds such a file cannot be removed ("The directory is not
// empty"), which Go's TempDir cleanup does not retry. A test that closes a store ends with Released, so the
// cleanup of its temporary folder starts after the file is free (VV-385).
package testfs

import (
	"testing"
	"time"
)

// limit is how long Released waits for a file before it fails the test: a handle held for longer than this
// is a leak in the test or in the code, not a scanner.
const limit = 30 * time.Second

// Released returns when path can be opened with no other handle on it, or fails the test after limit. A
// missing file is released. It does nothing off Windows, where an open file never keeps its folder.
func Released(tb testing.TB, path string) {
	tb.Helper()

	start := time.Now()

	for held(path) {
		if time.Since(start) > limit {
			tb.Errorf("%s is still held by a handle %v after the store was closed", path, limit)

			return
		}

		time.Sleep(time.Millisecond)
	}
}
