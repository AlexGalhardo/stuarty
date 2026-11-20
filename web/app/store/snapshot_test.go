package store

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/AlexGalhardo/vukem/database"
)

// SDD-009 section 21.4: the snapshot is consistent while the server writes, and a project written
// before it starts is in it.
func TestWorkspaceBackupSnapshotWhileWriting(t *testing.T) {
	d := newDir(t)
	s := openStore(t, d, Options{})

	if _, err := s.DB().
		ExecContext(ctx, "CREATE TABLE snap_t (id INTEGER PRIMARY KEY, a INTEGER NOT NULL, b INTEGER NOT NULL)"); err != nil {
		t.Fatal(err)
	}

	// Every row holds a pair of equal values and a writer keeps adding rows while the snapshots are
	// taken: a snapshot that copied a live file half way would be damaged or show a pair that differs.
	insert := func(i int) error {
		_, err := s.DB().ExecContext(ctx, "INSERT INTO snap_t (a, b) VALUES (?, ?)", i, i)

		return err
	}

	for i := range 200 {
		if err := insert(i); err != nil {
			t.Fatal(err)
		}
	}

	stop := make(chan struct{})

	var (
		wg      sync.WaitGroup
		written int
		werr    error
	)

	wg.Go(func() {
		for i := 200; ; i++ {
			select {
			case <-stop:
				return
			default:
			}

			if err := insert(i); err != nil {
				werr = err

				return
			}

			written++
		}
	})

	var snaps []string

	for range 5 {
		dest := filepath.Join(t.TempDir(), "snap.sqlite")
		if err := SnapshotDatabase(ctx, d.Path("db.sqlite"), dest); err != nil {
			close(stop)
			wg.Wait()
			t.Fatal(err)
		}

		snaps = append(snaps, dest)
	}

	close(stop)
	wg.Wait()

	if werr != nil {
		t.Fatalf("the writer failed while the snapshots ran: %v", werr)
	}

	if written == 0 {
		t.Log("the writer made no progress during the snapshots (a very fast machine); the check still holds")
	}

	for _, p := range snaps {
		db, err := database.Open(ctx, "file:"+filepath.ToSlash(p))
		if err != nil {
			t.Fatal(err)
		}

		var check string
		if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&check); err != nil || check != "ok" {
			t.Errorf("integrity_check of the snapshot = %q, %v", check, err)
		}

		var n, differing int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(a <> b), 0) FROM snap_t").
			Scan(&n, &differing); err != nil {
			t.Fatal(err)
		}

		if n < 200 || differing != 0 {
			t.Errorf("snapshot has %d rows (want at least 200), %d with a pair that differs", n, differing)
		}

		_ = db.Close()
	}
}

func TestWorkspaceBackupSnapshotRefusesWhatIsNotAWorkspace(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.sqlite")

	if err := SnapshotDatabase(ctx, filepath.Join(dir, "db.sqlite"), dest); err == nil {
		t.Error("a snapshot of a missing database succeeded")
	}

	if err := os.WriteFile(filepath.Join(dir, "db.sqlite"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SnapshotDatabase(ctx, filepath.Join(dir, "db.sqlite"), dest); err == nil {
		t.Error("a snapshot of an empty database file succeeded")
	}

	if err := os.WriteFile(dest, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	d := newDir(t)
	openStore(t, d, Options{})

	if err := SnapshotDatabase(ctx, d.Path("db.sqlite"), dest); err == nil {
		t.Error("a snapshot over an existing file succeeded")
	}
}
