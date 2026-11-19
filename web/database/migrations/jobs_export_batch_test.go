package migrations

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem/database"
	"github.com/AlexGalhardo/vukem/queue"
	"github.com/AlexGalhardo/vukem/schema"
)

const exportBatchMigrationID = "20261119120050_jobs_export_batch_kind"

// migratedBefore is a workspace migrated up to the migration before the export_batch rebuild, with the
// migrator that runs the rest.
func migratedBefore(t *testing.T) (*database.DB, *schema.Migrator) {
	t.Helper()

	ctx := context.Background()

	db, err := database.Open(ctx, "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "db.sqlite")))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	qopts := queue.DatabaseOptions{Table: "queue_jobs", FailedTable: "queue_failed_jobs"}
	all := slices.Concat(queue.Migrations(qopts), All())
	before := slices.DeleteFunc(slices.Clone(all), func(m schema.Migration) bool {
		return m.ID >= exportBatchMigrationID
	})

	m, err := schema.NewMigrator(db, before)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}

	rest, err := schema.NewMigrator(db, all)
	if err != nil {
		t.Fatal(err)
	}

	return db, rest
}

func execAll(t *testing.T, db *database.DB, qs ...string) {
	t.Helper()

	for _, q := range qs {
		if _, err := db.ExecContext(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func jobRows(t *testing.T, db *database.DB) []string {
	t.Helper()

	rows, err := db.QueryContext(
		context.Background(),
		`SELECT id || '|' || kind || '|' || coalesce(project_id, '-') || '|' ||
 coalesce(asset_hash, '-') || '|' || actor || '|' || state || '|' || progress || '|' || message || '|' || input || '|' ||
 coalesce(result, '-') || '|' || coalesce(error, '-') || '|' || cancel_requested || '|' || attempts || '|' ||
 created_ms || '|' || updated_ms || '|' || coalesce(started_ms, '-') || '|' || coalesce(finished_ms, '-')
 FROM jobs ORDER BY id`,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

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

// TestJobsExportBatchKindKeepsEveryJob: the rebuild of jobs for the export_batch kind keeps every row of a
// workspace that already has jobs, of every kind and state, with every column; it accepts export_batch, still
// refuses an unknown kind or state, keeps its indexes and its cascades, and Down gives the old check back.
func TestJobsExportBatchKindKeepsEveryJob(t *testing.T) {
	db, rest := migratedBefore(t)
	ctx := context.Background()

	execAll(
		t,
		db,
		`INSERT INTO projects (id, name, schema_version, document, doc_sha256, width, height, fps_num, fps_den, created_ms, updated_ms, created_by)
 VALUES ('prj_a', 'a', 1, '{}', 'x', 1, 1, 30, 1, 1, 1, 'local:ui')`,
		`INSERT INTO assets (id, kind, size_bytes, original_name, created_ms, updated_ms) VALUES ('sha256:a', 'video', 1, 'a.mp4', 1, 1)`,
		`INSERT INTO jobs (id, kind, project_id, asset_hash, actor, state, progress, message, input, result, error, cancel_requested, attempts, created_ms, updated_ms, started_ms, finished_ms)
 VALUES ('job_1', 'ingest', NULL, 'sha256:a', 'local:ui', 'running', 0.5, 'm', '{"a":1}', NULL, NULL, 0, 1, 10, 11, 12, NULL)`,
		`INSERT INTO jobs (id, kind, project_id, asset_hash, actor, state, progress, message, input, result, error, cancel_requested, attempts, created_ms, updated_ms, started_ms, finished_ms)
 VALUES ('job_2', 'transcribe', 'prj_a', 'sha256:a', 'local:cli', 'queued', 0, '', '{}', NULL, NULL, 1, 0, 20, 20, NULL, NULL)`,
		`INSERT INTO jobs (id, kind, project_id, asset_hash, actor, state, progress, message, input, result, error, cancel_requested, attempts, created_ms, updated_ms, started_ms, finished_ms)
 VALUES ('job_3', 'export', 'prj_a', NULL, 'key:k', 'succeeded', 1, 'done', '{}', '{"export":{}}', NULL, 0, 1, 30, 31, 30, 31)`,
		`INSERT INTO jobs (id, kind, project_id, asset_hash, actor, state, progress, message, input, result, error, cancel_requested, attempts, created_ms, updated_ms, started_ms, finished_ms)
 VALUES ('job_4', 'export', 'prj_a', NULL, 'key:k', 'failed', 0.25, '', '{}', NULL, '{"code":"E_EXPORT_FAILED"}', 0, 3, 40, 41, 40, 41)`,
		`INSERT INTO jobs (id, kind, project_id, asset_hash, actor, state, progress, message, input, result, error, cancel_requested, attempts, created_ms, updated_ms, started_ms, finished_ms)
 VALUES ('job_5', 'frame', 'prj_a', NULL, 'local:ui', 'cancelled', 0, '', '{}', NULL, '{"code":"E_CANCELLED"}', 1, 1, 50, 51, 50, 51)`,
	)

	want := jobRows(t, db)

	ran, err := rest.Up(ctx)
	if err != nil || !slices.Contains(ran, exportBatchMigrationID) {
		t.Fatalf("Up = %v, %v", ran, err)
	}

	if got := jobRows(t, db); !slices.Equal(got, want) {
		t.Errorf("rows after the rebuild:\n%q\nwant\n%q", got, want)
	}

	execAll(t, db, `INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms)
 VALUES ('job_6', 'export_batch', 'prj_a', 'local:ui', '{}', 60, 60)`)

	for name, q := range map[string]string{
		"an unknown kind":  `INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms) VALUES ('job_7', 'nope', 'prj_a', 'a', '{}', 1, 1)`,
		"an unknown state": `INSERT INTO jobs (id, kind, project_id, actor, state, input, created_ms, updated_ms) VALUES ('job_8', 'export', 'prj_a', 'a', 'paused', '{}', 1, 1)`,
		"no owner":         `INSERT INTO jobs (id, kind, actor, input, created_ms, updated_ms) VALUES ('job_9', 'export_batch', 'a', '{}', 1, 1)`,
		"a lost project":   `INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms) VALUES ('job_10', 'export', 'prj_zz', 'a', '{}', 1, 1)`,
	} {
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name IN
 ('jobs_project_id_created_ms_index','jobs_kind_state_index','jobs_active_ingest_unique')`).Scan(&n); err != nil || n != 3 {
		t.Errorf("indexes of jobs after the rebuild = %d, %v; want 3", n, err)
	}

	execAll(t, db, `DELETE FROM projects WHERE id = 'prj_a'`)

	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM jobs WHERE project_id IS NOT NULL`).
		Scan(&n); err != nil ||
		n != 0 {
		t.Errorf("jobs of the deleted project = %d, %v; want 0 (the cascade)", n, err)
	}

	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM jobs`).Scan(&n); err != nil || n != 1 {
		t.Errorf("jobs after the cascade = %d, %v; want the ingest", n, err)
	}
}

// TestJobsExportBatchKindFollowsTheRenamedTimelines: once the table of the timelines is called timelines (the
// project-as-production migration renames projects), the rebuild keeps project_id pointing at it, not at the
// projects of the productions.
func TestJobsExportBatchKindFollowsTheRenamedTimelines(t *testing.T) {
	db, rest := migratedBefore(t)
	ctx := context.Background()

	execAll(
		t,
		db,
		`ALTER TABLE projects RENAME TO timelines`,
		`CREATE TABLE projects (id TEXT NOT NULL PRIMARY KEY)`,
		`INSERT INTO timelines (id, name, schema_version, document, doc_sha256, width, height, fps_num, fps_den, created_ms, updated_ms, created_by)
 VALUES ('prj_b', 'b', 1, '{}', 'x', 1, 1, 30, 1, 1, 1, 'local:ui')`,
		`INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms) VALUES ('job_1', 'export', 'prj_b', 'a', '{}', 1, 1)`,
	)

	if _, err := rest.Up(ctx); err != nil {
		t.Fatal(err)
	}

	var parent string
	if err := db.QueryRowContext(ctx, `SELECT "table" FROM pragma_foreign_key_list('jobs') WHERE "from" = 'project_id'`).
		Scan(&parent); err != nil ||
		parent != "timelines" {
		t.Fatalf("project_id references %q, %v; want timelines", parent, err)
	}

	execAll(
		t,
		db,
		`INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms) VALUES ('job_2', 'export_batch', 'prj_b', 'a', '{}', 2, 2)`,
		`DELETE FROM timelines WHERE id = 'prj_b'`,
	)

	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM jobs`).Scan(&n); err != nil || n != 0 {
		t.Errorf("jobs after the timeline went = %d, %v; want 0", n, err)
	}
}
