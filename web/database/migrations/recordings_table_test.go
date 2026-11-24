package migrations

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem/schema"
)

// The recordings table (SDD-005 section 5.18): an old workspace gains it with its rows untouched, the states and
// reviews are closed sets, and Down leaves the workspace as it was.
func TestRecordingsTableUpAndDown(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	const id = "20261124120000_create_recordings_table"

	var upTo []schema.Migration

	for _, m := range allMigrations() {
		if m.ID < id {
			upTo = append(upTo, m)
		}
	}

	old, err := schema.NewMigrator(db, upTo)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := old.Up(ctx); err != nil {
		t.Fatal(err)
	}

	execAll(
		t,
		db,
		`INSERT INTO projects (id, name, created_ms, updated_ms, created_by) VALUES ('prj_0000000001', 'Old', 1, 1, 'u')`,
	)

	full, err := schema.NewMigrator(db, allMigrations())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := full.Up(ctx); err != nil {
		t.Fatal(err)
	}

	execAll(t, db,
		`INSERT INTO recordings (id, project_id, state, review, source_kind, fps, actor, created_ms)
 VALUES ('rec_0000000001', 'prj_0000000001', 'ready', 'pending', 'display', 30, 'local:ui', 1)`)

	for _, bad := range []string{
		`INSERT INTO recordings (id, project_id, state, source_kind, fps, actor, created_ms)
 VALUES ('rec_0000000002', 'prj_0000000001', 'paused', 'display', 30, 'local:ui', 1)`,
		`INSERT INTO recordings (id, project_id, state, review, source_kind, fps, actor, created_ms)
 VALUES ('rec_0000000003', 'prj_0000000001', 'ready', 'maybe', 'display', 30, 'local:ui', 1)`,
	} {
		if _, err := db.ExecContext(ctx, bad); err == nil {
			t.Errorf("a value outside the closed set was accepted: %s", bad)
		}
	}

	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM projects`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("the old project: %d %v", n, err)
	}

	rolled, err := full.Rollback(ctx, 1)
	if err != nil || len(rolled) != 1 || rolled[0] != id {
		t.Fatalf("Rollback = %v, %v", rolled, err)
	}

	if _, err := db.ExecContext(ctx, `SELECT 1 FROM recordings`); err == nil {
		t.Error("Down must drop the table")
	}
}

// Epic 78 adds 20261124110000_project_kind. The migrator refuses a pending migration that sorts before an applied
// one, so a workspace that ran epic 78 first must still take the recordings table: its id sorts after.
func TestRecordingsTableComesAfterTheProjectKind(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	const projectKind = "20261124110000_project_kind"

	stand := schema.Migration{
		ID:   projectKind,
		Up:   func(*schema.Schema) error { return nil },
		Down: func(*schema.Schema) error { return nil },
	}

	var applied, all []schema.Migration

	for _, m := range allMigrations() {
		if m.ID == projectKind {
			stand = m
			continue
		}

		if m.ID < projectKind {
			applied = append(applied, m)
		}

		all = append(all, m)
	}

	first, err := schema.NewMigrator(db, append(applied, stand))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := first.Up(ctx); err != nil {
		t.Fatal(err)
	}

	all = append(all, stand)
	slices.SortFunc(all, func(a, b schema.Migration) int { return strings.Compare(a.ID, b.ID) })

	later, err := schema.NewMigrator(db, all)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := later.Up(ctx); err != nil {
		t.Fatalf("the recordings table after the project kind: %v", err)
	}

	if _, err := db.ExecContext(ctx, `SELECT 1 FROM recordings`); err != nil {
		t.Errorf("the recordings table is missing: %v", err)
	}
}
