package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveUsesTheEnvironmentAndCreatesTheDirectory(t *testing.T) {
	t.Parallel()

	want := filepath.Join(t.TempDir(), "ws")

	d, err := Resolve(func(k string) string {
		if k == EnvHome {
			return want
		}

		return ""
	})
	if err != nil {
		t.Fatal(err)
	}

	if string(d) != want {
		t.Errorf("Resolve() = %q, want %q", d, want)
	}

	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Errorf("the workspace directory was not created: %v", err)
	}
}

func TestResolveDefaultsToTheHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	d, err := Resolve(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}

	if got, want := filepath.Base(string(d)), ".stuarty"; got != want {
		t.Errorf("Resolve() base = %q, want %q", got, want)
	}
}

func TestResolveRefusesPathsThatBreakTheDSN(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"ws?x", "ws#x", "ws%x"} {
		_, err := Resolve(func(string) string { return filepath.Join(t.TempDir(), bad) })
		if err == nil || !strings.Contains(err.Error(), EnvHome) {
			t.Errorf("Resolve(%q) = %v, want a refusal naming %s", bad, err, EnvHome)
		}
	}
}

func TestEnsureLayoutCreatesEverySubdirectory(t *testing.T) {
	t.Parallel()

	d := Dir(t.TempDir())
	if err := d.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	for _, name := range Subdirectories {
		if info, err := os.Stat(d.Path(name)); err != nil || !info.IsDir() {
			t.Errorf("%s was not created: %v", name, err)
		}
	}
}

func TestErrorTextAndUnwrap(t *testing.T) {
	t.Parallel()

	plain := &Error{Code: CodeUnauthorized, Message: "no"}
	if got := plain.Error(); got != "E_UNAUTHORIZED: no" {
		t.Errorf("Error() = %q", got)
	}

	if err := plain.Unwrap(); err != nil {
		t.Errorf("Unwrap() = %v, want nil", err)
	}

	locked := Dir(t.TempDir()).LockedError()

	var e *Error
	if !errors.As(locked, &e) || e.Code != CodeWorkspaceLocked || !e.Retryable || !errors.Is(locked, ErrLocked) {
		t.Fatalf("LockedError() = %v", locked)
	}

	if !strings.Contains(locked.Error(), "another process owns this workspace") ||
		!strings.Contains(locked.Error(), ErrLocked.Error()) {
		t.Errorf("Error() = %q", locked.Error())
	}
}

func TestRotateTokenReplacesTheToken(t *testing.T) {
	t.Parallel()

	d := Dir(t.TempDir())

	first, err := d.RotateToken()
	if err != nil {
		t.Fatal(err)
	}

	second, err := d.RotateToken()
	if err != nil {
		t.Fatal(err)
	}

	if first == second || !validToken(second) {
		t.Errorf("tokens %q then %q", first, second)
	}

	raw, err := os.ReadFile(d.Path("token"))
	if err != nil || strings.TrimSpace(string(raw)) != second {
		t.Errorf("token file = %q, %v; want %q", raw, err, second)
	}

	if leftovers, _ := filepath.Glob(d.Path("token-*.tmp")); len(leftovers) != 0 {
		t.Errorf("temporary token files left: %v", leftovers)
	}
}
