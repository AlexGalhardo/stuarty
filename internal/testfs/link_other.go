//go:build !windows

package testfs

import "testing"

func linkDirFallback(tb testing.TB, _, _ string) {
	tb.Helper()
	tb.Skip("symbolic links are not available here")
}
