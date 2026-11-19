package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem/database"
	"github.com/AlexGalhardo/vukem/queue"
	"github.com/AlexGalhardo/vukem/schema"
)

// release is the set of migrations a released binary had: the fixture of a workspace of that release is the
// database those migrations make, filled with what the release wrote (the tags were read with
// git ls-tree <tag> web/database/migrations/).
type release struct {
	name    string
	through string // the last migration of the release
	apiKeys bool   // the release has the api_keys table
}

var releases = []release{
	{name: "0.2.0", through: "20261114100700_create_audit_table"},
	{name: "0.17.0", through: "20261115150000_assets_frame_delays_and_frames_kind", apiKeys: true},
	{name: "0.29.0", through: "20261116130000_assets_lut_kind", apiKeys: true},
}

func allMigrations() []schema.Migration {
	return slices.Concat(
		queue.Migrations(queue.DatabaseOptions{Table: "queue_jobs", FailedTable: "queue_failed_jobs"}),
		All(),
	)
}

func openDB(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.Open(context.Background(), "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "db.sqlite")))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return db
}

// workspaceOf makes the database of a release, with three projects in it and everything that hangs from a
// project: a log with an undo, assets, a job, a backup, an idempotency key, an audit row and, where the table
// exists, an API key.
func workspaceOf(t *testing.T, r release) *database.DB {
	t.Helper()

	ctx := context.Background()
	db := openDB(t)

	var migs []schema.Migration

	for _, m := range allMigrations() {
		if m.ID <= r.through {
			migs = append(migs, m)
		}
	}

	m, err := schema.NewMigrator(db, migs)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}

	exec := func(q string, args ...any) {
		t.Helper()

		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	const insert = `INSERT INTO projects (id, name, state, revision, schema_version, document, doc_sha256, width, height,
 fps_num, fps_den, created_ms, updated_ms, created_by, owner_user_id, trashed_ms, trashed_by)
 VALUES (?, ?, ?, 3, 1, '{"id":"x"}', 'sha', 1080, 1920, 30, 1, 100, 200, 'local:cli', ?, ?, ?)`

	exec(insert, "prj_0000000001", "Teaser", "active", nil, nil, nil)
	exec(insert, "prj_0000000002", "Old ad", "trashed", nil, 300, "local:ui")
	exec(insert, "prj_0000000003", "Mine", "active", "usr_a", nil, nil)

	for _, id := range []string{"prj_0000000001", "prj_0000000002", "prj_0000000003"} {
		exec(
			`INSERT INTO operations (project_id, revision, kind, actor, base_revision, target_revision, at_ms, ops, inverse,
 diff, schema_version, doc_sha256, closed_by) VALUES (?, 1, 'edit', 'local:cli', 0, NULL, 10, '[]', '[]', '[]', 1, 'a', NULL)`,
			id,
		)
		exec(
			`INSERT INTO operations (project_id, revision, kind, actor, base_revision, target_revision, at_ms, ops, inverse,
 diff, schema_version, doc_sha256, closed_by) VALUES (?, 2, 'edit', 'local:cli', 1, NULL, 11, '[]', '[]', '[]', 1, 'b', 3)`,
			id,
		)
		exec(
			`INSERT INTO operations (project_id, revision, kind, actor, base_revision, target_revision, at_ms, ops, inverse,
 diff, schema_version, doc_sha256, closed_by) VALUES (?, 3, 'undo', 'local:cli', 2, 2, 12, '[]', '[]', '[]', 1, 'a', NULL)`,
			id,
		)
		exec(
			`INSERT INTO idempotency_keys (project_id, actor, key, request_hash, revision, result, created_ms, expires_ms)
 VALUES (?, 'local:cli', 'k1', 'h', 1, '{}', 1, 999)`,
			id,
		)
		exec(
			`INSERT INTO document_backups (owner_kind, owner_id, revision, schema_version, document, reason, created_ms)
 VALUES ('project', ?, 0, 1, '{}', 'migrate', 1)`,
			id,
		)
		exec(`INSERT INTO audit (at_ms, actor, action, project_id) VALUES (5, 'local:cli', 'project.create', ?)`, id)
	}

	exec(
		`INSERT INTO assets (id, size_bytes, original_name, created_ms, updated_ms) VALUES ('sha256:aa', 10, 'a.mp4', 1, 1)`,
	)
	exec(`INSERT INTO project_assets (project_id, asset_id, asset_hash, state, name, imported_by, imported_ms)
 VALUES ('prj_0000000001', 'ast_0000000001', 'sha256:aa', 'active', 'a.mp4', 'local:cli', 1)`)
	exec(`INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms)
 VALUES ('job_0000000001', 'export', 'prj_0000000002', 'local:cli', '{}', 1, 1)`)

	if r.apiKeys {
		exec(`INSERT INTO api_keys VALUES ('key_0000000001', 'tok_1', 'usr_a', 'prj_0000000003', 'k', 1)`)
	}

	return db
}

// upgrade runs the migrations through project_items and no further: these tests are about that one, and a
// later migration would be the last one a Rollback undoes.
func upgrade(t *testing.T, db *database.DB) *schema.Migrator {
	t.Helper()

	var through []schema.Migration

	for _, m := range allMigrations() {
		if m.ID <= "20261119120000_project_items" {
			through = append(through, m)
		}
	}

	m, err := schema.NewMigrator(db, through)
	if err != nil {
		t.Fatal(err)
	}

	ran, err := m.Up(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(ran, "20261119120000_project_items") {
		t.Fatalf("the migration did not run: %v", ran)
	}

	return m
}

func count(t *testing.T, db *database.DB, q string, args ...any) int {
	t.Helper()

	var n int
	if err := db.QueryRowContext(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}

	return n
}

func checkNoOrphans(t *testing.T, db *database.DB) {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	if rows.Next() {
		t.Error("foreign_key_check found a violation")
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// TestProjectItemsMigration is epic 06 task 3: a workspace of each release opens with every former project as a
// project with one timeline, with its ids, its log and everything that hangs from it intact.
func TestProjectItemsMigration(t *testing.T) {
	for _, r := range releases {
		t.Run(r.name, func(t *testing.T) {
			ctx := context.Background()
			db := workspaceOf(t, r)

			ids := []string{"prj_0000000001", "prj_0000000002", "prj_0000000003"}
			before := map[string]int{}

			for _, table := range []string{"operations", "idempotency_keys", "document_backups", "audit", "project_assets", "jobs"} {
				before[table] = count(t, db, "SELECT count(*) FROM "+table)
			}

			upgrade(t, db)
			checkNoOrphans(t, db)

			if n := count(t, db, "SELECT count(*) FROM projects"); n != len(ids) {
				t.Fatalf("projects = %d, want %d", n, len(ids))
			}

			for _, id := range ids {
				var name, state, itemState, kind, title, ref string

				var position int

				err := db.QueryRowContext(ctx, `SELECT p.name, p.state, i.state, i.kind, i.title, i.position, i.ref
 FROM projects p JOIN project_items i ON i.project_id = p.id WHERE p.id = ?`, id).
					Scan(&name, &state, &itemState, &kind, &title, &position, &ref)
				if err != nil {
					t.Fatalf("%s: %v", id, err)
				}

				if kind != "timeline" || ref != id || position != 0 || itemState != "active" || title != name {
					t.Errorf(
						"%s: item kind=%s ref=%s position=%d state=%s title=%q name=%q",
						id,
						kind,
						ref,
						position,
						itemState,
						title,
						name,
					)
				}

				if n := count(t, db, "SELECT count(*) FROM project_items WHERE project_id = ?", id); n != 1 {
					t.Errorf("%s has %d items, want 1", id, n)
				}

				var tlState string

				var revision int
				if err := db.QueryRowContext(ctx, "SELECT state, revision FROM timelines WHERE id = ?", id).
					Scan(&tlState, &revision); err != nil {
					t.Fatal(err)
				}

				if tlState != state || revision != 3 {
					t.Errorf("%s: timeline state %s revision %d, project state %s", id, tlState, revision, state)
				}
			}

			var trashedBy sql.NullString

			var owner sql.NullString
			if err := db.QueryRowContext(ctx, "SELECT trashed_by FROM projects WHERE id = 'prj_0000000002'").
				Scan(&trashedBy); err != nil ||
				trashedBy.String != "local:ui" {
				t.Errorf("trashed_by = %v, %v", trashedBy, err)
			}

			if err := db.QueryRowContext(ctx, "SELECT owner_user_id FROM projects WHERE id = 'prj_0000000003'").
				Scan(&owner); err != nil ||
				owner.String != "usr_a" {
				t.Errorf("owner = %v, %v", owner, err)
			}

			for table, n := range before {
				if got := count(t, db, "SELECT count(*) FROM "+table); got != n {
					t.Errorf("%s has %d rows after the migration, %d before", table, got, n)
				}
			}

			// The history and the undo chain: the rows are as they were, and still hang from the timeline.
			if n := count(
				t,
				db,
				"SELECT count(*) FROM operations WHERE project_id = 'prj_0000000001' AND closed_by = 3",
			); n != 1 {
				t.Errorf("the undone step lost its closing row: %d", n)
			}

			for _, child := range []string{"operations", "idempotency_keys", "project_assets", "jobs"} {
				var target string
				if err := db.QueryRowContext(ctx, "SELECT \"table\" FROM pragma_foreign_key_list(?) WHERE \"from\" = 'project_id'", child).
					Scan(&target); err != nil ||
					target != "timelines" {
					t.Errorf("%s.project_id references %q (%v), want timelines", child, target, err)
				}
			}

			// A cascade still works through the renamed table.
			if _, err := db.ExecContext(ctx, "DELETE FROM timelines WHERE id = 'prj_0000000002'"); err != nil {
				t.Fatal(err)
			}

			if n := count(t, db, "SELECT count(*) FROM operations WHERE project_id = 'prj_0000000002'"); n != 0 {
				t.Errorf("the log of a deleted timeline stayed: %d rows", n)
			}
		})
	}
}

// TestProjectItemsMigrationDown is the down path: the former table comes back with its rows, a timeline that is
// not the first of its project goes with its log, and the migration can run again.
func TestProjectItemsMigrationDown(t *testing.T) {
	for _, r := range releases {
		t.Run(r.name, func(t *testing.T) {
			ctx := context.Background()
			db := workspaceOf(t, r)
			m := upgrade(t, db)

			exec := func(q string, args ...any) {
				t.Helper()

				if _, err := db.ExecContext(ctx, q, args...); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}

			// A second timeline of the first project, written by a later release.
			exec(
				`INSERT INTO timelines (id, name, schema_version, document, doc_sha256, width, height, fps_num, fps_den,
 created_ms, updated_ms, created_by) VALUES ('prj_0000000009', 'Second', 1, '{}', 'x', 1, 1, 30, 1, 1, 1, 'local:cli')`,
			)
			exec(
				`INSERT INTO project_items (id, project_id, kind, title, position, ref, created_ms, updated_ms, created_by)
 VALUES ('pit_0000000009', 'prj_0000000001', 'timeline', 'Second', 1, 'prj_0000000009', 1, 1, 'local:cli')`,
			)
			exec(
				`INSERT INTO operations (project_id, revision, kind, actor, base_revision, at_ms, ops, inverse, diff, schema_version, doc_sha256)
 VALUES ('prj_0000000009', 1, 'edit', 'local:cli', 0, 10, '[]', '[]', '[]', 1, 'a')`,
			)
			exec(
				`INSERT INTO project_items (id, project_id, kind, title, position, body, created_ms, updated_ms, created_by)
 VALUES ('pit_000000000a', 'prj_0000000001', 'text', 'Script', 2, 'scene', 1, 1, 'local:cli')`,
			)

			// The export_batch kind (epic 35), the library index (epic 15) and the approval tables (epic 05) are newer
			// than project_items: they go down first.
			rolled, err := m.Rollback(ctx, 5)
			if err != nil || len(rolled) != 5 || rolled[4] != "20261119120000_project_items" {
				t.Fatalf("Rollback = %v, %v", rolled, err)
			}

			for _, table := range []string{"project_items", "timelines"} {
				if got := columns(t, db, table); len(got) != 0 {
					t.Errorf("%s still exists after the down: %v", table, got)
				}
			}

			if n := count(t, db, "SELECT count(*) FROM projects"); n != 3 {
				t.Errorf("projects after the down = %d, want 3 (the second timeline is gone)", n)
			}

			if n := count(t, db, "SELECT count(*) FROM operations WHERE project_id = 'prj_0000000009'"); n != 0 {
				t.Errorf("the log of the second timeline stayed: %d", n)
			}

			if n := count(t, db, "SELECT count(*) FROM operations"); n != 9 {
				t.Errorf("operations after the down = %d, want 9", n)
			}

			var doc string
			if err := db.QueryRowContext(ctx, "SELECT document FROM projects WHERE id = 'prj_0000000001'").
				Scan(&doc); err != nil ||
				doc != `{"id":"x"}` {
				t.Errorf("the document after the down = %q, %v", doc, err)
			}

			checkNoOrphans(t, db)

			var idx string
			if err := db.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE name = 'projects_state_updated_ms_index'").
				Scan(&idx); err != nil ||
				idx == "" {
				t.Errorf("the index of the former table: %q, %v", idx, err)
			}

			// And forward again.
			upgrade(t, db)
			checkNoOrphans(t, db)

			if n := count(t, db, "SELECT count(*) FROM project_items WHERE kind = 'timeline'"); n != 3 {
				t.Errorf("timeline items after going forward again = %d", n)
			}
		})
	}
}

func TestProjectItemsConstraints(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	exec := func(q string) error {
		_, err := db.ExecContext(ctx, q)
		return err
	}

	if err := exec(
		`INSERT INTO projects (id, name, created_ms, updated_ms, created_by) VALUES ('prj_a', 'a', 1, 1, 'u')`,
	); err != nil {
		t.Fatal(err)
	}

	item := func(id, kind, state, ref string) string {
		return fmt.Sprintf(
			`INSERT INTO project_items (id, project_id, kind, title, position, state, ref, created_ms, updated_ms, created_by)
 VALUES ('%s', 'prj_a', '%s', 't', 0, '%s', %s, 1, 1, 'u')`,
			id,
			kind,
			state,
			ref,
		)
	}

	if err := exec(item("pit_1", "brief", "active", "NULL")); err != nil {
		t.Fatal(err)
	}

	bad := map[string]string{
		"a kind that is reserved":         item("pit_2", "image", "active", "NULL"),
		"an unknown state":                item("pit_3", "text", "gone", "NULL"),
		"a second active brief":           item("pit_4", "brief", "active", "NULL"),
		"an item of a missing project":    `INSERT INTO project_items (id, project_id, kind, title, position, created_ms, updated_ms, created_by) VALUES ('pit_5', 'prj_zz', 'text', 't', 0, 1, 1, 'u')`,
		"a project state that is unknown": `UPDATE projects SET state = 'gone'`,
	}
	for name, q := range bad {
		if err := exec(q); err == nil {
			t.Errorf("%s: the database accepted %s", name, q)
		}
	}

	if err := exec(item("pit_6", "brief", "trashed", "NULL")); err != nil {
		t.Errorf("a trashed brief beside the active one: %v", err)
	}

	if err := exec(item("pit_7", "timeline", "active", "'prj_a'")); err != nil {
		t.Fatal(err)
	}

	if err := exec(item("pit_8", "timeline", "active", "'prj_a'")); err == nil {
		t.Error("two timeline items for one timeline were accepted")
	}

	if err := exec(`DELETE FROM projects WHERE id = 'prj_a'`); err != nil {
		t.Fatal(err)
	}

	if n := count(t, db, "SELECT count(*) FROM project_items"); n != 0 {
		t.Errorf("items after the cascade = %d", n)
	}
}
