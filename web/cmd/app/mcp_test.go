package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// SDD-003 section 4.7: a stdio MCP server with no owner becomes the owner. It
// serves the API on a loopback port and writes runtime.json, so the CLI and
// the browser (open) reach the project the agent is editing, and the file goes
// away when the client closes the pipe.
func TestStdioOwnerIsReachedByTheCLIAndOpen(t *testing.T) {
	bin := buildBinary(t)
	home := filepath.Join(t.TempDir(), "ws")

	cmd := exec.CommandContext(t.Context(), bin, "mcp")
	cmd.Env = append(os.Environ(), mcpTestEnv(t, home)...)

	client := sdk.NewClient(&sdk.Implementation{Name: "Claude Code", Version: "1"}, nil)

	cs, err := client.Connect(t.Context(), &sdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}

	closed := false

	defer func() {
		if !closed {
			_ = cs.Close()
		}
	}()

	created, err := cs.CallTool(
		t.Context(),
		&sdk.CallToolParams{Name: "project_create", Arguments: map[string]any{"name": "Agent project"}},
	)
	if err != nil || created.IsError {
		t.Fatalf("project_create: %v %+v", err, created)
	}

	sc, _ := created.StructuredContent.(map[string]any)
	id, _ := sc["project_id"].(string)

	dir := workspace.Dir(home)

	rt, err := dir.ReadRuntime()
	if err != nil || rt.PID != cmd.Process.Pid || rt.Mode != workspace.ModeLocal {
		t.Fatalf("runtime.json = %+v, %v; want the pid %d of the MCP process", rt, err, cmd.Process.Pid)
	}

	resp, err := outbound.LocalServer(5 * time.Second).Get("http://" + rt.Addr + "/up")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("/up on the stdio owner: %v", err)
	}

	_ = resp.Body.Close()

	// The CLI finds the owner and reads, over HTTP, what the agent made.
	conn := resolve(t, home)
	if conn.InProcess {
		t.Fatal("the CLI took the workspace from the stdio owner instead of using HTTP")
	}

	ctx := service.WithCaller(t.Context(), service.Caller{Actor: "cli", Scopes: service.AllScopes})

	got, err := conn.Service.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: id})
	if err != nil || got.Name != "Agent project" {
		t.Errorf("the CLI does not see the project of the agent: %+v, %v", got, err)
	}

	if conn.Close != nil {
		_ = conn.Close()
	}

	// A second owner is refused while the pipe is open.
	if stderr, code := serveOnce(t, bin, home); code != exitLocked {
		t.Errorf("serve next to the stdio owner: exit %d, %q", code, stderr)
	}

	closed = true

	_ = cs.Close()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := dir.ReadRuntime(); err != nil {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Error("runtime.json survived the close of the pipe")
}

func TestFreePortIfBusyMovesOnlyWhenTheDefaultIsTakenAndUnset(t *testing.T) {
	t.Parallel()

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = taken.Close() }()

	busy := taken.Addr().String()
	none := func(string) string { return "" }

	if got := freePortIfBusy(t.Context(), none, busy, listenProbe); got != "127.0.0.1:0" {
		t.Errorf("a busy default = %q, want any free port", got)
	}

	chosen := func(k string) string {
		if k == envAddr {
			return "127.0.0.1:9999"
		}

		return ""
	}
	if got := freePortIfBusy(t.Context(), chosen, busy, listenProbe); got != "" {
		t.Errorf("an address the person chose = %q, want it kept", got)
	}

	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := free.Addr().String()
	_ = free.Close()

	if got := freePortIfBusy(t.Context(), none, addr, listenProbe); got != "" {
		t.Errorf("a free default = %q, want it kept", got)
	}
}

// VV-294: HOST and PORT choose the address as STUARTY_ADDR does, and then the default port is never probed.
func TestFreePortIfBusyKeepsAnAddressChosenByHostOrPort(t *testing.T) {
	t.Parallel()

	for _, key := range []string{envAddr, "HOST", "PORT"} {
		probed := false
		probe := func(context.Context, string) (net.Listener, error) {
			probed = true

			return nil, errors.New("taken")
		}
		chosen := func(k string) string {
			if k == key {
				return "9000"
			}

			return ""
		}

		if got := freePortIfBusy(t.Context(), chosen, "127.0.0.1:7878", probe); got != "" {
			t.Errorf("%s set: got %q, want the configured address kept", key, got)
		}

		if probed {
			t.Errorf("%s set: the default port was probed", key)
		}
	}
}

// mcpTestEnv is the environment of a `stuarty mcp` process under test: its own
// workspace and its own address. The address goes in STUARTY_ADDR, not in HOST
// and PORT: with STUARTY_ADDR unset the owner probes the default port 7878
// (freePortIfBusy), which may belong to a server the person is running.
func mcpTestEnv(t *testing.T, home string) []string {
	t.Helper()

	return []string{
		"STUARTY_HOME=" + home, "STUARTY_URL=", "APP_KEY=", "APP_ENV=",
		"STUARTY_ADDR=127.0.0.1:" + freePort(t), "HOST=", "PORT=", "STUARTY_MIN_FREE_BYTES=1",
	}
}

// VV-205: a client that writes its requests and closes the pipe at once (a script, a one-line test)
// still gets every response before the process exits, through the real binary.
func TestMCPAnswersTheRequestsReadBeforeStdinCloses(t *testing.T) {
	bin := buildBinary(t)
	home := filepath.Join(t.TempDir(), "ws")

	const reqs = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"

	// A process that never drains fails the test instead of hanging the package.
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "mcp")
	cmd.Env = append(os.Environ(), mcpTestEnv(t, home)...)
	cmd.Stdin = strings.NewReader(reqs)

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("mcp: %v\n%s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"id":1`) || !strings.Contains(out, `"id":2`) || !strings.Contains(out, `"tools"`) {
		t.Errorf("responses missing from stdout: %q\nstderr: %s", out, stderr.String())
	}
}
