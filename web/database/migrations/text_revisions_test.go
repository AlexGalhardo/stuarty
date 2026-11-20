package migrations

import (
	"context"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem/database"
)

const textRevisionsID = "20261119240000_text_revisions"

// textWorkspace is the workspace of a release, taken to the project items, with a brief and two texts in its first
// project: the texts epic 06 wrote, which had no kind and no history.
func textWorkspace(t *testing.T, r release) *database.DB {
	t.Helper()

	db := workspaceOf(t, r)
	upgrade(t, db)

	for _, q := range []string{
		`INSERT INTO project_items (id, project_id, kind, title, position, body, created_ms, updated_ms, created_by)
 VALUES ('pit_000000000b', 'prj_0000000001', 'brief', 'Brief', 1, 'a brief', 1, 2, 'local:cli')`,
		`INSERT INTO project_items (id, project_id, kind, title, position, body, created_ms, updated_ms, created_by)
 VALUES ('pit_000000000c', 'prj_0000000001', 'text', 'Script', 2, 'scene one', 1, 5, 'local:cli')`,
		`INSERT INTO project_items (id, project_id, kind, title, position, body, created_ms, updated_ms, created_by)
 VALUES ('pit_000000000d', 'prj_0000000001', 'text', 'Empty', 3, '', 1, 6, 'usr_a')`,
	} {
		if _, err := db.ExecContext(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	return db
}

func upgradeToTextRevisions(t *testing.T, db *database.DB) {
	t.Helper()

	m, err := newMigrator(db)
	if err != nil {
		t.Fatal(err)
	}

	ran, err := m.Up(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(ran, textRevisionsID) {
		t.Fatalf("the migration did not run: %v", ran)
	}
}

// TestTextRevisionsMigration is epic 17 task 3: the workspace of each release opens with every text a `copy` that has
// a revision 1 holding its body, made by its creator at the time it was last written, and nothing else touched.
func TestTextRevisionsMigration(t *testing.T) {
	for _, r := range releases {
		t.Run(r.name, func(t *testing.T) {
			db := textWorkspace(t, r)
			items := count(t, db, "SELECT count(*) FROM project_items")

			upgradeToTextRevisions(t, db)
			checkNoOrphans(t, db)

			if n := count(t, db, "SELECT count(*) FROM project_items"); n != items {
				t.Fatalf("items = %d, want %d", n, items)
			}

			if n := count(
				t,
				db,
				"SELECT count(*) FROM project_items WHERE kind = 'text' AND text_kind = 'copy'",
			); n != 2 {
				t.Errorf("texts with the kind copy = %d, want 2", n)
			}

			if n := count(
				t,
				db,
				"SELECT count(*) FROM project_items WHERE kind <> 'text' AND text_kind IS NOT NULL",
			); n != 0 {
				t.Errorf("%d items that are not texts have a text kind", n)
			}

			if n := count(t, db, "SELECT count(*) FROM text_revisions"); n != 2 {
				t.Fatalf("revisions = %d, want one for each text", n)
			}

			if n := count(t, db, `SELECT count(*) FROM text_revisions r JOIN project_items i ON i.id = r.item_id
 WHERE r.revision = 1 AND r.source = 'created' AND r.body = i.body AND r.actor = i.created_by
 AND r.created_ms = i.updated_ms AND r.provider IS NULL AND r.cost_micros IS NULL`); n != 2 {
				t.Errorf("revisions that match their text = %d, want 2", n)
			}
		})
	}
}

func TestTextRevisionsMigrationDown(t *testing.T) {
	ctx := context.Background()
	db := textWorkspace(t, releases[len(releases)-1])

	upgradeToTextRevisions(t, db)

	m, err := newMigrator(db)
	if err != nil {
		t.Fatal(err)
	}

	want := downTo(textRevisionsID)

	rolled, err := m.Rollback(ctx, len(want))
	if err != nil || !slices.Equal(rolled, want) {
		t.Fatalf("Rollback = %v, %v; want %v", rolled, err, want)
	}

	if got := columns(t, db, "text_revisions"); len(got) != 0 {
		t.Errorf("text_revisions still exists after the down: %v", got)
	}

	if n := count(t, db, "SELECT count(*) FROM pragma_table_info('project_items') WHERE name = 'text_kind'"); n != 0 {
		t.Errorf("text_kind is still a column after the down")
	}

	if n := count(t, db, "SELECT count(*) FROM project_items WHERE kind = 'text' AND body <> ''"); n != 1 {
		t.Errorf("the bodies of the texts are not kept: %d", n)
	}

	upgradeToTextRevisions(t, db)

	if n := count(t, db, "SELECT count(*) FROM text_revisions"); n != 2 {
		t.Errorf("revisions after going forward again = %d, want 2", n)
	}
}

func TestTextRevisionsConstraints(t *testing.T) {
	ctx := context.Background()
	db := textWorkspace(t, releases[len(releases)-1])

	upgradeToTextRevisions(t, db)

	good := `INSERT INTO text_revisions (item_id, revision, body, actor, source, created_ms)
 VALUES ('pit_000000000c', 2, 'x', 'local:cli', 'write', 9)`
	if _, err := db.ExecContext(ctx, good); err != nil {
		t.Fatalf("a valid revision was refused: %v", err)
	}

	bad := map[string]string{
		"a kind that does not exist": `UPDATE project_items SET text_kind = 'poem' WHERE id = 'pit_000000000c'`,
		"an unknown source": `INSERT INTO text_revisions (item_id, revision, body, actor, source, created_ms)
 VALUES ('pit_000000000c', 3, 'x', 'a', 'stolen', 9)`,
		"revision zero": `INSERT INTO text_revisions (item_id, revision, body, actor, source, created_ms)
 VALUES ('pit_000000000c', 0, 'x', 'a', 'write', 9)`,
		"a repeated revision": good,
		"a negative cost": `INSERT INTO text_revisions (item_id, revision, body, actor, source, cost_micros, created_ms)
 VALUES ('pit_000000000c', 3, 'x', 'a', 'generated', -1, 9)`,
		"an item that does not exist": `INSERT INTO text_revisions (item_id, revision, body, actor, source, created_ms)
 VALUES ('pit_missing', 1, 'x', 'a', 'write', 9)`,
	}

	for name, q := range bad {
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM project_items WHERE id = 'pit_000000000c'`); err != nil {
		t.Fatal(err)
	}

	if n := count(t, db, "SELECT count(*) FROM text_revisions WHERE item_id = 'pit_000000000c'"); n != 0 {
		t.Errorf("%d revisions outlived their item", n)
	}
}
