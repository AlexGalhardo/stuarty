package migrations

import (
	"context"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem/schema"
)

// The rows of a generation name the project, not a timeline (SDD-005 section 5.12.3): after the migration a job, an
// asset link and a key can name a project that has no timeline row; the rows of an old workspace stay; and Down
// puts the foreign keys back and drops only the rows the keys cannot hold.
func TestProjectScopedJobsMigrationUpAndDown(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	const before = "20261120140000_project_context"

	var upTo []schema.Migration

	for _, m := range allMigrations() {
		if m.ID <= before {
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
		`INSERT INTO timelines (id, name, revision, schema_version, document, doc_sha256, width, height, fps_num, fps_den,
 created_ms, updated_ms, created_by) VALUES ('prj_0000000001', 'Old', 0, 1, '{}', 'x', 1080, 1920, 30, 1, 1, 1, 'u')`,
		`INSERT INTO assets (id, size_bytes, original_name, created_ms, updated_ms) VALUES ('sha256:aa', 1, 'a.mp4', 1, 1)`,
		`INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms)
 VALUES ('job_0000000001', 'export', 'prj_0000000001', 'u', '{}', 1, 1)`,
		`INSERT INTO project_assets (project_id, asset_id, asset_hash, state, name, imported_by, imported_ms)
 VALUES ('prj_0000000001', 'ast_0000000001', 'sha256:aa', 'active', 'a.mp4', 'u', 1)`,
		`INSERT INTO api_keys (id, token_id, user_id, project_id, name, created_ms)
 VALUES ('key_0000000001', 'tok_1', 'usr_a', 'prj_0000000001', 'k', 1)`,
	)

	full, err := schema.NewMigrator(db, allMigrations())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := full.Up(ctx); err != nil {
		t.Fatal(err)
	}

	for table, q := range map[string]string{
		"jobs": `SELECT count(*) FROM jobs`, "project_assets": `SELECT count(*) FROM project_assets`,
		"api_keys": `SELECT count(*) FROM api_keys`,
	} {
		if n := count(t, db, q); n != 1 {
			t.Fatalf("%s kept %d rows through the rebuild, want 1", table, n)
		}
	}

	checkNoOrphans(t, db)

	// A project that has no timeline row can hold a job, a file and a key.
	execAll(
		t,
		db,
		`INSERT INTO projects (id, name, created_ms, updated_ms, created_by) VALUES ('prj_0000000002', 'New', 1, 1, 'u')`,
		`INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms)
 VALUES ('job_0000000002', 'image.generate', 'prj_0000000002', 'u', '{}', 1, 1)`,
		`INSERT INTO project_assets (project_id, asset_id, asset_hash, state, name, imported_by, imported_ms)
 VALUES ('prj_0000000002', 'ast_0000000002', 'sha256:aa', 'pending', 'b.png', 'u', 1)`,
		`INSERT INTO api_keys (id, token_id, user_id, project_id, name, created_ms)
 VALUES ('key_0000000002', 'tok_2', 'usr_a', 'prj_0000000002', 'k2', 1)`,
	)

	// The kind rule of the table still holds: an export names a project.
	if _, err := db.ExecContext(ctx, `INSERT INTO jobs (id, kind, actor, input, created_ms, updated_ms)
 VALUES ('job_0000000003', 'export', 'u', '{}', 1, 1)`); err == nil {
		t.Error("a job of a kind that needs a project was accepted with none")
	}

	// Down of this migration and of every one made after it, newest first.
	var want []string

	for _, m := range allMigrations() {
		if m.ID >= "20261121100000_project_scoped_jobs" {
			want = append([]string{m.ID}, want...)
		}
	}

	rolled, err := full.Rollback(ctx, len(want))
	if err != nil || !slices.Equal(rolled, want) {
		t.Fatalf("Rollback = %v, %v; want %v", rolled, err, want)
	}

	for table, q := range map[string]string{
		"jobs": `SELECT count(*) FROM jobs`, "project_assets": `SELECT count(*) FROM project_assets`,
		"api_keys": `SELECT count(*) FROM api_keys`,
	} {
		if n := count(t, db, q); n != 1 {
			t.Errorf("Down left %d rows in %s, want only the row of the timeline", n, table)
		}
	}

	checkNoOrphans(t, db)

	if _, err := db.ExecContext(ctx, `INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms)
 VALUES ('job_0000000004', 'image.generate', 'prj_0000000002', 'u', '{}', 1, 1)`); err == nil {
		t.Error("Down did not put the foreign key of jobs back")
	}
}
