package owner

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/vault"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

// A backup of an account workspace is made with no password (`workspace:backup` is a process of its own, and the
// server it may run beside is locked): it copies the sealed fields and the sealed vault as they are, so the copy
// holds no clear word of the person and opens only with the password or the recovery key (SDD-007 section 3.9
// item 6). The files of the library are sealed too (ADR-0053), so the scan covers them: a backup copies them as they
// are.
func TestSecurity_T108_ABackupOfALockedWorkspaceHoldsNoClearText(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)

	o, ctx, _ := existingWorkspace(t, getenv)

	defer func() { _ = o.Close() }()

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	if o.Account.State() != account.StateLocked {
		t.Fatalf("the studio after the creation is %s, want locked", o.Account.State())
	}

	out := filepath.Join(t.TempDir(), "backup")

	// What runBackup of `workspace:backup` does, beside the locked server that owns the workspace.
	if _, err := workspace.Backup(ctx, workspace.BackupOptions{
		Dir: workspace.Dir(home), Out: out, Version: "test", Snapshot: store.SnapshotDatabase,
	}); err != nil {
		t.Fatal(err)
	}

	has, err := store.DatabaseHasAccount(ctx, filepath.Join(out, "db.sqlite"))
	if err != nil || !has {
		t.Fatalf("the backup has no account: %v %v", has, err)
	}

	scanned := 0

	err = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}

		scanned++

		for _, w := range canaryWords {
			if bytes.Contains(raw, []byte(w)) {
				t.Errorf("%q is in clear in the backup, in %s", w, p)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if scanned == 0 {
		t.Fatal("the backup holds no file to scan")
	}

	// The vault of the copy is sealed under the key of the account: the key of the machine opens nothing in it.
	if _, err := vault.Open(
		filepath.Join(out, "vault.json"), vault.FileProvider{Path: getenv("STUARTY_MASTER_KEY_FILE")},
	).Get("conn.one"); err == nil {
		t.Error("the vault of the backup opens with the key of the machine")
	}
}
