package migrations

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem/schema"
)

const renderOriginID = "20261119210000_asset_render_origin"

// hasColumn reads the lines of columns(): a column is a line that starts with its name and its text type.
func hasColumn(lines []string, name string) bool {
	return slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, name+" TEXT 1 '' ") })
}

// The render origin (SDD-005 section 5.5.1): three text columns, empty on the rows that exist, and a down that takes
// them off and leaves the library as it was.
func TestAssetRenderOriginMigration(t *testing.T) {
	ctx := context.Background()
	db := libraryWorkspace(t, releases[len(releases)-1])

	upgradeAll(t, db)

	got := columns(t, db, "asset_info")
	for _, c := range []string{"engine", "engine_version", "render_key"} {
		if !hasColumn(got, c) {
			t.Errorf("asset_info has no %s column: %v", c, got)
		}
	}

	// The rows the library migration wrote (one per owner and hash, SDD-011 DS-22) all keep an empty render origin.
	rows := count(t, db, "SELECT count(*) FROM asset_info")
	if rows == 0 {
		t.Fatal("the workspace has no asset_info row to keep")
	}

	if n := count(
		t,
		db,
		`SELECT count(*) FROM asset_info WHERE engine = '' AND engine_version = '' AND render_key = ''`,
	); n != rows {
		t.Errorf("rows with the empty render origin = %d, want %d", n, rows)
	}

	m, err := newMigrator(db)
	if err != nil {
		t.Fatal(err)
	}

	// Later epics add migrations after this one (the texts of epic 17): they go first, and this one is the last
	// that is undone.
	all := All()
	at := slices.IndexFunc(all, func(m schema.Migration) bool { return m.ID == renderOriginID })

	rolled, err := m.Rollback(ctx, len(all)-at)
	if err != nil || at < 0 || len(rolled) != len(all)-at || rolled[len(rolled)-1] != renderOriginID {
		t.Fatalf("Rollback = %v, %v", rolled, err)
	}

	got = columns(t, db, "asset_info")
	for _, c := range []string{"engine", "engine_version", "render_key"} {
		if hasColumn(got, c) {
			t.Errorf("%s is still there after the down: %v", c, got)
		}
	}

	if n := count(t, db, "SELECT count(*) FROM asset_info"); n != rows {
		t.Errorf("asset_info rows after the down = %d, want %d", n, rows)
	}
}
