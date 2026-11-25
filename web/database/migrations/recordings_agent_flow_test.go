package migrations

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/vukem/schema"
)

// The plan, the marks and the proposals of a recording (SDD-025 section 19.6): a row of epic 61 to 65 keeps its
// values, is not "prepared" (an empty plan), and Down removes the prepared rows and the three columns.
func TestRecordingsAgentFlowUpAndDown(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	const id = "20261125100000_recordings_agent_flow"

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

	execAll(
		t,
		db,
		`INSERT INTO projects (id, name, created_ms, updated_ms, created_by) VALUES ('prj_0000000001', 'Old', 1, 1, 'u')`,
		`INSERT INTO recordings (id, project_id, state, review, source_kind, fps, duration_ms, actor, created_ms)
 VALUES ('rec_0000000001', 'prj_0000000001', 'ready', 'pending', 'display', 30, 4200, 'local:ui', 1)`,
	)

	var through []schema.Migration

	for _, m := range allMigrations() {
		if m.ID <= id {
			through = append(through, m)
		}
	}

	full, err := schema.NewMigrator(db, through)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := full.Up(ctx); err != nil {
		t.Fatal(err)
	}

	var plan, marks, proposals, state string

	if err := db.QueryRowContext(ctx,
		`SELECT plan, marks, proposals, state FROM recordings WHERE id = 'rec_0000000001'`).
		Scan(&plan, &marks, &proposals, &state); err != nil || plan != "" || marks != "[]" || proposals != "[]" ||
		state != "ready" {
		t.Fatalf("the old row: %q %q %q %q %v", plan, marks, proposals, state, err)
	}

	execAll(t, db,
		`INSERT INTO recordings (id, project_id, state, source_kind, fps, actor, created_ms, plan)
 VALUES ('rec_0000000002', 'prj_0000000001', 'prepared', 'display', 30, 'local:ui', 2, '{"source_id":"d1"}')`)

	if rolled, err := full.Rollback(ctx, 1); err != nil || len(rolled) != 1 || rolled[0] != id {
		t.Fatalf("Rollback = %v, %v", rolled, err)
	}

	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM recordings`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("a prepared row must go with its plan: %d %v", n, err)
	}

	if _, err := db.ExecContext(ctx, `SELECT plan FROM recordings`); err == nil {
		t.Error("Down must drop the new columns")
	}
}
