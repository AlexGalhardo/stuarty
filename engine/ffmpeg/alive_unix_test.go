//go:build !windows

package ffmpeg

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func isZombie(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err == nil {
		// The state is the field after the parenthesised command name.
		s := string(b)
		if i := strings.LastIndexByte(s, ')'); i >= 0 && i+2 < len(s) {
			return s[i+2] == 'Z'
		}
		return false
	}
	// With procfs mounted, a missing stat file means the process was reaped
	// between the signal probe and this read: report it gone. Falling through
	// to ps would answer "not a zombie" and make a dead process look alive.
	if _, statErr := os.Stat("/proc/self/stat"); statErr == nil {
		return true
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}

// processAlive reports whether a process with this pid is still running. A zombie that
// nobody has reaped yet counts as gone: it holds no resources and cannot run.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	return !isZombie(pid)
}
