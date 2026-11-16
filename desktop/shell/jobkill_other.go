//go:build !windows

package shell

import "os/exec"

// bindToShell has no counterpart outside Windows yet: the server is stopped by Stop.
func bindToShell(*exec.Cmd) (func(), error) { return func() {}, nil }
