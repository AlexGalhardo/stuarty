package store_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/store"

	"github.com/AlexGalhardo/vukem/database"
)

func tempWorkspace(t *testing.T) workspace.Dir {
	t.Helper()

	dir, err := workspace.Resolve(func(string) string { return filepath.Join(t.TempDir(), "ws") })
	if err != nil {
		t.Fatal(err)
	}
	if err := dir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func schemaSnapshot(t *testing.T, db *database.DB) []string {
	t.Helper()

	rows, err := db.QueryContext(
		context.Background(),
		"SELECT type || ':' || name || ':' || COALESCE(sql, '') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOpenTemplateMatchesFreshSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	opts := store.Options{SchemaVersion: timeline.CurrentSchemaVersion}

	fresh, err := store.Open(ctx, tempWorkspace(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })

	tmpl, err := store.OpenTemplate(ctx, tempWorkspace(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tmpl.Close() })

	want := schemaSnapshot(t, fresh.DB())
	got := schemaSnapshot(t, tmpl.DB())
	if !slices.Equal(want, got) {
		t.Fatalf("template schema differs from fresh Open: got %d objects, want %d", len(got), len(want))
	}
}

func TestOpenTemplateIsolatesInstances(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	opts := store.Options{SchemaVersion: timeline.CurrentSchemaVersion}

	a, err := store.OpenTemplate(ctx, tempWorkspace(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })

	b, err := store.OpenTemplate(ctx, tempWorkspace(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	if _, err := a.DB().
		ExecContext(ctx, "CREATE TABLE _iso_check (k TEXT PRIMARY KEY); INSERT INTO _iso_check (k) VALUES ('v1')"); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := b.DB().
		QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name = '_iso_check'").
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("second template store saw table created in first store: count=%d", count)
	}
}
