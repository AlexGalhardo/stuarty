package shell

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const envCrashShell = "VUKEM_TEST_CRASH_SHELL"

// TestCrashShellHelper is the shell of TestServerDiesWithACrashedShell: it starts the server, prints
// its address and dies without Stop, as a crash would. It does nothing in a normal run.
func TestCrashShellHelper(t *testing.T) {
	if os.Getenv(envCrashShell) == "" {
		t.Skip("helper of TestServerDiesWithACrashedShell")
	}
	s, err := Start(context.Background(), newConfig(t, "ok"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = os.Stdout.WriteString(s.Addr + "\n")
	os.Exit(3)
}

// The server must not outlive a shell that dies without stopping it (VV-006).
func TestServerDiesWithACrashedShell(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashShellHelper$")
	cmd.Env = append(os.Environ(), envCrashShell+"=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	addr, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("the helper shell did not print the address: %v", err)
	}
	addr = strings.TrimSpace(addr)
	_ = cmd.Wait()

	client := &http.Client{Timeout: time.Second} //nolint:forbidigo // loopback only
	deadline := time.Now().Add(15 * time.Second)
	for up(context.Background(), client, "http://"+addr+"/up") {
		if time.Now().After(deadline) {
			t.Fatalf("the server at %s survived the crash of the shell", addr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
