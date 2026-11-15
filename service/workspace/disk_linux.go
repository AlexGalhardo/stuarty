package workspace

import "golang.org/x/sys/unix"

// FreeBytes is the space a process may still write on the volume of dir
// (SDD-007 section 9.2 item 4).
func FreeBytes(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}

	return st.Bavail * uint64(st.Bsize), nil //nolint:gosec // Bsize is an int64 that is never negative
}
