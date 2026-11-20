package agenthost

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeAgentPath is the fake agent, built once by TestMain.
var fakeAgentPath string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "stuarty-fakeagent-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "agenthost tests:", err)
		return 1
	}

	defer os.RemoveAll(dir)

	fakeAgentPath = filepath.Join(dir, "fakeagent"+exeSuffix())

	build := exec.Command("go", "build", "-o", fakeAgentPath, "./fakeagent")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "agenthost tests: building the fake agent: %v\n%s", err, out)
		return 1
	}

	return m.Run()
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}

	return ""
}

// fakeAgent is the path of the fake agent for a test that needs it.
func fakeAgent(tb testing.TB) string {
	tb.Helper()

	return fakeAgentPath
}
