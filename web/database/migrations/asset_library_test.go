package migrations

import (
	"context"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem/database"
	"github.com/AlexGalhardo/vukem/schema"
)

const assetLibraryID = "20261119150000_asset_library"

// libraryWorkspace is the workspace of a release with a few more files in it: one used by two projects under two
// names, one used by one project, and one that no project uses any more.
func libraryWorkspace(t *testing.T, r release) *database.DB {
	t.Helper()

	db := workspaceOf(t, r)
	exec := func(q string, args ...any) {
		t.Helper()

		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	exec(`INSERT INTO assets (id, size_bytes, original_name, created_ms, updated_ms) VALUES
 ('sha256:bb', 20, 'reaction.mov', 5, 5), ('sha256:cc', 30, 'logo.png', 6, 6)`)
	exec(`INSERT INTO project_assets (project_id, asset_id, asset_hash, state, name, imported_by, imported_ms) VALUES
 ('prj_0000000001', 'ast_0000000002', 'sha256:bb', 'active', 'reaction.mov', 'local:cli', 7),
 ('prj_0000000003', 'ast_0000000003', 'sha256:bb', 'active', 'Funny face', 'usr_a', 3),
 ('prj_0000000003', 'ast_0000000004', 'sha256:aa', 'trashed', 'a.mp4', 'usr_a', 9)`)

	return db
}

// downTo is what a rollback to id, inclusive, takes off: the ids from the newest migration down to id. A test of a
// down names its own migration and keeps passing when later ones are added.
func downTo(id string) []string {
	all := All()
	at := slices.IndexFunc(all, func(mig schema.Migration) bool { return mig.ID == id })

	var want []string
	for i := len(all) - 1; i >= at && at >= 0; i-- {
		want = append(want, all[i].ID)
	}

	return want
}

func newMigrator(db *database.DB) (*schema.Migrator, error) {
	return schema.NewMigrator(db, allMigrations())
}

func upgradeAll(t *testing.T, db *database.DB) {
	t.Helper()

	m, err := newMigrator(db)
	if err != nil {
		t.Fatal(err)
	}

	ran, err := m.Up(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(ran, assetLibraryID) {
		t.Fatalf("the migration did not run: %v", ran)
	}
}

func searchHashes(t *testing.T, db *database.DB, match string) []string {
	t.Helper()

	rows, err := db.QueryContext(context.Background(),
		"SELECT asset_hash FROM asset_search WHERE asset_search MATCH ? ORDER BY asset_hash", match)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var out []string

	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			t.Fatal(err)
		}

		out = append(out, h)
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	return out
}

// TestWorkspaceAssetsMigration is epic 08 task 3: the workspace of each release opens with every asset kept, every
// project's use of it intact and every file in the library as an imported one, found by the name it is known by in
// any project.
func TestWorkspaceAssetsMigration(t *testing.T) {
	for _, r := range releases {
		t.Run(r.name, func(t *testing.T) {
			ctx := context.Background()
			db := libraryWorkspace(t, r)

			assets := count(t, db, "SELECT count(*) FROM assets")
			uses := count(t, db, "SELECT count(*) FROM project_assets")

			upgradeAll(t, db)
			checkNoOrphans(t, db)

			if n := count(t, db, "SELECT count(*) FROM assets"); n != assets || assets != 3 {
				t.Fatalf("assets = %d, want %d (3)", n, assets)
			}

			if n := count(t, db, "SELECT count(*) FROM project_assets"); n != uses {
				t.Errorf("project_assets = %d, want %d", n, uses)
			}

			// Every use still resolves its media by the hash.
			if n := count(
				t,
				db,
				`SELECT count(*) FROM project_assets p JOIN assets a ON a.id = p.asset_hash`,
			); n != uses {
				t.Errorf("%d of %d uses resolve their asset", n, uses)
			}

			// One imported row for each owner whose projects use a file (the projects of the fixture belong to
			// the local owner and to usr_a), and one for the local owner for a file no project uses.
			if n := count(t, db, `SELECT count(*) FROM assets a JOIN asset_info i ON i.asset_hash = a.id
 WHERE i.origin_kind = 'imported' AND i.reusable = 0 AND i.source = '' AND i.created_ms = a.created_ms`); n != 5 {
				t.Errorf("%d imported rows, want 5", n)
			}

			by := map[string]string{
				":sha256:bb": "local:cli", "usr_a:sha256:bb": "usr_a", ":sha256:cc": "migration",
			}
			for key, want := range by {
				var got string
				if err := db.QueryRowContext(ctx,
					"SELECT created_by FROM asset_info WHERE owner_user_id || ':' || asset_hash = ?", key).
					Scan(&got); err != nil || got != want {
					t.Errorf("created_by of %s = %q, %v; want %q (the oldest use of that owner)", key, got, err, want)
				}
			}

			if got := searchHashes(t, db, `"reaction"*`); !slices.Equal(got, []string{"sha256:bb"}) {
				t.Errorf("search by the first name: %v", got)
			}

			if got := searchHashes(t, db, `"funny"*`); !slices.Equal(got, []string{"sha256:bb"}) {
				t.Errorf("search by the name in another project: %v", got)
			}

			if got := searchHashes(t, db, `"logo"*`); !slices.Equal(got, []string{"sha256:cc"}) {
				t.Errorf("search for the unused file: %v", got)
			}
		})
	}
}

// TestWorkspaceAssetsMigrationDown drops the library tables and leaves the assets and their uses; going forward
// again rebuilds the rows.
func TestWorkspaceAssetsMigrationDown(t *testing.T) {
	ctx := context.Background()
	db := libraryWorkspace(t, releases[len(releases)-1])

	upgradeAll(t, db)

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO asset_tags (owner_user_id, asset_hash, tag) VALUES ('', 'sha256:aa', 'logo')`,
	); err != nil {
		t.Fatal(err)
	}

	m, err := newMigrator(db)
	if err != nil {
		t.Fatal(err)
	}

	// The migrations of later epics come off first, one at a time, until the library's own.
	var rolled []string

	for !slices.Contains(rolled, assetLibraryID) {
		step, rerr := m.Rollback(ctx, 1)
		if rerr != nil || len(step) != 1 {
			t.Fatalf("Rollback = %v, %v (rolled so far %v)", step, rerr, rolled)
		}

		rolled = append(rolled, step...)
	}

	if rolled[len(rolled)-1] != assetLibraryID {
		t.Fatalf("Rollback = %v", rolled)
	}

	for _, table := range []string{"asset_info", "asset_tags", "asset_search"} {
		if got := columns(t, db, table); len(got) != 0 {
			t.Errorf("%s still exists after the down: %v", table, got)
		}
	}

	if n := count(t, db, "SELECT count(*) FROM assets"); n != 3 {
		t.Errorf("assets after the down = %d, want 3", n)
	}

	upgradeAll(t, db)

	if n := count(t, db, "SELECT count(*) FROM asset_info"); n != 5 {
		t.Errorf("asset_info after going forward again = %d, want 5", n)
	}
}

func TestAssetLibraryConstraints(t *testing.T) {
	ctx := context.Background()
	db := libraryWorkspace(t, releases[len(releases)-1])

	upgradeAll(t, db)

	bad := []string{
		`UPDATE asset_info SET origin_kind = 'stolen' WHERE asset_hash = 'sha256:aa'`,
		`UPDATE asset_info SET reusable = 2 WHERE asset_hash = 'sha256:aa'`,
		`UPDATE asset_info SET cost_micros = -1 WHERE asset_hash = 'sha256:aa'`,
		`INSERT INTO asset_tags (owner_user_id, asset_hash, tag) VALUES ('', 'sha256:missing', 'x')`,
		`INSERT INTO asset_info (asset_hash, created_by, created_ms, updated_ms) VALUES ('sha256:missing', 'x', 1, 1)`,
	}

	for _, q := range bad {
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Errorf("accepted: %s", q)
		}
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO asset_tags (owner_user_id, asset_hash, tag) VALUES ('', 'sha256:cc', 'logo')`,
	); err != nil {
		t.Fatal(err)
	}

	// An asset that goes takes its library row and its tags with it.
	if _, err := db.ExecContext(ctx, `DELETE FROM assets WHERE id = 'sha256:cc'`); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"asset_info", "asset_tags"} {
		if n := count(t, db, "SELECT count(*) FROM "+table+" WHERE asset_hash = 'sha256:cc'"); n != 0 {
			t.Errorf("%s kept the row of a deleted asset", table)
		}
	}
}

