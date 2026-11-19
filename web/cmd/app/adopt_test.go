package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// former029 is a workspace written by the binary of release v0.29.0 (service/testdata/former-workspace).
const former029 = "../../../service/testdata/former-workspace"

// copyFixture copies the former workspace to dst.
func copyFixture(t *testing.T, dst string) {
	t.Helper()

	err := filepath.WalkDir(former029, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(former029, path)
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(target, b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// runIn runs the binary in a clean environment: only home (HOME and USERPROFILE) and the extra variables.
func runIn(t *testing.T, bin, home string, extra []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)

	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "STUARTY_") || strings.HasPrefix(kv, "VUKEM_VIDEO_") {
			continue
		}

		cmd.Env = append(cmd.Env, kv)
	}

	cmd.Env = append(cmd.Env, "HOME="+home, "USERPROFILE="+home, "APP_KEY=", "APP_ENV=")
	cmd.Env = append(cmd.Env, extra...)

	var out, errOut bytes.Buffer

	cmd.Stdout, cmd.Stderr = &out, &errOut

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run %v: %v", args, err)
		}

		code = exitErr.ExitCode()
	}

	return out.String(), errOut.String(), code
}

// TestFirstStartAdoptsTheWorkspaceOfRelease029 starts the binary on a temporary home that holds only
// ~/.vukem-video written by the release 0.29.0 binary: the workspace is adopted, every project is still
// there, a backup is left, and the person is told once (ADR-0044, PRD-003 RF-221).
func TestFirstStartAdoptsTheWorkspaceOfRelease029(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	former := filepath.Join(home, ".vukem-video")
	current := filepath.Join(home, ".stuarty")

	copyFixture(t, former)

	original, err := os.ReadFile(filepath.Join(former, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runIn(t, bin, home, nil, "project:list", "--json")
	if code != 0 {
		t.Fatalf("project:list exited %d: %s", code, stderr)
	}

	for _, id := range []string{"prj_1000000001", "prj_1000000002", "prj_1000000003", "prj_1000000005"} {
		if !strings.Contains(stdout, id) {
			t.Errorf("project %s is missing after the adoption: %s", id, stdout)
		}
	}

	if n := strings.Count(stderr, "\n"); n != 1 || !strings.Contains(stderr, "workspace of Vukem Video") ||
		!strings.Contains(stderr, "backup") {
		t.Errorf("standard error = %q, want one line that says the workspace was adopted and backed up", stderr)
	}

	if _, err := os.Stat(former); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the former folder is still there after a rename: %v", err)
	}

	backups, err := filepath.Glob(filepath.Join(current, "backups", "adopt-*", "db.sqlite"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, %v, want one copy of the database", backups, err)
	}

	if got, err := os.ReadFile(backups[0]); err != nil || !bytes.Equal(got, original) {
		t.Errorf("the backup is not the database that was adopted (%v)", err)
	}

	if _, err := os.Stat(filepath.Join(current, "adopted.json")); err != nil {
		t.Errorf("the adoption was not recorded: %v", err)
	}

	// The second start finds the adopted workspace: the projects are there and nothing is said.
	again, stderr, code := runIn(t, bin, home, nil, "project:list", "--json")
	if code != 0 || stderr != "" || !strings.Contains(again, "prj_1000000001") {
		t.Errorf("second start: exit %d, %q, %q", code, stderr, again)
	}
}

// TestFormerHomeVariableIsUsedWithOneWarning starts the binary with VUKEM_VIDEO_HOME set and STUARTY_HOME
// unset: it uses that folder and says once that the variable is deprecated (PRD-003 RF-221).
func TestFormerHomeVariableIsUsedWithOneWarning(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	ws := filepath.Join(t.TempDir(), "mine")

	copyFixture(t, ws)

	stdout, stderr, code := runIn(t, bin, home, []string{"VUKEM_VIDEO_HOME=" + ws}, "project:list", "--json")
	if code != 0 || !strings.Contains(stdout, "prj_1000000001") {
		t.Fatalf("exit %d: %s: %s", code, stderr, stdout)
	}

	if n := strings.Count(stderr, "\n"); n != 1 || !strings.Contains(stderr, "VUKEM_VIDEO_HOME is deprecated") ||
		!strings.Contains(stderr, "STUARTY_HOME") {
		t.Errorf("standard error = %q, want one deprecation warning", stderr)
	}

	// Nothing was created in the home: the folder in the variable is the workspace.
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Errorf("the home holds %v (%v), want it empty", entries, err)
	}

	// With both variables set the new one wins, and the old one is reported as ignored.
	other := filepath.Join(t.TempDir(), "other")
	_, stderr, code = runIn(t, bin, home,
		[]string{"VUKEM_VIDEO_HOME=" + ws, "STUARTY_HOME=" + other}, "project:list", "--json")

	if code != 0 || !strings.Contains(stderr, "VUKEM_VIDEO_HOME is ignored") {
		t.Errorf("both set: exit %d, %q", code, stderr)
	}

	if _, err := os.Stat(other); err != nil {
		t.Errorf("STUARTY_HOME was not used: %v", err)
	}
}
