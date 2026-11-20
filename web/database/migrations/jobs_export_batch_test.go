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

const (
	exportBatchMigrationID = "20261119120050_jobs_export_batch_kind"
	waitingApprovalID      = "20261119140100_jobs_waiting_approval"
)

// migrations are the vukem queue tables and the editor's, in order.
func migrations() []schema.Migration {
	qopts := queue.DatabaseOptions{Table: "queue_jobs", FailedTable: "queue_failed_jobs"}

	return slices.Concat(queue.Migrations(qopts), All())
}

// openWith is a new workspace with the migrations that keep returns true for.
func openWith(t *testing.T, keep func(id string) bool) *database.DB {
	t.Helper()

	ctx := context.Background()

	db, err := database.Open(ctx, "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "db.sqlite")))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	upTo(t, db, keep)

	return db
}

// upTo runs, on db, the migrations that keep returns true for and that have not run.
func upTo(t *testing.T, db *database.DB, keep func(id string) bool) []string {
	t.Helper()

	some := slices.DeleteFunc(migrations(), func(m schema.Migration) bool { return !keep(m.ID) })

	m, err := schema.NewMigrator(db, some)
	if err != nil {
		t.Fatal(err)
	}

	ran, err := m.Up(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	return ran
}

func before(id string) func(string) bool { return func(m string) bool { return m < id } }

func through(id string) func(string) bool { return func(m string) bool { return m <= id } }

func every(string) bool { return true }

func execAll(t *testing.T, db *database.DB, qs ...string) {
	t.Helper()

	for _, q := range qs {
		if _, err := db.ExecContext(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

// timelineTable is the table that holds the timeline documents: projects before the project-as-production
// migration renames it, timelines after.
func timelineTable(t *testing.T, db *database.DB) string {
	t.Helper()

	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'timelines'`).Scan(&n); err != nil {
		t.Fatal(err)
	}

	if n == 1 {
		return "timelines"
	}

	return "projects"
}

func insertTimeline(t *testing.T, db *database.DB, id string) {
	t.Helper()

	execAll(t, db, `INSERT INTO `+timelineTable(t, db)+` (id, name, schema_version, document, doc_sha256, width, height,
 fps_num, fps_den, created_ms, updated_ms, created_by) VALUES ('`+id+`', 'a', 1, '{}', 'x', 1, 1, 30, 1, 1, 1, 'local:ui')`)
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

// insertEveryJob writes a job of every kind and every state with every column set.
func insertEveryJob(t *testing.T, db *database.DB) {
	t.Helper()

	cols := `(id, kind, project_id, asset_hash, actor, state, progress, message, input, result, error, cancel_requested,
 attempts, created_ms, updated_ms, started_ms, finished_ms)`

	execAll(
		t,
		db,
		`INSERT INTO assets (id, kind, size_bytes, original_name, created_ms, updated_ms) VALUES ('sha256:a', 'video', 1, 'a.mp4', 1, 1)`,
		`INSERT INTO jobs `+cols+` VALUES ('job_1', 'ingest', NULL, 'sha256:a', 'local:ui', 'running', 0.5, 'm', '{"a":1}', NULL, NULL, 0, 1, 10, 11, 12, NULL)`,
		`INSERT INTO jobs `+cols+` VALUES ('job_2', 'transcribe', 'prj_a', 'sha256:a', 'local:cli', 'queued', 0, '', '{}', NULL, NULL, 1, 0, 20, 20, NULL, NULL)`,
		`INSERT INTO jobs `+cols+` VALUES ('job_3', 'export', 'prj_a', NULL, 'key:k', 'succeeded', 1, 'done', '{}', '{"export":{}}', NULL, 0, 1, 30, 31, 30, 31)`,
		`INSERT INTO jobs `+cols+` VALUES ('job_4', 'export', 'prj_a', NULL, 'key:k', 'failed', 0.25, '', '{}', NULL, '{"code":"E_EXPORT_FAILED"}', 0, 3, 40, 41, 40, 41)`,
		`INSERT INTO jobs `+cols+` VALUES ('job_5', 'frame', 'prj_a', NULL, 'local:ui', 'cancelled', 0, '', '{}', NULL, '{"code":"E_CANCELLED"}', 1, 1, 50, 51, 50, 51)`,
	)
}

// TestJobsExportBatchKindKeepsEveryJob: the rebuild of jobs for the export_batch kind keeps every row of a
// workspace that already has jobs, of every kind and state, with every column; it accepts export_batch, still
// refuses an unknown kind or state, keeps its indexes and its cascades, and points project_id at the table of the
// timelines (timelines once the project-as-production migration renamed it).
func TestJobsExportBatchKindKeepsEveryJob(t *testing.T) {
	db := openWith(t, before(exportBatchMigrationID))
	ctx := context.Background()

	insertTimeline(t, db, "prj_a")
	insertEveryJob(t, db)

	want := jobRows(t, db)

	if ran := upTo(t, db, through(exportBatchMigrationID)); !slices.Equal(ran, []string{exportBatchMigrationID}) {
		t.Fatalf("ran %v", ran)
	}

	if got := jobRows(t, db); !slices.Equal(got, want) {
		t.Errorf("rows after the rebuild:\n%q\nwant\n%q", got, want)
	}

	var parent string
	if err := db.QueryRowContext(ctx, `SELECT "table" FROM pragma_foreign_key_list('jobs') WHERE "from" = 'project_id'`).
		Scan(&parent); err != nil ||
		parent != timelineTable(t, db) {
		t.Fatalf("project_id references %q, %v; want %s", parent, err, timelineTable(t, db))
	}

	execAll(t, db, `INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms)
 VALUES ('job_6', 'export_batch', 'prj_a', 'local:ui', '{}', 60, 60)`)

	for name, q := range map[string]string{
		"an unknown kind":  `INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms) VALUES ('job_7', 'nope', 'prj_a', 'a', '{}', 1, 1)`,
		"an unknown state": `INSERT INTO jobs (id, kind, project_id, actor, state, input, created_ms, updated_ms) VALUES ('job_8', 'export', 'prj_a', 'a', 'paused', '{}', 1, 1)`,
		"no owner":         `INSERT INTO jobs (id, kind, actor, input, created_ms, updated_ms) VALUES ('job_9', 'export_batch', 'a', '{}', 1, 1)`,
		"a lost timeline":  `INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms) VALUES ('job_10', 'export', 'prj_zz', 'a', '{}', 1, 1)`,
	} {
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	if n := count(t, db, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name IN
 ('jobs_project_id_created_ms_index','jobs_kind_state_index','jobs_active_ingest_unique')`); n != 3 {
		t.Errorf("indexes of jobs after the rebuild = %d; want 3", n)
	}

	execAll(t, db, `DELETE FROM `+timelineTable(t, db)+` WHERE id = 'prj_a'`)

	if n := count(t, db, `SELECT count(*) FROM jobs WHERE project_id IS NOT NULL`); n != 0 {
		t.Errorf("jobs of the deleted timeline = %d; want 0 (the cascade)", n)
	}

	if n := count(t, db, `SELECT count(*) FROM jobs`); n != 1 {
		t.Errorf("jobs after the cascade = %d; want the ingest", n)
	}
}

// TestJobsMigrationsKeepEveryJobInAnyOrder: the two rebuilds of jobs, the export_batch kind and the approval queue
// (which drops the closed list of kinds and adds approval_id and waiting_approval), keep every row whichever ran
// first. In ID order the export_batch rebuild runs before; a workspace that already ran the approval rebuild runs
// it out of order, and it leaves the table alone. Down of the approval rebuild keeps the export batches.
func TestJobsMigrationsKeepEveryJobInAnyOrder(t *testing.T) {
	ctx := context.Background()

	t.Run("in order", func(t *testing.T) {
		db := openWith(t, before(exportBatchMigrationID))
		insertTimeline(t, db, "prj_a")
		insertEveryJob(t, db)
		want := jobRows(t, db)

		ran := upTo(t, db, every)
		if !slices.Contains(ran, exportBatchMigrationID) || !slices.Contains(ran, waitingApprovalID) ||
			slices.Index(ran, exportBatchMigrationID) > slices.Index(ran, waitingApprovalID) {
			t.Fatalf("ran %v", ran)
		}

		if got := jobRows(t, db); !slices.Equal(got, want) {
			t.Errorf("rows after both rebuilds:\n%q\nwant\n%q", got, want)
		}

		execAll(
			t,
			db,
			`INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms) VALUES ('job_6', 'export_batch', 'prj_a', 'a', '{}', 1, 1)`,
			`INSERT INTO jobs (id, kind, project_id, actor, state, input, created_ms, updated_ms, approval_id) VALUES ('job_7', 'publish.post', 'prj_a', 'a', 'waiting_approval', '{}', 1, 1, 'apr_0000000001')`,
		)

		// Down of the approval rebuild gives back the closed list of kinds, export_batch included: the batch stays.
		if err := m20261119140100JobsWaitingApproval().Down(schema.New(ctx, db)); err != nil {
			t.Fatal(err)
		}

		if n := count(t, db, `SELECT count(*) FROM jobs WHERE kind = 'export_batch'`); n != 1 {
			t.Errorf("export batches after Down of the approval rebuild = %d; want 1", n)
		}

		if n := count(t, db, `SELECT count(*) FROM jobs`); n != len(want)+1 {
			t.Errorf(
				"jobs after Down of the approval rebuild = %d; want %d (only the waiting one goes)",
				n,
				len(want)+1,
			)
		}

		execAll(
			t,
			db,
			`INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms) VALUES ('job_8', 'export_batch', 'prj_a', 'a', '{}', 1, 1)`,
		)
	})

	t.Run("approval first", func(t *testing.T) {
		db := openWith(t, func(id string) bool { return id != exportBatchMigrationID })
		insertTimeline(t, db, "prj_a")
		insertEveryJob(t, db)
		execAll(
			t,
			db,
			`INSERT INTO jobs (id, kind, project_id, actor, state, input, created_ms, updated_ms, approval_id)
 VALUES ('job_9', 'publish.post', 'prj_a', 'a', 'waiting_approval', '{}', 1, 1, 'apr_0000000001')`,
		)

		want := jobRows(t, db)
		sql := func() string {
			var s string
			if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name = 'jobs'`).
				Scan(&s); err != nil {
				t.Fatal(err)
			}

			return s
		}
		was := sql()

		if ran := upTo(t, db, every); !slices.Equal(ran, []string{exportBatchMigrationID}) {
			t.Fatalf("ran %v", ran)
		}

		if got := jobRows(t, db); !slices.Equal(got, want) {
			t.Errorf("rows after the out-of-order rebuild:\n%q\nwant\n%q", got, want)
		}

		if sql() != was {
			t.Errorf("the table was rebuilt:\n%s\nwas\n%s", sql(), was)
		}

		if n := count(t, db, `SELECT count(*) FROM jobs WHERE approval_id = 'apr_0000000001'`); n != 1 {
			t.Errorf("approval_id after the out-of-order rebuild: %d rows; want 1", n)
		}

		execAll(
			t,
			db,
			`INSERT INTO jobs (id, kind, project_id, actor, input, created_ms, updated_ms) VALUES ('job_10', 'export_batch', 'prj_a', 'a', '{}', 1, 1)`,
		)
	})
}
