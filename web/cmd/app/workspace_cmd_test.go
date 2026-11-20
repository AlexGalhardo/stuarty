package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/vault"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

type wsRun struct{ out, err bytes.Buffer }

func (r *wsRun) run(t *testing.T, home, keyFile, name string, args ...string) int {
	t.Helper()

	return runWorkspace(t.Context(), name, args, workspaceIO{
		Out: &r.out, Err: &r.err,
		Getenv: func(k string) string {
			switch k {
			case workspace.EnvHome:
				return home
			case vault.EnvMasterKeyFile:
				return keyFile
			}

			return ""
		},
	})
}

func newKeyFile(t *testing.T) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), "master.key")
	if err := os.WriteFile(
		p,
		[]byte(vault.FormatKey(bytes.Repeat([]byte{byte(len(p))}, vault.KeySize))+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	return p
}

// seedWorkspace makes a workspace that has been started: a database with a user, an original, a library
// file and a vault with one credential sealed under keyFile.
func seedWorkspace(t *testing.T, keyFile string) string {
	t.Helper()

	home := filepath.Join(t.TempDir(), "ws")

	dir, err := workspace.Resolve(func(string) string { return home })
	if err != nil {
		t.Fatal(err)
	}

	if err := dir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	net, err := auth.NewNetwork(st.DB(), auth.NetworkOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := net.Users.Create(t.Context(), "ana@example.com", userPassword, "admin"); err != nil {
		t.Fatal(err)
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	for rel, body := range map[string]string{
		"store/ab/abcdef": "original media", "library/memory/note.md": "a note", "exports/p/j.mp4": "export",
	} {
		p := dir.Path(filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := vault.Open(dir.Path("vault.json"), vault.FileProvider{Path: keyFile}).
		Put("youtube", []byte("secret")); err != nil {
		t.Fatal(err)
	}

	return home
}

func TestWorkspaceBackupCommandsRoundTripAndKeepTheKeyOut(t *testing.T) {
	key := newKeyFile(t)
	home := seedWorkspace(t, key)
	backup := filepath.Join(t.TempDir(), "backup")

	var r wsRun
	if code := r.run(t, home, key, "workspace:backup", backup); code != 0 {
		t.Fatalf("backup exit %d: %s", code, r.err.String())
	}

	if !strings.Contains(r.out.String(), "master key is NOT in this backup") {
		t.Errorf("the backup does not say that the key is not in it: %q", r.out.String())
	}

	keyBytes, _ := os.ReadFile(key)
	raw := strings.TrimSpace(string(keyBytes))

	_ = filepath.WalkDir(backup, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		if b, _ := os.ReadFile(p); bytes.Contains(b, []byte(raw)) {
			t.Errorf("%s holds the master key", p)
		}

		return nil
	})

	fresh := filepath.Join(t.TempDir(), "fresh")

	var rr wsRun
	if code := rr.run(t, fresh, key, "workspace:restore", backup); code != 0 {
		t.Fatalf("restore exit %d: %s", code, rr.err.String())
	}

	for rel, want := range map[string]string{"store/ab/abcdef": "original media", "library/memory/note.md": "a note"} {
		got, err := os.ReadFile(filepath.Join(fresh, filepath.FromSlash(rel)))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v", rel, got, err)
		}
	}

	if _, err := os.Stat(filepath.Join(fresh, "exports", "p", "j.mp4")); err == nil {
		t.Error("an export was restored")
	}

	if _, err := userByEmail(t, fresh, "ana@example.com"); err != nil {
		t.Errorf("the user is not in the restored database: %v", err)
	}

	// The vault of the restored workspace opens with the same key and holds the credential.
	got, err := vault.Open(filepath.Join(fresh, "vault.json"), vault.FileProvider{Path: key}).Get("youtube")
	if err != nil || string(got) != "secret" {
		t.Errorf("the restored vault: %q, %v", got, err)
	}
}

func TestWorkspaceBackupCommandsRestoreRefusesAMismatchedKey(t *testing.T) {
	key := newKeyFile(t)
	home := seedWorkspace(t, key)
	backup := filepath.Join(t.TempDir(), "backup")

	var r wsRun
	if code := r.run(t, home, key, "workspace:backup", backup); code != 0 {
		t.Fatalf("backup exit %d: %s", code, r.err.String())
	}

	other := filepath.Join(t.TempDir(), "other.key")
	if err := os.WriteFile(
		other,
		[]byte(vault.FormatKey(bytes.Repeat([]byte{7}, vault.KeySize))+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	for name, keyFile := range map[string]string{"another key": other, "no key": ""} {
		fresh := filepath.Join(t.TempDir(), "fresh")

		var rr wsRun
		if code := rr.run(t, fresh, keyFile, "workspace:restore", backup); code == 0 {
			t.Fatalf("%s: the restore succeeded", name)
		}

		msg := rr.err.String()
		if !strings.Contains(msg, "master key") || !strings.Contains(msg, "--without-vault") {
			t.Errorf("%s: the message does not explain the key: %q", name, msg)
		}

		if _, err := os.Stat(filepath.Join(fresh, "db.sqlite")); err == nil {
			t.Errorf("%s: the restore wrote the database before it refused", name)
		}
	}

	fresh := filepath.Join(t.TempDir(), "fresh")

	var rr wsRun
	if code := rr.run(t, fresh, "", "workspace:restore", backup, "--without-vault"); code != 0 {
		t.Fatalf("--without-vault exit %d: %s", code, rr.err.String())
	}

	if _, err := os.Stat(filepath.Join(fresh, "vault.json")); err == nil {
		t.Error("--without-vault restored the vault")
	}
}

func TestWorkspaceBackupCommandsRestoreRefusesAWorkspaceWithData(t *testing.T) {
	key := newKeyFile(t)
	home := seedWorkspace(t, key)
	backup := filepath.Join(t.TempDir(), "backup")

	var r wsRun
	if code := r.run(t, home, key, "workspace:backup", backup); code != 0 {
		t.Fatalf("backup exit %d: %s", code, r.err.String())
	}

	var rr wsRun
	if code := rr.run(
		t,
		home,
		key,
		"workspace:restore",
		backup,
	); code == 0 ||
		!strings.Contains(rr.err.String(), "already has data") {
		t.Errorf("restore into the source workspace: exit %d, %q", code, rr.err.String())
	}

	var rp wsRun
	if code := rp.run(t, home, key, "workspace:restore", backup, "--replace"); code != 0 {
		t.Fatalf("--replace exit %d: %s", code, rp.err.String())
	}

	if !strings.Contains(rp.out.String(), "replaced-") {
		t.Errorf("--replace does not say where the old data is: %q", rp.out.String())
	}
}

func TestWorkspaceBackupCommandsArguments(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
		bad  bool
	}{
		{"workspace:backup", nil, true},
		{"workspace:backup", []string{"a", "b"}, true},
		{"workspace:backup", []string{"--replace", "a"}, true},
		{"workspace:restore", []string{"a", "--replace", "--without-vault"}, false},
		{"workspace:restore", []string{"--force", "a"}, true},
	} {
		_, _, _, err := parseWorkspaceArgs(tc.name, tc.args)
		if (err != nil) != tc.bad {
			t.Errorf("%s %v: err = %v", tc.name, tc.args, err)
		}
	}
}
