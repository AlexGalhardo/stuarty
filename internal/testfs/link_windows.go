package testfs

import (
	"context"
	"os/exec"
	"testing"
)

func linkDirFallback(tb testing.TB, link, target string) {
	tb.Helper()

	// Both paths are folders of the test's own temporary directory.
	cmd := exec.CommandContext(context.Background(), "cmd", "/c", "mklink", "/J", link, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		tb.Skipf("neither a symbolic link nor a junction can be made here: %v: %s", err, out)
	}
}
