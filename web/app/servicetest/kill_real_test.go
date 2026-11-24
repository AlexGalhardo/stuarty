//go:build crashtest

package servicetest

import (
	"bufio"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/web/app/store"
)

// Real kills (epic 90, SDD-008 section 13.3). A child is the test binary started again with a role: it works on
// a workspace the parent prepared and talks to a fake network on loopback. The parent ends the child with
// Process.Kill (TerminateProcess on Windows, SIGKILL elsewhere) at a moment it observes, and then checks what
// the workspace and the network hold. No wait here is a sleep that stands in for an event: a wait polls a fact
// until a deadline and fails with what it saw.

const (
	killRealRoleEnv   = "STUARTY_KILL_REAL_ROLE"
	killRealDirEnv    = "STUARTY_KILL_REAL_DIR"
	killRealNetEnv    = "STUARTY_KILL_REAL_NET"
	killRealKeyEnv    = "STUARTY_KILL_REAL_KEY"
	killRealOffsetEnv = "STUARTY_KILL_REAL_OFFSET"
	killRealArgsEnv   = "STUARTY_KILL_REAL_ARGS"

	// killDeadline bounds every wait of a real-kill test.
	killDeadline = 60 * time.Second
)

// realChild is a started child process and what it printed.
type realChild struct {
	cmd  *exec.Cmd
	mu   sync.Mutex
	out  []string
	done chan struct{}
}

// startRealChild starts this test binary as the child of a role, with the given variables added.
func startRealChild(t *testing.T, role string, vars ...string) *realChild {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestKillRealChild$", "-test.v")
	cmd.Env = append(os.Environ(), killRealRoleEnv+"="+role)
	cmd.Env = append(cmd.Env, vars...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	c := &realChild{cmd: cmd, done: make(chan struct{})}

	go func() {
		defer close(c.done)

		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64<<10), 1<<20)

		for sc.Scan() {
			c.mu.Lock()
			c.out = append(c.out, sc.Text())
			c.mu.Unlock()
		}
	}()

	t.Cleanup(func() {
		if c.cmd.ProcessState == nil {
			_ = c.cmd.Process.Kill()
			_ = c.cmd.Wait()
		}
	})

	return c
}

// output is what the child printed so far, for the message of a failed wait.
func (c *realChild) output() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.out) > 40 {
		return strings.Join(c.out[len(c.out)-40:], "\n")
	}

	return strings.Join(c.out, "\n")
}

// line returns the first printed line that starts with prefix, polling until the deadline.
func (c *realChild) line(t *testing.T, prefix string) string {
	t.Helper()

	var found string

	waitFor(t, "the child to print "+prefix, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()

		for _, l := range c.out {
			if strings.HasPrefix(l, prefix) {
				found = l

				return true
			}
		}

		return false
	}, c.output)

	return found
}

// kill ends the child at once and waits for it to be gone.
func (c *realChild) kill(t *testing.T) {
	t.Helper()

	if err := c.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill the child: %v\nchild output:\n%s", err, c.output())
	}

	_ = c.cmd.Wait()
	<-c.done
}

// exited reports whether the child ended by itself.
func (c *realChild) exited() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// waitFor polls cond every few milliseconds until it holds or the deadline passes; on failure the message
// says what the parent saw, through seen.
func waitFor(t *testing.T, what string, cond func() bool, seen func() string) {
	t.Helper()

	deadline := time.NewTimer(killDeadline)
	defer deadline.Stop()

	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()

	for !cond() {
		select {
		case <-deadline.C:
			t.Fatalf("gave up waiting for %s after %s; saw:\n%s", what, killDeadline, seen())
		case <-tick.C:
		}
	}
}

// killVars are the variables a child of a workspace needs.
func killVars(dir, netURL string, key []byte, offset time.Duration, args ...string) []string {
	return []string{
		killRealDirEnv + "=" + dir, killRealNetEnv + "=" + netURL,
		killRealKeyEnv + "=" + hex.EncodeToString(key),
		killRealOffsetEnv + "=" + strconv.FormatInt(int64(offset), 10),
		killRealArgsEnv + "=" + strings.Join(args, ","),
	}
}

// integrityOK opens the database of a workspace as a new process does and checks it.
func integrityOK(t *testing.T, st *store.Store) {
	t.Helper()

	var integrity string
	if err := st.DB().QueryRowContext(context.Background(), "PRAGMA integrity_check").Scan(&integrity); err != nil ||
		integrity != "ok" {
		t.Errorf("integrity_check = %q, %v", integrity, err)
	}
}

// onWindows says whether a kill must also end the processes the child started.
func onWindows() bool { return runtime.GOOS == "windows" }
