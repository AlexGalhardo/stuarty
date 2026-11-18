package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

func TestTokenRotateReplacesTheTokenAndNeverPrintsIt(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "ws")
	h := newHarness(t)
	h.rt.Getenv = func(k string) string {
		if k == workspace.EnvHome {
			return home
		}

		return ""
	}

	if code := h.run("token:rotate", "--json"); code != ExitOK {
		t.Fatalf("exit %d: %s %s", code, h.stdout, h.stderr)
	}

	first, err := os.ReadFile(filepath.Join(home, "token"))
	if err != nil {
		t.Fatal(err)
	}

	if code := h.run("token:rotate", "--json"); code != ExitOK {
		t.Fatalf("second rotation: exit %d", code)
	}

	second, _ := os.ReadFile(filepath.Join(home, "token"))
	if len(bytes.TrimSpace(second)) == 0 || bytes.Equal(first, second) {
		t.Errorf("the token did not change: %q then %q", first, second)
	}

	out := h.stdout.String()
	if strings.Contains(out, strings.TrimSpace(string(second))) ||
		strings.Contains(out, strings.TrimSpace(string(first))) {
		t.Errorf("the output carries a token: %s", out)
	}

	var doc tokenRotateDoc
	if err := json.Unmarshal(h.stdout.Bytes(), &doc); err != nil || !doc.Rotated || doc.Text == "" {
		t.Errorf("document = %+v, %v", doc, err)
	}

	if h.stderr.Len() != 0 {
		t.Errorf("standard error in --json mode: %q", h.stderr)
	}
}

func TestTokenRotateRefusesWhileAnOwnerHoldsTheWorkspace(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "ws")
	getenv := func(k string) string {
		if k == workspace.EnvHome {
			return home
		}

		return ""
	}

	held, err := workspace.Open(t.Context(), workspace.Options{Force: workspace.InProcess, Getenv: getenv})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = held.Close() }()

	h := newHarness(t)
	h.rt.Getenv = getenv

	if code := h.run(
		"token:rotate",
		"--json",
	); code == ExitOK ||
		!strings.Contains(h.stdout.String(), "E_WORKSPACE_LOCKED") {
		t.Errorf("exit %d: %s", code, h.stdout)
	}

	if !strings.Contains(h.stdout.String(), "close the desktop app or serve") {
		t.Errorf("the refusal has no hint: %s", h.stdout)
	}
}

func TestListNamesTheCommandsTheBinaryDispatchesItself(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	List(&out)

	for _, want := range []string{"serve", "open", "user:create", "user:passwd", "token:rotate", "agent:docs"} {
		if !strings.Contains(out.String(), "  "+want+" ") {
			t.Errorf("the command list lacks %s:\n%s", want, out.String())
		}
	}
}
