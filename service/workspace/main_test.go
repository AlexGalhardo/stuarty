package workspace

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const helperEnv = "STUARTY_WORKSPACE_TEST_HELPER"

// TestMain lets the test binary act as the child process of the multi-process
// tests: it takes the lock of the workspace in STUARTY_HOME, prints GOT
// once it holds it and keeps it until its standard input closes.
func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "lock" {
		os.Exit(lockHelper())
	}
	os.Exit(m.Run())
}

func lockHelper() int {
	d := Dir(os.Getenv(EnvHome))
	for {
		l, err := d.TryLock()
		if err == nil {
			fmt.Println("GOT")
			_, _ = io.Copy(io.Discard, os.Stdin)
			_ = l.Release()
			return 0
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type child struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	got   chan struct{}
}

// startLockChild runs a child that loops on TryLock.
func startLockChild(t *testing.T, d Dir) *child {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), helperEnv+"=lock", EnvHome+"="+string(d))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &child{cmd: cmd, stdin: stdin, got: make(chan struct{})}
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) == "GOT" {
				close(c.got)
				return
			}
		}
	}()
	t.Cleanup(c.stop)
	return c
}

func (c *child) stop() {
	_ = c.stdin.Close()
	_ = c.cmd.Process.Kill()
	_, _ = c.cmd.Process.Wait()
}

func (c *child) gotWithin(d time.Duration) bool {
	select {
	case <-c.got:
		return true
	case <-time.After(d):
		return false
	}
}