// TestAssetLibraryMigrationPerOwner is VV-254 (prj_0000000003 belongs to usr_a, the others to the local owner): a file that projects of two owners use gets one library row for
// each owner, created by that owner's oldest import and found by the names that owner's projects give it, and a
// file no project uses gets one for the empty owner.
func TestAssetLibraryMigrationPerOwner(t *testing.T) {
	ctx := context.Background()
	db := libraryWorkspace(t, releases[len(releases)-1])

	upgradeAll(t, db)

	owners := func(hash string) []string {
		rows, err := db.QueryContext(
			ctx,
			"SELECT owner_user_id || ':' || created_by FROM asset_info WHERE asset_hash = ? ORDER BY owner_user_id",
			hash,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()

		var out []string

		for rows.Next() {
			var o string
			if err := rows.Scan(&o); err != nil {
				t.Fatal(err)
			}

			out = append(out, o)
		}

		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}

		return out
	}

	if got := owners("sha256:bb"); !slices.Equal(got, []string{":local:cli", "usr_a:usr_a"}) {
		t.Errorf("rows of the shared file = %v", got)
	}

	if got := owners("sha256:cc"); !slices.Equal(got, []string{":migration"}) {
		t.Errorf("rows of the unused file = %v", got)
	}

	found := func(owner, match string) int {
		return count(
			t,
			db,
			"SELECT count(*) FROM asset_search WHERE owner_user_id = '"+owner+"' AND asset_search MATCH '"+match+"'",
		)
	}

	if found("usr_a", `"funny"*`) != 1 || found("", `"funny"*`) != 0 {
		t.Error("the name one owner gave the file is found by the other owner")
	}

	if found("", `"reaction"*`) != 1 || found("usr_a", `"reaction"*`) != 0 {
		t.Error("the name the other owner gave the file is found by this owner")
	}
}
