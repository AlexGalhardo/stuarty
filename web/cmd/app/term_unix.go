//go:build linux || darwin

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// isTerminal reports whether f is a terminal.
func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), ioctlReadTermios)

	return err == nil
}

// withoutEcho runs fn with the terminal f not echoing what is typed.
func withoutEcho(f *os.File, fn func() error) error {
	fd := int(f.Fd())

	old, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		return err
	}

	quiet := *old
	quiet.Lflag &^= unix.ECHO

	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, &quiet); err != nil {
		return err
	}

	defer func() { _ = unix.IoctlSetTermios(fd, ioctlWriteTermios, old) }()

	return fn()
}
