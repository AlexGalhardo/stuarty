//go:build !windows

package workspace

import (
	"fmt"
	"os"
	"syscall"
)

// restrictTokenFile has nothing to add: the file was created with mode 0600.
func restrictTokenFile(string) error { return nil }

// checkTokenFile requires a regular file (not a symlink) owned by the
// effective user with no group or other access (SDD-007 section 3.2).
func checkTokenFile(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrTokenPermissions, path)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Geteuid() {
		return fmt.Errorf("%w: %s is owned by another user", ErrTokenPermissions, path)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: mode is %04o; run: chmod 600 %s", ErrTokenPermissions, fi.Mode().Perm(), path)
	}
	return nil
}
