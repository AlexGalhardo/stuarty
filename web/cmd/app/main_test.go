package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// buildBinary compiles the real binary once for the test.
func buildBinary(t *testing.T) string {
	t.Helper()

	name := "stuarty"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	bin := filepath.Join(t.TempDir(), name)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if out, err := exec.CommandContext(ctx, "go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	return bin
}

// runBinary runs the binary with a clean environment but for the workspace and
// returns standard output, standard error and the exit code.
func runBinary(t *testing.T, bin, home string, args ...string) (stdout, stderr string, code int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cmd := stuartyCmd(ctx, t, bin, addrEnv(t),
		[]string{"STUARTY_HOME=" + home, "STUARTY_URL=", "APP_KEY=", "APP_ENV="}, args...)

	var out, errOut bytes.Buffer

	cmd.Stdout, cmd.Stderr = &out, &errOut

	err := cmd.Run()

	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}

	return out.String(), errOut.String(), code
}

// TestProductCommandsThroughTheBinary checks the wiring of SDD-003 section 5.1:
// the commands run without APP_KEY, --json is one document with nothing on
// standard error, the exit code follows the error, and no arguments list the
// commands instead of starting a server.
func TestProductCommandsThroughTheBinary(t *testing.T) {
	bin := buildBinary(t)
	home := filepath.Join(t.TempDir(), "ws")

	stdout, stderr, code := runBinary(t, bin, home)
	if code != 0 || !strings.Contains(stdout, "project:create") || stderr != "" {
		t.Fatalf("no arguments: exit %d, %q, %q", code, stdout, stderr)
	}

	stdout, stderr, code = runBinary(t, bin, home, "project:create", "--name", "Launch teaser", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("project:create: exit %d, stderr %q", code, stderr)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil || strings.Count(stdout, "\n") != 1 {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, stdout)
	}

	if strings.Contains(stdout, "\\u003c") {
		t.Errorf("the untrusted-text markers must not be escaped: %s", stdout)
	}

	id, _ := doc["project_id"].(string)

	stdout, stderr, code = runBinary(t, bin, home, "project:show", id, "--json", "--detail", "full")
	if code != 0 || stderr != "" || !strings.Contains(stdout, id) {
		t.Fatalf("project:show with a flag after the id: exit %d, %q, %q", code, stdout, stderr)
	}

	stdout, stderr, code = runBinary(t, bin, home, "project:show", "prj_unknown0001", "--json")
	if code != 1 || stderr != "" || !strings.Contains(stdout, "E_NOT_FOUND") {
		t.Fatalf("an unknown project: exit %d, %q, %q", code, stdout, stderr)
	}

	stdout, stderr, code = runBinary(t, bin, home, "project:list", "--nope")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "error E_VALIDATION") {
		t.Fatalf("a bad flag in human mode: exit %d, %q, %q", code, stdout, stderr)
	}
}

// TestUnknownCommandAndFlagsAnswerInTheProductFormat checks VV-078: a name nobody knows, and a flag a
// process command does not take, are E_UNKNOWN_COMMAND and E_VALIDATION documents (no APP_KEY complaint, no
// usage line), with --json on standard output.
func TestUnknownCommandAndFlagsAnswerInTheProductFormat(t *testing.T) {
	bin := buildBinary(t)
	home := filepath.Join(t.TempDir(), "ws")

	stdout, stderr, code := runBinary(t, bin, home, "projetos", "--json")
	if code != 1 || stderr != "" || !strings.Contains(stdout, "E_UNKNOWN_COMMAND") ||
		!strings.Contains(stdout, "project:list") && !strings.Contains(stdout, "help") {
		t.Fatalf("an unknown command: exit %d, %q, %q", code, stdout, stderr)
	}

	if _, stderr, code = runBinary(t, bin, home, "projetos"); code != 1 || strings.Contains(stderr, "APP_KEY") ||
		!strings.Contains(stderr, "E_UNKNOWN_COMMAND") {
		t.Fatalf("an unknown command in text: exit %d, %q", code, stderr)
	}

	for _, name := range []string{"open", "serve"} {
		stdout, stderr, code = runBinary(t, bin, home, name, "--no-such-flag", "--json")
		if code != 1 || stderr != "" || !strings.Contains(stdout, "E_VALIDATION") ||
			!strings.Contains(stdout, "--no-such-flag") {
			t.Fatalf("%s with a bad flag: exit %d, %q, %q", name, code, stdout, stderr)
		}
	}
}

// TestHelpThroughTheBinaryIsTheProductHelp checks VV-029: the root help, help
// <command> and serve --help come from the product table, need no APP_KEY and
// start nothing.
func TestHelpThroughTheBinaryIsTheProductHelp(t *testing.T) {
	bin := buildBinary(t)
	home := filepath.Join(t.TempDir(), "ws")

	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
		stdout, stderr, code := runBinary(t, bin, home, args...)
		listsProduct := strings.Contains(stdout, "project:list") && !strings.Contains(stdout, "migrate")
		if code != 0 || stderr != "" || !listsProduct {
			t.Fatalf("%v: exit %d, %q, %q", args, code, stdout, stderr)
		}
	}

	for _, name := range []string{"project:list", "serve"} {
		viaHelp, stderr, code := runBinary(t, bin, home, "help", name)
		if code != 0 || stderr != "" || !strings.Contains(viaHelp, "usage:") {
			t.Fatalf("help %s: exit %d, %q, %q", name, code, viaHelp, stderr)
		}

		viaFlag, _, _ := runBinary(t, bin, home, name, "--help")
		if viaFlag != viaHelp {
			t.Errorf("%s --help differs from help %s", name, name)
		}
	}
}

// TestMCPOverStdioThroughTheBinary runs stuarty mcp as a client such as
// Claude Code does: a process on standard input and output. The client name
// becomes the actor, standard output carries only protocol messages, and the
// server stops when the client closes the pipe.
func TestMCPOverStdioThroughTheBinary(t *testing.T) {
	bin := buildBinary(t)
	home := filepath.Join(t.TempDir(), "ws")

	cmd := stuartyCmd(t.Context(), t, bin, nil, mcpTestEnv(t, home), "mcp")

	client := sdk.NewClient(&sdk.Implementation{Name: "Claude Code", Version: "1"}, nil)

	cs, err := client.Connect(t.Context(), &sdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = cs.Close() }()

	list, err := cs.ListTools(t.Context(), nil)
	if err != nil || len(list.Tools) == 0 || list.Tools[0].Name != "project_list" {
		t.Fatalf("tools = %v, err = %v", list, err)
	}

	created, err := cs.CallTool(
		t.Context(),
		&sdk.CallToolParams{Name: "project_create", Arguments: map[string]any{"name": "Over stdio"}},
	)
	if err != nil || created.IsError {
		t.Fatalf("project_create: %v %+v", err, created)
	}

	sc, _ := created.StructuredContent.(map[string]any)
	id, _ := sc["project_id"].(string)

	edited, err := cs.CallTool(t.Context(), &sdk.CallToolParams{Name: "edit", Arguments: map[string]any{
		"project_id": id, "base_revision": 0,
		"operations": []any{map[string]any{"op": "track.add", "args": map[string]any{"kind": "visual"}}},
	}})
	if err != nil || edited.IsError {
		t.Fatalf("edit: %v %+v", err, edited)
	}

	if es, _ := edited.StructuredContent.(map[string]any); !regexp.MustCompile(`^local:claude-code-[0-9a-z]{6}$`).
		MatchString(fmt.Sprint(es["actor"])) {
		t.Fatalf("actor = %v", es["actor"])
	}
}
