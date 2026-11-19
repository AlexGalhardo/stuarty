package contract

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const bindHelperEnv = "STUARTY_CONTRACT_BIND_HELPER"

// TestBindHelper is not a test: it is the parent process of TestServerDiesWithTheTestProcess.
// It starts a long-lived child the way the harness starts `serve`, prints its pid and waits.
func TestBindHelper(t *testing.T) {
	if os.Getenv(bindHelperEnv) != "parent" {
		t.Skip("helper of TestServerDiesWithTheTestProcess")
	}

	child := exec.Command(os.Args[0], "-test.run=^TestBindHelperSleeper$", "-test.timeout=2m")
	child.Env = append(os.Environ(), bindHelperEnv+"=sleep")

	if err := startBound(child); err != nil {
		t.Fatal(err)
	}

	os.Stdout.WriteString(
		"CHILD " + strconv.Itoa(child.Process.Pid) + "\n",
	)
	time.Sleep(time.Minute)
}

// startBound starts cmd so that it dies with the test process, whatever kills it.
func startBound(cmd *exec.Cmd) error {
	prepareChild(cmd)

	if err := cmd.Start(); err != nil {
		return err
	}

	if err := bindChild(cmd); err != nil {
		_ = cmd.Process.Kill()

		return err
	}

	return nil
}

func TestBindHelperSleeper(t *testing.T) {
	if os.Getenv(bindHelperEnv) != "sleep" {
		t.Skip("helper of TestServerDiesWithTheTestProcess")
	}

	time.Sleep(time.Minute)
}

// A server started by the harness must die with the test process (VV-188).
func TestServerDiesWithTheTestProcess(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("no kill-on-parent-exit mechanism on " + runtime.GOOS)
	}

	parent := exec.Command(os.Args[0], "-test.run=^(TestBindHelper|TestBindHelperSleeper)$", "-test.timeout=2m")
	parent.Env = append(os.Environ(), bindHelperEnv+"=parent")

	out, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = parent.Process.Kill(); _ = parent.Wait() })

	sc := bufio.NewScanner(out)
	childPID := 0

	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "CHILD "); ok {
			childPID, err = strconv.Atoi(strings.TrimSpace(rest))
			if err != nil {
				t.Fatal(err)
			}

			break
		}
	}

	if childPID == 0 {
		t.Fatal("the helper did not report the child pid")
	}

	if !processAlive(childPID) {
		t.Fatal("the child is not running before the parent is killed")
	}

	if err := parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	_ = parent.Wait()

	deadline := time.Now().Add(10 * time.Second)
	for processAlive(childPID) {
		if time.Now().After(deadline) {
			if p, err := os.FindProcess(childPID); err == nil {
				_ = p.Kill()
			}

			t.Fatalf("child %d outlived its parent", childPID)
		}

		time.Sleep(50 * time.Millisecond)
	}
}
