//go:build !linux && !darwin && !windows

package main

import "os"

// No terminal control on this system: the password is read from standard input.
func isTerminal(*os.File) bool { return false }

func withoutEcho(_ *os.File, fn func() error) error { return fn() }
