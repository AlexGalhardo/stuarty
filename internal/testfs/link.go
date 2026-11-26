package testfs

import (
	"os"
	"testing"
)

// LinkDir makes link a folder that leads to target: a symbolic link, or on Windows, where an ordinary account may
// not make one, a junction (which needs no privilege and is followed the same way). The test is skipped when
// neither can be made, so a missing right is never read as a pass or a failure of the code under test.
func LinkDir(tb testing.TB, link, target string) {
	tb.Helper()

	if err := os.Symlink(target, link); err == nil {
		return
	}

	linkDirFallback(tb, link, target)
}
