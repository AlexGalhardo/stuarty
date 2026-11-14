package workspace

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestTokenFormatAndPersistence(t *testing.T) {
	t.Parallel()
	d := newDir(t)

	tok, err := d.EnsureToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 68 || !strings.HasPrefix(tok, "vvl_") || !validToken(tok) {
		t.Errorf("token %q is not vvl_ plus 64 hex characters", tok)
	}
	raw, err := os.ReadFile(d.Path("token"))
	if err != nil || string(raw) != tok+"\n" {
		t.Errorf("token file = %q, %v; want the token and one line feed", raw, err)
	}

	again, err := d.EnsureToken()
	if err != nil || again != tok {
		t.Errorf("second EnsureToken = %q, %v; want the same token", again, err)
	}

	rotated, err := d.RotateToken()
	if err != nil || rotated == tok {
		t.Errorf("RotateToken = %q, %v; want a different token", rotated, err)
	}
	if got, err := d.LoadToken(); err != nil || got != rotated {
		t.Errorf("LoadToken after rotate = %q, %v", got, err)
	}

	entries, _ := os.ReadDir(string(d))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}

func TestLoadTokenRejectsMalformed(t *testing.T) {
	t.Parallel()
	d := newDir(t)
	if _, err := d.EnsureToken(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.Path("token"), []byte("not-a-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.LoadToken(); err == nil {
		t.Fatal("a malformed token must be refused")
	}
}

func TestSecurity_T03_TokenFileModeOwnerOnly(t *testing.T) {
	t.Parallel()
	d := newDir(t)
	if _, err := d.EnsureToken(); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Lstat(d.Path("token"))
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() {
		t.Errorf("token is not a regular file: %v", fi.Mode())
	}
	if runtime.GOOS != "windows" {
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("mode = %04o, want 0600", fi.Mode().Perm())
		}
		root, err := os.Stat(string(d))
		if err != nil {
			t.Fatal(err)
		}
		if root.Mode().Perm()&0o077 != 0 {
			t.Errorf("workspace mode = %04o, want no group or other access", root.Mode().Perm())
		}
	}
	// On Windows the owner-only guarantee is the ACL; the test below checks it.
	if err := checkTokenFile(d.Path("token")); err != nil {
		t.Errorf("a token just created must pass the check: %v", err)
	}
}

func TestSecurity_T03_LooseTokenFileRefusesStart(t *testing.T) {
	t.Parallel()
	d := newDir(t)
	if _, err := d.EnsureToken(); err != nil {
		t.Fatal(err)
	}
	loosen(t, d.Path("token"))

	// The owner start-up path.
	if _, err := d.EnsureToken(); !errors.Is(err, ErrTokenPermissions) {
		t.Fatalf("EnsureToken = %v, want ErrTokenPermissions", err)
	}
	// The client path.
	if _, err := d.LoadToken(); !errors.Is(err, ErrTokenPermissions) {
		t.Fatalf("LoadToken = %v, want ErrTokenPermissions", err)
	}
	// The loose file is not replaced behind the operator's back.
	if _, err := d.EnsureToken(); !errors.Is(err, ErrTokenPermissions) {
		t.Errorf("a loose file was silently replaced: %v", err)
	}
}
