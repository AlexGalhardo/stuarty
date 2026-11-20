//go:build !windows

package main

import "os"

// handleCount is the number of file descriptors the process holds open, or -1 when the system does not say.
func handleCount() int {
	for _, dir := range []string{"/proc/self/fd", "/dev/fd"} {
		if entries, err := os.ReadDir(dir); err == nil {
			return len(entries)
		}
	}

	return -1
}
