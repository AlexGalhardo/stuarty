//go:build !windows

package testfs

func held(string) bool { return false }
