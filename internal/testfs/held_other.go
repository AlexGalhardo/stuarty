//go:build !windows

package testfs

// isHeld is false: an open file never keeps its folder from being removed.
func isHeld(error) bool { return false }
