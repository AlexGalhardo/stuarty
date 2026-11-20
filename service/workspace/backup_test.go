package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()

	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))

	return err == nil
}

// fakeSnapshot stands for store.SnapshotDatabase: it writes what the database file holds.
func fakeSnapshot(_ context.Context, dbPath, dest string) error {
	raw, err := os.ReadFile(dbPath)
	if err != nil {
		return err
	}

	return os.WriteFile(dest, raw, 0o600)
}

// seeded makes a workspace with data of every kind, and files a backup must leave out.
func seeded(t *testing.T) Dir {
	t.Helper()

	d := Dir(t.TempDir())
	writeFile(t, string(d), "db.sqlite", "the database")
	writeFile(t, string(d), "vault.json", "sealed credentials")
	writeFile(t, string(d), "store/ab/abcdef", "original media")
	writeFile(t, string(d), "store/.incoming/part", "an import in progress")
	writeFile(t, string(d), "library/memory/note.md", "a note")
	writeFile(t, string(d), "library/.review-key", "seal")
	writeFile(t, string(d), "fonts/f1", "a font")
	writeFile(t, string(d), "providers/prices.json", "{}")
	writeFile(t, string(d), "exports/p/j.mp4", "a rendered export")
	writeFile(t, string(d), "artifacts/x/proxy.mp4", "derived")
	writeFile(t, string(d), "token", "local token")
	writeFile(t, string(d), "tmp/job/x", "child process work")

	return d
}

