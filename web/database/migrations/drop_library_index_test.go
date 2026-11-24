package migrations

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/vukem/schema"
)

// The index of the library leaves db.sqlite (ADR-0053): Up drops both tables, Down recreates them empty.
func TestDropLibraryIndexDropsTheTablesAndDownBringsThemBackEmpty(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	const id = "20261124150000_drop_library_index"

	var before []schema.Migration

	for _, m := range allMigrations() {
		if m.ID < id {
			before = append(before, m)
		}
	}

	old, err := schema.NewMigrator(db, before)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := old.Up(ctx); err != nil {
		t.Fatal(err)
	}

	execAll(t, db,
		`INSERT INTO library_entries (key, kind, name, description, scope, origin, reviewed, kept, actor, tags, state,
 error, path, sig, lint_errors, lint_warnings, has_scripts, created_ms, updated_ms)
 VALUES ('memory/a', 'memory', 'a', 'd', 'workspace', '', 0, 1, 'u', '[]', 'ok', '', 'memory/a.md', '1', 0, 0, 0, 1, 1)`,
		`INSERT INTO library_fts (key, name, description, tags, body) VALUES ('memory/a', 'a', 'd', '', 'a body')`,
	)

	full, err := schema.NewMigrator(db, allMigrations())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := full.Up(ctx); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"library_entries", "library_fts"} {
		if _, err := db.ExecContext(ctx, `SELECT 1 FROM `+table); err == nil {
			t.Errorf("%s is still in the database", table)
		}
	}

	later := 0

	for _, m := range allMigrations() {
		if m.ID > id {
			later++
		}
	}

	if _, err := full.Rollback(ctx, later+1); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM library_entries`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("Down must recreate library_entries empty: %d %v", n, err)
	}

	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM library_fts`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("Down must recreate library_fts empty: %d %v", n, err)
	}
}
