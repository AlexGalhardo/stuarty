package migrations

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/vukem/schema"
)

// The tracks of a recording (SDD-025 section 16.8): a table of epic 61 keeps its rows, gains the three columns with
// their defaults and the `paused` state, and Down returns it as it was with a paused row made a recording.
func TestRecordingsTracksUpAndDown(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	const id = "20261124160000_recordings_tracks"

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
		`INSERT INTO recordings (id, project_id, state, review, source_kind, source_label, fps, duration_ms, asset_id,
 actor, created_ms)
 VALUES ('rec_0000000001', 'prj_0000000001', 'ready', 'added', 'window', 'sealed', 30, 4200, 'ast_0000000001',
 'local:ui', 1)`,
	)

	// Through the migration under test: a later one would be the one Rollback undoes.
	var throughID []schema.Migration

	for _, m := range allMigrations() {
		if m.ID <= id {
			throughID = append(throughID, m)
		}
	}

	full, err := schema.NewMigrator(db, throughID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := full.Up(ctx); err != nil {
		t.Fatal(err)
	}

	var tracks, pauses string

	var events int
	if err := db.QueryRowContext(ctx,
		`SELECT tracks, pauses, events_count FROM recordings WHERE id = 'rec_0000000001'`).
		Scan(&tracks, &pauses, &events); err != nil || tracks != "[]" || pauses != "[]" || events != 0 {
		t.Fatalf("the old row: %q %q %d %v", tracks, pauses, events, err)
	}

	var indexes int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name IN
 ('recordings_project_index', 'recordings_state_index')`).Scan(&indexes); err != nil || indexes != 2 {
		t.Fatalf("the rebuild lost an index: %d %v", indexes, err)
	}

	var state, review, label, asset string

	var duration int
	if err := db.QueryRowContext(ctx,
		`SELECT state, review, source_label, duration_ms, asset_id FROM recordings WHERE id = 'rec_0000000001'`).
		Scan(&state, &review, &label, &duration, &asset); err != nil || state != "ready" || review != "added" ||
		label != "sealed" || duration != 4200 || asset != "ast_0000000001" {
		t.Fatalf("the old row lost a value: %q %q %q %d %q %v", state, review, label, duration, asset, err)
	}

	execAll(t, db,
		`INSERT INTO recordings (id, project_id, state, source_kind, fps, actor, created_ms)
 VALUES ('rec_0000000002', 'prj_0000000001', 'paused', 'display', 30, 'local:ui', 2)`)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO recordings (id, project_id, state, source_kind, fps, actor, created_ms)
 VALUES ('rec_0000000003', 'prj_0000000001', 'exploding', 'display', 30, 'local:ui', 3)`); err == nil {
		t.Error("a state outside the closed set was accepted")
	}

	if rolled, err := full.Rollback(ctx, 1); err != nil || len(rolled) != 1 || rolled[0] != id {
		t.Fatalf("Rollback = %v, %v", rolled, err)
	}

	if err := db.QueryRowContext(ctx, `SELECT state FROM recordings WHERE id = 'rec_0000000002'`).
		Scan(&state); err != nil ||
		state != "recording" {
		t.Fatalf("a paused row after Down: %q %v", state, err)
	}

	if _, err := db.ExecContext(ctx, `SELECT tracks FROM recordings`); err == nil {
		t.Error("Down must drop the new columns")
	}
}
