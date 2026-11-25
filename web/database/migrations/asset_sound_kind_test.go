package migrations

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/vukem/schema"
)

// A row of the library made before the migration keeps an empty sound kind; the column refuses a kind that is not
// one of the three; Down drops it.
func TestAssetSoundKindUpAndDown(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	const id = "20261125120000_asset_sound_kind"

	var before, through []schema.Migration

	for _, m := range allMigrations() {
		if m.ID < id {
			before = append(before, m)
		}

		if m.ID <= id {
			through = append(through, m)
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
		`INSERT INTO assets (id, kind, size_bytes, original_name, created_ms, updated_ms)
 VALUES ('sha256:aa', 'audio', 1, 'a.wav', 1, 1)`,
		`INSERT INTO asset_info (owner_user_id, asset_hash, created_by, created_ms, updated_ms)
 VALUES ('', 'sha256:aa', 'u', 1, 1)`,
	)

	full, err := schema.NewMigrator(db, through)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := full.Up(ctx); err != nil {
		t.Fatal(err)
	}

	var kind string
	if err := db.QueryRowContext(ctx, `SELECT sound_kind FROM asset_info WHERE asset_hash = 'sha256:aa'`).
		Scan(&kind); err != nil || kind != "" {
		t.Fatalf("the old row: %q, %v", kind, err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE asset_info SET sound_kind = 'voice'`); err != nil {
		t.Fatalf("voice: %v", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE asset_info SET sound_kind = 'noise'`); err == nil {
		t.Error("a sound kind outside the three must be refused")
	}

	if rolled, err := full.Rollback(ctx, 1); err != nil || len(rolled) != 1 || rolled[0] != id {
		t.Fatalf("Rollback = %v, %v", rolled, err)
	}

	if _, err := db.ExecContext(ctx, `SELECT sound_kind FROM asset_info`); err == nil {
		t.Error("Down must drop the column")
	}
}
