//go:build !linux && !darwin

package shell

import "os/exec"

// prepareCommand has nothing to do here: Windows ties the child to the shell with a job object
// (bindToShell), and other systems rely on Stop.
func prepareCommand(*exec.Cmd) {}
