package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// buildBinary compiles the real binary once for the test.
func buildBinary(t *testing.T) string {
	t.Helper()

	name := "vukem-video"
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

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "VUKEM_VIDEO_HOME="+home, "VUKEM_VIDEO_URL=", "APP_KEY=", "APP_ENV=")

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
