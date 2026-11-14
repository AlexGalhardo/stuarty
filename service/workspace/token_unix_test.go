//go:build !windows

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func loosen(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTokenSymlinkRefused(t *testing.T) {
	t.Parallel()
	d := newDir(t)
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, d.Path("token")); err != nil {
		t.Skip("cannot create symlinks here")
	}
	if _, err := d.LoadToken(); !errors.Is(err, ErrTokenPermissions) {
		t.Fatalf("LoadToken through a symlink = %v, want ErrTokenPermissions", err)
	}
}
