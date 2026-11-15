//go:build !windows

package shell

import (
	"os"
	"syscall"
)

// terminate asks the child to exit and flush (SIGTERM).
func terminate(p *os.Process) error { return p.Signal(syscall.SIGTERM) }
