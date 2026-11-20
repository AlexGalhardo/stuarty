package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/store"

	"github.com/AlexGalhardo/vukem/auth/hash"
)

const userPassword = "c4n4ry correct horse battery"

type userRun struct {
	out, err bytes.Buffer
}

func (r *userRun) run(t *testing.T, home, name, stdin string, args ...string) int {
	t.Helper()

	return runUser(t.Context(), name, args, userIO{
		In: strings.NewReader(stdin), Out: &r.out, Err: &r.err,
		Getenv: func(k string) string {
			if k == workspace.EnvHome {
				return home
			}

			return ""
		},
	})
}

// userByEmail reads a user back through the store of the workspace.
func userByEmail(t *testing.T, home, email string) (auth.User, error) {
	t.Helper()

	dir, err := workspace.Resolve(func(string) string { return home })
	if err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = st.Close() }()

	return auth.NewUsers(st.DB()).ByEmail(t.Context(), email)
}

func TestUserCreateReadsThePasswordFromStandardInput(t *testing.T) {
	home := filepath.Join(t.TempDir(), "ws")

	var r userRun

	if code := r.run(t, home, "user:create", userPassword+"\n", "ana@example.com", "--admin"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.err.String())
	}

	u, err := userByEmail(t, home, "ana@example.com")
	if err != nil || u.Role != "admin" {
		t.Fatalf("user = %+v, %v", u, err)
	}

	if ok, _ := hash.Default().Verify(u.Password, userPassword); !ok {
		t.Error("the stored hash does not verify the password read from standard input")
	}

	if strings.Contains(r.out.String()+r.err.String(), userPassword) {
		t.Error("the password is in the output")
	}

	// A second user without --admin is a member, and a duplicate is refused.
	if code := r.run(t, home, "user:create", userPassword+"\n", "ben@example.com"); code != 0 {
		t.Fatalf("exit %d: %s", code, r.err.String())
	}

	if u, _ := userByEmail(t, home, "ben@example.com"); u.Role != "member" {
		t.Errorf("role = %q, want member", u.Role)
	}

	r = userRun{}
	if code := r.run(t, home, "user:create", userPassword+"\n", "ben@example.com"); code != 1 ||
		!strings.Contains(r.err.String(), "already exists") {
		t.Errorf("a duplicate = exit %d: %s", code, r.err.String())
	}
}

func TestUserCommandsNeverTakeThePasswordFromArgv(t *testing.T) {
	home := filepath.Join(t.TempDir(), "ws")

	for _, args := range [][]string{
		{"ana@example.com", "--password=" + userPassword},
		{"ana@example.com", "-p", userPassword},
		{"ana@example.com", userPassword},
		{},
	} {
		var r userRun
		if code := r.run(t, home, "user:create", userPassword+"\n", args...); code != 1 {
			t.Errorf("%v: exit %d, want a usage error", args, code)
		}

		if strings.Contains(r.err.String(), userPassword) {
			t.Errorf("%v: the usage error repeats the password:\n%s", args, r.err.String())
		}
	}

	if _, err := userByEmail(t, home, "ana@example.com"); err == nil {
		t.Error("a user was created from a refused command line")
	}
}

// With STUARTY_URL set the commands refuse and name the variable instead of creating a workspace in the
// current directory (VV-272).
func TestUserCommandsRefuseARemoteSession(t *testing.T) {
	for _, name := range []string{"user:create", "user:passwd"} {
		cwd := t.TempDir()
		t.Chdir(cwd)

		var r userRun

		code := runUser(t.Context(), name, []string{"ana@example.com"}, userIO{
			In: strings.NewReader(userPassword + "\n"), Out: &r.out, Err: &r.err,
			Getenv: func(k string) string {
				switch k {
				case workspace.EnvURL:
					return "https://stuarty.example.invalid"
				case workspace.EnvKey:
					return "a-key"
				}

				return ""
			},
		})
		if code == 0 || !strings.Contains(r.err.String(), workspace.EnvURL) {
			t.Errorf("%s with %s set: exit %d, %q", name, workspace.EnvURL, code, r.err.String())
		}

		if left, _ := os.ReadDir(cwd); len(left) != 0 {
			t.Errorf("%s wrote into the current directory: %v", name, left)
		}
	}
}

// While a server owns the workspace the commands fail with E_WORKSPACE_LOCKED;
// with none they work (P2-SRV-012 assert 3).
func TestUserCreateWhileAServerOwnsTheWorkspace(t *testing.T) {
	home := filepath.Join(t.TempDir(), "ws")

	dir, err := workspace.Resolve(func(string) string { return home })
	if err != nil {
		t.Fatal(err)
	}

	if err := dir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	lock, err := dir.TryLock()
	if err != nil {
		t.Fatal(err)
	}

	var r userRun
	if code := r.run(t, home, "user:create", userPassword+"\n", "ana@example.com"); code == 0 ||
		!strings.Contains(r.err.String(), "E_WORKSPACE_LOCKED") {
		t.Errorf("exit %d, stderr %q; want E_WORKSPACE_LOCKED", code, r.err.String())
	}

	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}

	r = userRun{}
	if code := r.run(t, home, "user:create", userPassword+"\n", "ana@example.com"); code != 0 {
		t.Errorf("with no server: exit %d, stderr %q", code, r.err.String())
	}
}

func TestUserPasswdChangesThePasswordAndRevokesTheKeys(t *testing.T) {
	home := filepath.Join(t.TempDir(), "ws")

	var r userRun
	if code := r.run(t, home, "user:create", userPassword+"\n", "ana@example.com"); code != 0 {
		t.Fatalf("create: exit %d: %s", code, r.err.String())
	}

	const next = "a second long password"

	if code := r.run(t, home, "user:passwd", next+"\n", "ana@example.com"); code != 0 {
		t.Fatalf("passwd: exit %d: %s", code, r.err.String())
	}

	u, _ := userByEmail(t, home, "ana@example.com")
	if ok, _ := hash.Default().Verify(u.Password, next); !ok {
		t.Error("the new password does not verify")
	}

	if u.Version != 1 {
		t.Errorf("session version = %d, want 1 so that the sessions end", u.Version)
	}

	r = userRun{}
	if code := r.run(t, home, "user:passwd", next+"\n", "nobody@example.com"); code != 1 ||
		!strings.Contains(r.err.String(), "no user") {
		t.Errorf("an unknown user = exit %d: %s", code, r.err.String())
	}
}

func TestReadLineTakesOneLine(t *testing.T) {
	t.Parallel()

	in := strings.NewReader("first\r\nsecond\n")

	if got, err := readLine(in); err != nil || got != "first" {
		t.Errorf("readLine = %q, %v", got, err)
	}

	if got, err := readLine(in); err != nil || got != "second" {
		t.Errorf("readLine = %q, %v", got, err)
	}

	if _, err := readLine(in); err == nil {
		t.Error("an empty input is a missing password")
	}
}
