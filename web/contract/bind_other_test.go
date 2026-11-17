//go:build !windows && !linux

package contract

import "os/exec"

func prepareChild(*exec.Cmd) {}

func bindChild(*exec.Cmd) error { return nil }

func processAlive(int) bool { return false }
