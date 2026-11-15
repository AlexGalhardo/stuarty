//go:build windows

package shell

import "os"

// terminate has no graceful form for a child without a console: Windows has no
// SIGTERM, so the grace period is skipped and the process is killed.
func terminate(p *os.Process) error { return p.Kill() }