func backupOf(t *testing.T, d Dir) string {
	t.Helper()

	out := filepath.Join(t.TempDir(), "backup")

	_, err := Backup(context.Background(), BackupOptions{
		Dir: d, Out: out, Version: "0.30.0", Snapshot: fakeSnapshot,
		Now: func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func TestWorkspaceBackupHoldsTheDataAndNotTheMasterKey(t *testing.T) {
	t.Parallel()

	d := seeded(t)
	out := backupOf(t, d)

	for _, rel := range []string{
		"db.sqlite", "vault.json", "store/ab/abcdef", "library/memory/note.md", "library/.review-key",
		"fonts/f1", "providers/prices.json", ManifestFile, SumsFile,
	} {
		if !exists(out, rel) {
			t.Errorf("the backup has no %s", rel)
		}
	}

	for _, rel := range []string{"store/.incoming/part", "exports/p/j.mp4", "artifacts/x/proxy.mp4", "token", "tmp/job/x", "lock"} {
		if exists(out, rel) {
			t.Errorf("the backup holds %s, which it must leave out", rel)
		}
	}

	var m Manifest
	if err := json.Unmarshal([]byte(readFile(t, out, ManifestFile)), &m); err != nil {
		t.Fatal(err)
	}

	if m.Format != BackupFormat || m.StuartyVersion != "0.30.0" || m.MasterKeyIncluded {
		t.Errorf("manifest = %+v", m)
	}

	if !strings.Contains(readFile(t, out, SumsFile), "  store/ab/abcdef\n") {
		t.Errorf("SHA256SUMS has no line for the original: %q", readFile(t, out, SumsFile))
	}

	if _, err := VerifyBackup(out); err != nil {
		t.Errorf("a fresh backup does not verify: %v", err)
	}
}

func TestWorkspaceBackupRefusesAFolderThatIsNotEmptyOrIsInsideTheData(t *testing.T) {
	t.Parallel()

	d := seeded(t)
	busy := t.TempDir()
	writeFile(t, busy, "x", "y")

	for _, out := range []string{busy, filepath.Join(string(d), "store", "backup")} {
		_, err := Backup(context.Background(), BackupOptions{Dir: d, Out: out, Snapshot: fakeSnapshot})
		if err == nil {
			t.Errorf("Backup into %s succeeded", out)
		}
	}
}

func TestWorkspaceBackupWithNoDatabaseFails(t *testing.T) {
	t.Parallel()

	_, err := Backup(context.Background(), BackupOptions{
		Dir: Dir(t.TempDir()), Out: filepath.Join(t.TempDir(), "b"), Snapshot: fakeSnapshot,
	})
	if err == nil {
		t.Fatal("a workspace with no database was backed up")
	}
}

func TestWorkspaceBackupRoundTrip(t *testing.T) {
	t.Parallel()

	d := seeded(t)
	out := backupOf(t, d)
	fresh := Dir(t.TempDir())

	res, err := Restore(context.Background(), RestoreOptions{
		Dir: fresh, From: out, VerifyVault: func(string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.ReplacedTo != "" || res.VaultDrop {
		t.Errorf("result = %+v", res)
	}

	for rel, want := range map[string]string{
		"db.sqlite": "the database", "vault.json": "sealed credentials", "store/ab/abcdef": "original media",
		"library/memory/note.md": "a note", "library/.review-key": "seal", "fonts/f1": "a font",
		"providers/prices.json": "{}",
	} {
		if got := readFile(t, string(fresh), rel); got != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}

	if exists(string(fresh), "exports") || exists(string(fresh), "token") {
		t.Error("the restore brought back what the backup leaves out")
	}
}

func TestWorkspaceBackupRestoreRefusesAWrongOrMissingKeyBeforeWriting(t *testing.T) {
	t.Parallel()

	out := backupOf(t, seeded(t))

	for name, verify := range map[string]func(string) error{
		"wrong key":   func(string) error { return errors.New("vault: the master key does not open this vault") },
		"no verifier": nil,
	} {
		fresh := Dir(t.TempDir())

		_, err := Restore(context.Background(), RestoreOptions{Dir: fresh, From: out, VerifyVault: verify})
		if !errors.Is(err, ErrVaultKey) {
			t.Errorf("%s: err = %v, want ErrVaultKey", name, err)
		}

		if left, _ := os.ReadDir(string(fresh)); len(left) != 0 {
			t.Errorf("%s: the restore wrote %d entries before refusing", name, len(left))
		}
	}
}

func TestWorkspaceBackupRestoreWithoutVaultDropsIt(t *testing.T) {
	t.Parallel()

	out := backupOf(t, seeded(t))
	fresh := Dir(t.TempDir())

	res, err := Restore(context.Background(), RestoreOptions{Dir: fresh, From: out, WithoutVault: true})
	if err != nil {
		t.Fatal(err)
	}

	if !res.VaultDrop || exists(string(fresh), "vault.json") || !exists(string(fresh), "db.sqlite") {
		t.Errorf("result = %+v; vault.json exists: %v", res, exists(string(fresh), "vault.json"))
	}
}

func TestWorkspaceBackupRestoreRefusesAWorkspaceWithDataAndReplaceMovesItAside(t *testing.T) {
	t.Parallel()

	out := backupOf(t, seeded(t))
	ws := Dir(t.TempDir())
	writeFile(t, string(ws), "db.sqlite", "newer data")
	writeFile(t, string(ws), "store/zz/zzzz", "newer original")
	ok := func(string) error { return nil }

	_, err := Restore(context.Background(), RestoreOptions{Dir: ws, From: out, VerifyVault: ok})
	if !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("err = %v, want ErrNotEmpty", err)
	}

	if readFile(t, string(ws), "db.sqlite") != "newer data" {
		t.Error("the refusal changed the workspace")
	}

	res, err := Restore(context.Background(), RestoreOptions{
		Dir: ws, From: out, Replace: true, VerifyVault: ok,
		Now: func() time.Time { return time.Date(2026, 11, 19, 13, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}

	if filepath.Base(res.ReplacedTo) != "replaced-20261119T130000Z" {
		t.Errorf("ReplacedTo = %q", res.ReplacedTo)
	}

	if readFile(t, res.ReplacedTo, "db.sqlite") != "newer data" ||
		readFile(t, res.ReplacedTo, "store/zz/zzzz") != "newer original" {
		t.Error("the old data was not kept")
	}

	if readFile(t, string(ws), "db.sqlite") != "the database" || exists(string(ws), "store/zz") {
		t.Error("the workspace does not hold the backup after --replace")
	}
}

func TestWorkspaceBackupRestoreTakesAnEmptyDatabaseFileAsEmpty(t *testing.T) {
	t.Parallel()

	out := backupOf(t, seeded(t))
	ws := Dir(t.TempDir())
	writeFile(t, string(ws), "db.sqlite", "")

	if _, err := Restore(
		context.Background(),
		RestoreOptions{Dir: ws, From: out, VerifyVault: func(string) error { return nil }},
	); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceBackupDetectsTamperingAndStrangers(t *testing.T) {
	t.Parallel()

	t.Run("a changed file", func(t *testing.T) {
		t.Parallel()

		out := backupOf(t, seeded(t))
		writeFile(t, out, "store/ab/abcdef", "original media, altered")

		if _, err := VerifyBackup(out); !errors.Is(err, ErrBackupInvalid) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("a file the manifest does not list", func(t *testing.T) {
		t.Parallel()

		out := backupOf(t, seeded(t))
		writeFile(t, out, "store/ab/extra", "x")

		if _, err := VerifyBackup(out); !errors.Is(err, ErrBackupInvalid) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("a missing file", func(t *testing.T) {
		t.Parallel()

		out := backupOf(t, seeded(t))
		_ = os.Remove(filepath.Join(out, "fonts", "f1"))

		if _, err := VerifyBackup(out); !errors.Is(err, ErrBackupInvalid) {
			t.Errorf("err = %v", err)
		}
	})

	// A crafted archive can carry a symbolic link with the name and the checksum of a listed file: the
	// link would be followed when hashed and hard linked as a link into the workspace.
	t.Run("a listed file that is a symbolic link", func(t *testing.T) {
		t.Parallel()

		out := backupOf(t, seeded(t))
		outside := filepath.Join(t.TempDir(), "elsewhere")
		writeFile(t, filepath.Dir(outside), filepath.Base(outside), "a font")

		_ = os.Remove(filepath.Join(out, "fonts", "f1"))
		if err := os.Symlink(outside, filepath.Join(out, "fonts", "f1")); err != nil {
			t.Skipf( // verify:env-skip creating a symbolic link needs a privilege on Windows
				"this system does not allow a symbolic link here: %v", err)
		}

		if _, err := VerifyBackup(out); !errors.Is(err, ErrBackupInvalid) {
			t.Errorf("err = %v", err)
		}

		fresh := Dir(t.TempDir())
		if _, err := Restore(context.Background(), RestoreOptions{
			Dir: fresh, From: out, VerifyVault: func(string) error { return nil },
		}); !errors.Is(err, ErrBackupInvalid) {
			t.Errorf("restore: err = %v", err)
		}

		if left, _ := os.ReadDir(string(fresh)); len(left) != 0 {
			t.Error("the restore wrote before it refused")
		}
	})

	t.Run("a path outside the trees", func(t *testing.T) {
		t.Parallel()

		for _, bad := range []string{"../escape", "/etc/passwd", "token", "store/../../x", `store\x`} {
			out := t.TempDir()
			m := Manifest{Format: BackupFormat, Version: BackupVersion, Files: []BackupFileEntry{{Path: bad}}}
			raw, _ := json.Marshal(m)
			writeFile(t, out, ManifestFile, string(raw))

			if _, err := ReadManifest(out); !errors.Is(err, ErrBackupInvalid) {
				t.Errorf("%q: err = %v", bad, err)
			}
		}
	})

	t.Run("a manifest that claims the master key", func(t *testing.T) {
		t.Parallel()

		out := t.TempDir()
		writeFile(t, out, ManifestFile, `{"format":"stuarty-backup","version":1,"master_key_included":true,"files":[]}`)

		if _, err := ReadManifest(out); !errors.Is(err, ErrBackupInvalid) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("not a backup", func(t *testing.T) {
		t.Parallel()

		if _, err := VerifyBackup(t.TempDir()); !errors.Is(err, ErrBackupInvalid) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestWorkspaceBackupRestoreRefusesATamperedBackupBeforeWriting(t *testing.T) {
	t.Parallel()

	out := backupOf(t, seeded(t))
	writeFile(t, out, "db.sqlite", "tampered")
	fresh := Dir(t.TempDir())

	_, err := Restore(
		context.Background(),
		RestoreOptions{Dir: fresh, From: out, VerifyVault: func(string) error { return nil }},
	)
	if !errors.Is(err, ErrBackupInvalid) {
		t.Fatalf("err = %v", err)
	}

	if left, _ := os.ReadDir(string(fresh)); len(left) != 0 {
		t.Error("the restore wrote before it refused")
	}
}
