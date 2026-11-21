package owner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// Epic 14 acceptance criterion 5, SDD-017 section 4: closing Stuarty leaves no agent process
// behind. The owner of a server hosts the agent; its Close ends every session and kills every
// process tree, the children the agent started included, before it returns.
func TestClosingTheServerLeavesNoAgentProcess(t *testing.T) {
	dir := t.TempDir()

	agent := filepath.Join(dir, "fakeagent")
	if runtime.GOOS == "windows" {
		agent += ".exe"
	}

	if out, err := exec.CommandContext(context.Background(), "go", "build", "-o", agent,
		"github.com/AlexGalhardo/stuarty/service/agenthost/fakeagent").CombinedOutput(); err != nil {
		t.Fatalf("building the fake agent: %v\n%s", err, out)
	}

	env := map[string]string{workspace.EnvHome: filepath.Join(dir, "ws")}

	// The LGPL build of the machine, when the run names one, rather than whatever is on the path.
	if ff := os.Getenv("STUARTY_TEST_FFMPEG_DIR"); ff != "" {
		exe := ""
		if runtime.GOOS == "windows" {
			exe = ".exe"
		}

		env["STUARTY_FFMPEG"] = filepath.Join(ff, "ffmpeg"+exe)
		env["STUARTY_FFPROBE"] = filepath.Join(ff, "ffprobe"+exe)
	}

	o, err := Open(context.Background(), Config{Getenv: envOf(env)})
	if err != nil {
		t.Fatal(err)
	}

	closed := false

	t.Cleanup(func() {
		if !closed {
			_ = o.Close()
		}
	})

	if o.Agents == nil {
		t.Fatal("a server hosts no agent")
	}

	person := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})

	pidfile := filepath.Join(dir, "grandchild.pid")

	if _, err := o.Service.AgentProfileSet(person, service.AgentProfileSetRequest{
		Name: "spawn", Command: agent, Args: []string{"--mode=spawn", "--pidfile=" + pidfile},
	}); err != nil {
		t.Fatal(err)
	}

	project, err := testproject.CreateWithTimeline(person, o.Service, "Close")
	if err != nil {
		t.Fatal(err)
	}

	session, err := o.Service.ChatSessionStart(person, service.ChatSessionStartRequest{
		ProjectID: project.ProjectID, Profile: "spawn",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The agent starts a child of its own when it gets a message, and names it in the file.
	if _, err := o.Service.ChatSend(person, service.ChatSendRequest{SessionID: session.ID, Text: "go"}); err != nil {
		t.Fatal(err)
	}

	pid := waitForPid(t, pidfile)
	if !alive(pid) {
		t.Fatal("the agent's child is not running before the close")
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	closed = true

	if n := o.Agents.LiveCount(); n != 0 {
		t.Errorf("%d agent sessions are live after the close", n)
	}

	// The tree was killed and waited for before Close returned; the operating system may take a
	// moment to report a killed process as gone.
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()

	limit := time.After(15 * time.Second)

	for alive(pid) {
		select {
		case <-tick.C:
		case <-limit:
			t.Fatalf("the agent's child %d is still running after the close", pid)
		}
	}
}

// waitForPid reads the pid the fake agent wrote, once it is there.
func waitForPid(t *testing.T, path string) int {
	t.Helper()

	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()

	limit := time.After(30 * time.Second)

	for {
		if raw, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(raw)) != "" {
			pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil {
				t.Fatalf("the pid file holds %q", raw)
			}

			return pid
		}

		select {
		case <-tick.C:
		case <-limit:
			t.Fatal("the agent never started its child")
		}
	}
}
