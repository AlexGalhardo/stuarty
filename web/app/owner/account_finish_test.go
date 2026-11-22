package owner

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/service/vault"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// clearFiles is everything of the workspace the words of a person must not be in, once the account exists: the
// database, its log, the vault and every copy in backups/. The library files stay in clear (D-127).
func clearFiles(t *testing.T, home string) map[string][]byte {
	t.Helper()

	out := map[string][]byte{}

	for _, n := range []string{"db.sqlite", "db.sqlite-wal", "vault.json"} {
		if raw, err := os.ReadFile(filepath.Join(home, n)); err == nil {
			out[n] = raw
		}
	}

	err := filepath.WalkDir(filepath.Join(home, "backups"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		raw, err := os.ReadFile(p)
		out[p] = raw

		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func assertNoClearWord(t *testing.T, home string) {
	t.Helper()

	for name, raw := range clearFiles(t, home) {
		for _, w := range canaryWords {
			if bytes.Contains(raw, []byte(w)) {
				t.Errorf("%q is in clear in %s", w, name)
			}
		}
	}
}

// plantSchemaBackup leaves in backups/ the copy a schema migration made before the account existed.
func plantSchemaBackup(t *testing.T, o *Owner, home string) string {
	t.Helper()

	checkpoint(t, o)

	raw, err := os.ReadFile(filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(raw, []byte(canaryScript)) {
		t.Fatal("the control does not see the script in the database")
	}

	if err := os.MkdirAll(filepath.Join(home, "backups"), 0o700); err != nil {
		t.Fatal(err)
	}

	name := filepath.Join(home, "backups", "db-20261121T000000-before-20261121200000_create_accounts_table.sqlite")
	if err := os.WriteFile(name, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	return name
}

// No checkpoint between the creation and the scan: what the person's disk holds right after is what counts.
func TestSecurity_T109_NoClearWordIsLeftRightAfterTheCreation(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	ctx := context.Background()

	o, _, _ := existingWorkspace(t, getenv)

	defer func() { _ = o.Close() }()

	backup := plantSchemaBackup(t, o, home)

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	assertNoClearWord(t, home)

	if _, err := os.Stat(backup); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the copy of the schema migration, with every word in clear, is still there: %v", err)
	}
}

// A process killed after the commit and before the cleaning leaves the clear pages and the copies: the next start
// finishes the work, and the account opens.
func TestSecurity_T109_AKillAfterTheCommitIsCleanedAtTheNextStart(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	ctx := context.Background()

	o, as, project := existingWorkspace(t, getenv)
	plantSchemaBackup(t, o, home)

	o.accountFault = func(stage string) error {
		if stage == "finish" {
			return errors.New("killed")
		}

		return nil
	}

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatalf("a failure after the commit must not fail the creation: %v", err)
	}

	if len(backupsLeft(t, home)) != 1 {
		t.Fatal("the copy is the mark of the unfinished work and must still be there")
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := Open(ctx, Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = again.Close() }()

	assertNoClearWord(t, home)

	if left := backupsLeft(t, home); len(left) != 0 {
		t.Errorf("the copy is still there: %v", left)
	}

	if err := again.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	proj, err := again.Service.ProjectGet(as, service.ProjectGetRequest{ProjectID: project})
	if err != nil || proj.Description != canaryDescription {
		t.Fatalf("the project after the restart: %+v %v", proj, err)
	}

	if secret, err := again.vault.Get("conn.one"); err != nil || string(secret) != canaryToken {
		t.Fatalf("the credential after the restart: %q %v", secret, err)
	}
}

// A process killed while it copied the vault leaves a partial file. It must never be taken for the vault.
func TestSecurity_T109_AHalfWrittenCopyIsNeverPutBack(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	keyFile := getenv("STUARTY_MASTER_KEY_FILE")
	ctx := context.Background()

	o, _, _ := existingWorkspace(t, getenv)

	good := scan(t, home, "vault.json")
	left := filepath.Join(home, "backups", accountBackupPrefix+"killed")

	if err := os.MkdirAll(left, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(left, "vault.json.123.tmp"), good[:len(good)/2], 0o600); err != nil {
		t.Fatal(err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := Open(ctx, Config{Getenv: getenv, Inline: true})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = again.Close() }()

	if !bytes.Equal(scan(t, home, "vault.json"), good) {
		t.Error("the vault is not the one it was")
	}

	if secret, err := vault.Open(filepath.Join(home, "vault.json"), vault.FileProvider{Path: keyFile}).
		Get("conn.one"); err != nil || string(secret) != canaryToken {
		t.Errorf("the vault does not open: %q %v", secret, err)
	}

	if rest := backupsLeft(t, home); len(rest) != 0 {
		t.Errorf("the folder of the killed creation is still there: %v", rest)
	}
}

// A vault that is already the copy needs no restore: writing it may be what failed, and a copy left behind holds
// the whole database in clear.
func TestRestoreVaultLeavesAnIdenticalFileAlone(t *testing.T) {
	dir := t.TempDir()
	saved, live := filepath.Join(dir, "saved"), filepath.Join(dir, "vault.json")

	if err := os.WriteFile(saved, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(live, []byte("same"), 0o400); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(live, 0o600) })

	if err := restoreVault(saved, live); err != nil {
		t.Fatalf("a read-only vault with the same content: %v", err)
	}

	if err := os.Chmod(live, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(live, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := restoreVault(saved, live); err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(live); string(got) != "same" {
		t.Errorf("the vault holds %q", got)
	}
}

func TestCopyFileAtomic(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "from"), filepath.Join(dir, "to")

	if had, err := copyFileAtomic(from, to); err != nil || had {
		t.Fatalf("a missing source: %v %v", had, err)
	}

	if err := os.WriteFile(from, []byte("whole"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(to, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if had, err := copyFileAtomic(from, to); err != nil || !had {
		t.Fatalf("the copy: %v %v", had, err)
	}

	if got, _ := os.ReadFile(to); string(got) != "whole" {
		t.Errorf("the destination holds %q", got)
	}

	if names, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(names) != 0 {
		t.Errorf("a temporary file was left: %v", names)
	}
}

// Network mode has no key: it would read a sealed value as text and write clear text over it.
func TestNetworkModeRefusesAWorkspaceWithAnAccount(t *testing.T) {
	getenv := accountEnv(t)
	ctx := context.Background()

	o, _, _ := existingWorkspace(t, getenv)

	defer func() { _ = o.Close() }()

	network := &Owner{mode: config.ModeNetwork}

	if err := network.setupAccount(ctx, o.Store, time.Now); err != nil {
		t.Fatalf("a workspace with no account opens in network mode: %v", err)
	}

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	if err := network.setupAccount(ctx, o.Store, time.Now); !errors.Is(err, ErrAccountInNetworkMode) {
		t.Fatalf("network mode on a workspace with an account: %v", err)
	}
}
