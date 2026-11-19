//go:build unix

package agenthost

import (
	"os/exec"
	"syscall"
)

// treeKiller ends every process of a session.
type treeKiller interface {
	Kill()
	Close()
}

// prepare puts the child in a process group of its own, so that the whole tree can be killed.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

type groupKiller struct{ pid int }

// attach is called after the start; on Unix the group exists already.
func attach(cmd *exec.Cmd) (treeKiller, error) { return groupKiller{pid: cmd.Process.Pid}, nil }

// Kill signals the whole group; the negative pid is what makes it the group. A group that is
// already gone is the normal case.
func (g groupKiller) Kill() { _ = syscall.Kill(-g.pid, syscall.SIGKILL) }

func (groupKiller) Close() {}
