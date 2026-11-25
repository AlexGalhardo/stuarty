package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// The commands the binary runs itself fail in the format of the others and accept --json (VV-448).
func TestProcessCommandsFailInTheStandardFormat(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "ws")

	for _, tc := range []struct {
		name string
		args []string
		run  func(*testing.T, string, string, ...string) (code int, out, err string)
	}{
		{"user:create", nil, runUserCase},
		{"user:passwd", []string{"--password=x"}, runUserCase},
		{"workspace:backup", nil, runWorkspaceCase},
		{"workspace:restore", []string{"--force", "a"}, runWorkspaceCase},
	} {
		code, out, errText := tc.run(t, home, tc.name, tc.args...)

		if code != 1 || out != "" || !strings.HasPrefix(errText, "error E_VALIDATION: ") ||
			!strings.Contains(errText, "\nhint: usage: stuarty "+tc.name) ||
			!strings.Contains(errText, "retryable: false") {
			t.Errorf("%s %v: exit %d, stdout %q, stderr %q", tc.name, tc.args, code, out, errText)
		}

		code, out, errText = tc.run(t, home, tc.name, append([]string{"--json"}, tc.args...)...)

		var doc struct {
			Code string `json:"code"`
			Hint string `json:"hint"`
		}

		if err := json.Unmarshal([]byte(out), &doc); err != nil || code != 1 || errText != "" ||
			doc.Code != "E_VALIDATION" || !strings.HasPrefix(doc.Hint, "usage: stuarty "+tc.name) {
			t.Errorf("%s --json: exit %d, stdout %q, stderr %q, err %v", tc.name, code, out, errText, err)
		}
	}
}

func TestWorkspaceBackupAcceptsJSON(t *testing.T) {
	t.Parallel()

	key := newKeyFile(t)
	home := seedWorkspace(t, key)

	var r wsRun
	if code := r.run(t, home, key, "workspace:backup", "--json", filepath.Join(t.TempDir(), "bk")); code != 0 {
		t.Fatalf("exit %d: %s", code, r.err.String())
	}

	var doc struct {
		Files int `json:"files"`
	}

	if err := json.Unmarshal(r.out.Bytes(), &doc); err != nil || doc.Files == 0 || r.err.Len() != 0 {
		t.Errorf("stdout %q, stderr %q, err %v", r.out.String(), r.err.String(), err)
	}
}

func TestUserCreateEmptyPasswordIsValidation(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "ws")
	code, out, errText := runUserCase(t, home, "user:create", "ana@example.com")

	if code != 1 || out != "" || !strings.HasPrefix(errText, "error E_VALIDATION: ") ||
		!strings.Contains(errText, "no password was given") {
		t.Errorf("empty password: exit %d, stdout %q, stderr %q", code, out, errText)
	}
}

func runUserCase(t *testing.T, home, name string, args ...string) (int, string, string) {
	t.Helper()

	var r userRun

	code := r.run(t, home, name, "", args...)

	return code, r.out.String(), r.err.String()
}

func runWorkspaceCase(t *testing.T, home, name string, args ...string) (int, string, string) {
	t.Helper()

	var r wsRun

	code := r.run(t, home, "", name, args...)

	return code, r.out.String(), r.err.String()
}
