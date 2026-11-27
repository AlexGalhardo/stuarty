//go:build !linux && !darwin && !windows

package shell

import "os/exec"

// prepareCommand has nothing to do on other operating systems; Stop handles shutdown.
func prepareCommand(*exec.Cmd) {}
