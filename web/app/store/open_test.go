package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem/database"
	"github.com/AlexGalhardo/vukem/schema"

	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

func newDir(t *testing.T) workspace.Dir {
	t.Helper()
	d, err := workspace.Resolve(func(string) string { return filepath.Join(t.TempDir(), "ws") })
	if err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	return d
}

func openStore(t *testing.T, d workspace.Dir, o Options) *Store {
	t.Helper()
	s, err := Open(context.Background(), d, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func scalar[T any](t *testing.T, s *Store, query string, args ...any) T {
	t.Helper()
	var v T
	if err := s.db.QueryRowContext(context.Background(), query, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

// SDD-005 section 18 check 2.
func TestOpenSettings(t *testing.T) {
	s := openStore(t, newDir(t), Options{})

	if got := scalar[string](t, s, "PRAGMA journal_mode"); got != "wal" {
		t.Errorf("journal_mode = %q", got)
	}
	if got := scalar[int](t, s, "PRAGMA foreign_keys"); got != 1 {
		t.Errorf("foreign_keys = %d", got)
	}
	if got := scalar[int](t, s, "PRAGMA busy_timeout"); got != 5000 {
		t.Errorf("busy_timeout = %d", got)
	}
	if got := scalar[int](t, s, "PRAGMA synchronous"); got != 1 {
		t.Errorf("synchronous = %d, want 1 (NORMAL)", got)
	}
}

func TestOpenRefusesPathCharacters(t *testing.T) {
	for _, bad := range []string{"a?b", "a#b", "a%b"} {
		d := workspace.Dir(filepath.Join(t.TempDir(), bad))
		// Not created: a ? cannot even be a Windows file name, and the
		// refusal must come before the file system is touched.
		_, err := Open(context.Background(), d, Options{})
		if err == nil || !strings.Contains(err.Error(), "breaks the database URL") {
			t.Errorf("%q: err = %v, want a refusal", bad, err)
		}
		if _, err := os.Stat(d.Path("db.sqlite")); err == nil {
			t.Errorf("%q: the database file must not be created", bad)
		}
	}
}

// A workspace under a path with a space and a non-ASCII letter opens (check 2,
// the Windows case is the one that matters).
func TestOpenUnderSpaceAndNonASCIIPath(t *testing.T) {
	d, err := workspace.Resolve(func(string) string {
		return filepath.Join(t.TempDir(), "Test User", "é", ".vukem-video")
	})
	if err != nil {
		t.Fatal(err)
	}
	s := openStore(t, d, Options{})
	if got := scalar[int](t, s, "SELECT count(*) FROM migrations"); got == 0 {
		t.Error("no migration recorded")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen: the data survives and nothing is pending.
	s2, err := Open(context.Background(), d, Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = s2.Close()
}

// Check 3, first half.
func TestOpenRefusesUnknownMigration(t *testing.T) {
	d := newDir(t)
	s := openStore(t, d, Options{})
	if _, err := s.db.ExecContext(
		context.Background(),
		"INSERT INTO migrations (id, batch, checksum, ran_at) VALUES ('29990101000000_from_the_future', 9, 'x', ?)",
		time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	before := scalar[int](t, s, "SELECT count(*) FROM migrations")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	_, err := Open(context.Background(), d, Options{})
	var newer *NewerWorkspaceError
	if !errors.As(err, &newer) || !strings.Contains(err.Error(), "newer version") ||
		!strings.Contains(err.Error(), "29990101000000_from_the_future") {
		t.Fatalf("err = %v, want the newer version message naming the migration", err)
	}

	// Nothing was written: no backup, the migrations table is as it was.
	if matches, _ := filepath.Glob(d.Path("backups", "*")); len(matches) != 0 {
		t.Errorf("a refused start wrote backups: %v", matches)
	}
	raw := openRaw(t, d)
	if got := scalar[int](t, raw, "SELECT count(*) FROM migrations"); got != before {
		t.Errorf("migrations rows = %d, want %d", got, before)
	}
}

// Check 3, second half.
func TestOpenRefusesNewerSchemaVersion(t *testing.T) {
	d := newDir(t)
	s := openStore(t, d, Options{SchemaVersion: 1})
	if _, err := s.db.ExecContext(context.Background(), `INSERT INTO projects
 (id, name, schema_version, document, doc_sha256, width, height, fps_num, fps_den, created_ms, updated_ms, created_by)
 VALUES ('prj_new', 'n', 2, '{}', 'x', 1, 1, 30, 1, 1, 1, 'local:ui')`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	_, err := Open(context.Background(), d, Options{SchemaVersion: 1})
	var newer *NewerWorkspaceError
	if !errors.As(err, &newer) || !strings.Contains(err.Error(), "schema version 2") ||
		!strings.Contains(err.Error(), "knows 1") {
		t.Fatalf("err = %v, want both numbers", err)
	}
	if s2, err := Open(context.Background(), d, Options{SchemaVersion: 2}); err != nil {
		t.Errorf("a binary that knows version 2 must start: %v", err)
	} else {
		_ = s2.Close()
	}
}

// openRaw opens the database without running the start-up steps, to read
// what a refused start left behind.
func openRaw(t *testing.T, d workspace.Dir) *Store {
	t.Helper()
	db, err := database.Open(context.Background(), "file:"+filepath.ToSlash(d.Path("db.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(db)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// Check 4.
func TestBackupBeforeMigratingAndRetention(t *testing.T) {
	d := newDir(t)
	first := schema.Migration{ID: "20260101000000_a", Up: func(s *schema.Schema) error {
		return s.Raw("CREATE TABLE a (id INTEGER)")
	}}

	s := openStore(t, d, Options{Migrations: []schema.Migration{first}})
	if matches, _ := filepath.Glob(d.Path("backups", "*")); len(matches) != 0 {
		t.Fatalf("a fresh workspace needs no backup: %v", matches)
	}
	if _, err := s.db.ExecContext(context.Background(), "INSERT INTO a VALUES (7)"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Seven later migrations, one open each, one second apart.
	migs := []schema.Migration{first}
	base := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for i := range 7 {
		id := "2026010100000" + string(rune('1'+i)) + "_step"
		migs = append(migs, schema.Migration{ID: id, Up: func(s *schema.Schema) error {
			return s.Raw("CREATE TABLE t" + id[13:14] + " (id INTEGER)")
		}})
		now := base.Add(time.Duration(i) * time.Second)
		st, err := Open(context.Background(), d, Options{Migrations: migs, Now: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
	}

	matches, _ := filepath.Glob(d.Path("backups", "db-*-before-*.sqlite"))
	if len(matches) != BackupsKept {
		t.Fatalf("%d backups, want the %d newest: %v", len(matches), BackupsKept, matches)
	}
	if !strings.Contains(filepath.Base(matches[len(matches)-1]), "20261114T120006-before-20260101000007_step") {
		t.Errorf("newest backup = %s", matches[len(matches)-1])
	}

	// A backup is a valid database holding the data as it was before the migration.
	bak := workspace.Dir(t.TempDir())
	data, err := os.ReadFile(matches[len(matches)-1])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bak.Path("db.sqlite"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	b := openRaw(t, bak)
	if got := scalar[int](t, b, "SELECT id FROM a"); got != 7 {
		t.Errorf("backup holds %d, want 7", got)
	}
	if got := scalar[string](t, b, "PRAGMA integrity_check"); got != "ok" {
		t.Errorf("integrity_check = %q", got)
	}
}
