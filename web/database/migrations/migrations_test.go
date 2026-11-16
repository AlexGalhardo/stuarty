package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem/database"
	"github.com/AlexGalhardo/vukem/queue"
	"github.com/AlexGalhardo/vukem/schema"
)

func openMigrated(t *testing.T) *database.DB {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "db.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	qopts := queue.DatabaseOptions{Table: "queue_jobs", FailedTable: "queue_failed_jobs"}
	m, err := schema.NewMigrator(db, slices.Concat(queue.Migrations(qopts), All()))
	if err != nil {
		t.Fatal(err)
	}
	ran, err := m.Up(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1+len(All()) {
		t.Fatalf("ran %d migrations, want %d", len(ran), 1+len(All()))
	}
	again, err := m.Up(ctx)
	if err != nil || len(again) != 0 {
		t.Fatalf("second Up = %v, %v; want nothing to migrate", again, err)
	}
	return db
}

// column is "name type notnull default pk" with - for no default.
func columns(t *testing.T, db *database.DB, table string) []string {
	t.Helper()
	rows, err := db.QueryContext(
		context.Background(),
		"SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info(?)",
		table,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var (
			name, typ string
			notNull   int
			dflt      sql.NullString
			pk        int
		)
		if err := rows.Scan(&name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		d := "-"
		if dflt.Valid {
			d = dflt.String
		}
		out = append(out, fmt.Sprintf("%s %s %d %s %d", name, typ, notNull, d, pk))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTablesMatchSDD005Section5(t *testing.T) {
	db := openMigrated(t)

	want := map[string][]string{
		"projects": {
			"id TEXT 1 - 1", "name TEXT 1 - 0", "state TEXT 1 'active' 0", "revision INTEGER 1 0 0",
			"schema_version INTEGER 1 - 0", "document TEXT 1 - 0", "doc_sha256 TEXT 1 - 0",
			"width INTEGER 1 - 0", "height INTEGER 1 - 0", "fps_num INTEGER 1 - 0", "fps_den INTEGER 1 - 0",
			"duration_frames INTEGER 1 0 0", "track_count INTEGER 1 0 0", "item_count INTEGER 1 0 0",
			"asset_count INTEGER 1 0 0", "resync_floor INTEGER 1 0 0", "created_ms INTEGER 1 - 0",
			"updated_ms INTEGER 1 - 0", "created_by TEXT 1 - 0", "owner_user_id TEXT 0 - 0",
			"trashed_ms INTEGER 0 - 0", "trashed_by TEXT 0 - 0",
		},
		"operations": {
			"project_id TEXT 1 - 1", "revision INTEGER 1 - 2", "kind TEXT 1 - 0", "actor TEXT 1 - 0",
			"base_revision INTEGER 1 - 0", "target_revision INTEGER 0 - 0", "at_ms INTEGER 1 - 0",
			"ops TEXT 1 - 0", "inverse TEXT 1 - 0", "diff TEXT 1 - 0", "skipped TEXT 0 - 0",
			"schema_version INTEGER 1 - 0", "doc_sha256 TEXT 1 - 0", "closed_by INTEGER 0 - 0",
			"idempotency_key TEXT 0 - 0",
		},
		"idempotency_keys": {
			"project_id TEXT 1 - 1", "actor TEXT 1 - 2", "key TEXT 1 - 3", "request_hash TEXT 1 - 0",
			"revision INTEGER 1 - 0", "result TEXT 1 - 0", "created_ms INTEGER 1 - 0", "expires_ms INTEGER 1 - 0",
		},
		"assets": {
			"id TEXT 1 - 1",
			"kind TEXT 0 - 0",
			"size_bytes INTEGER 1 - 0",
			"original_name TEXT 1 - 0",
			"ingest_state TEXT 1 'pending' 0",
			"ingest_progress REAL 1 0 0",
			"ingest_error TEXT 0 - 0",
			"normalized_version INTEGER 0 - 0",
			"duration_ms INTEGER 0 - 0",
			"width INTEGER 0 - 0",
			"height INTEGER 0 - 0",
			"has_audio INTEGER 1 0 0",
			"frame_rate_num INTEGER 0 - 0",
			"frame_rate_den INTEGER 0 - 0",
			"frame_count INTEGER 0 - 0",
			"sample_count INTEGER 0 - 0",
			"created_ms INTEGER 1 - 0",
			"updated_ms INTEGER 1 - 0",
			"frame_delays TEXT 0 - 0",
			"has_alpha INTEGER 1 0 0",
		},
		"project_assets": {
			"project_id TEXT 1 - 1", "asset_id TEXT 1 - 2", "asset_hash TEXT 1 - 0", "state TEXT 1 - 0",
			"name TEXT 1 - 0", "snapshot TEXT 0 - 0", "imported_by TEXT 1 - 0", "imported_ms INTEGER 1 - 0",
			"trashed_by TEXT 0 - 0", "trashed_ms INTEGER 0 - 0",
		},
		"artifacts": {
			"asset_hash TEXT 1 - 1", "kind TEXT 1 - 2", "profile INTEGER 1 - 3", "variant TEXT 1 '' 4",
			"ext TEXT 1 - 5", "rel_path TEXT 1 - 0", "size_bytes INTEGER 1 - 0", "sha256 TEXT 1 - 0",
			"meta TEXT 0 - 0", "created_ms INTEGER 1 - 0",
		},
		"jobs": {
			"id TEXT 1 - 1", "kind TEXT 1 - 0", "project_id TEXT 0 - 0", "asset_hash TEXT 0 - 0",
			"actor TEXT 1 - 0", "state TEXT 1 'queued' 0", "progress REAL 1 0 0", "message TEXT 1 '' 0",
			"input TEXT 1 - 0", "result TEXT 0 - 0", "error TEXT 0 - 0", "cancel_requested INTEGER 1 0 0",
			"attempts INTEGER 1 0 0", "created_ms INTEGER 1 - 0", "updated_ms INTEGER 1 - 0",
			"started_ms INTEGER 0 - 0", "finished_ms INTEGER 0 - 0",
		},
		"templates": {
			"id TEXT 1 - 1", "name TEXT 1 - 0", "description TEXT 1 '' 0", "schema_version INTEGER 1 - 0",
			"document TEXT 1 - 0", "doc_sha256 TEXT 1 - 0", "slots TEXT 1 - 0", "width INTEGER 1 - 0",
			"height INTEGER 1 - 0", "fps_num INTEGER 1 - 0", "fps_den INTEGER 1 - 0",
			"duration_frames INTEGER 1 0 0", "source_project_id TEXT 0 - 0", "created_ms INTEGER 1 - 0",
			"created_by TEXT 1 - 0", "owner_user_id TEXT 0 - 0",
		},
		"document_backups": {
			"id INTEGER 0 - 1", "owner_kind TEXT 1 - 0", "owner_id TEXT 1 - 0", "revision INTEGER 1 - 0",
			"schema_version INTEGER 1 - 0", "document TEXT 1 - 0", "reason TEXT 1 - 0", "created_ms INTEGER 1 - 0",
		},
		"audit": {
			"id INTEGER 0 - 1", "at_ms INTEGER 1 - 0", "actor TEXT 1 - 0", "action TEXT 1 - 0",
			"project_id TEXT 0 - 0", "subject_id TEXT 0 - 0", "details TEXT 0 - 0",
		},
		"api_keys": {
			"id TEXT 1 - 1", "token_id TEXT 1 - 0", "user_id TEXT 1 - 0", "project_id TEXT 0 - 0",
			"name TEXT 1 - 0", "created_ms INTEGER 1 - 0",
		},
		"brand_kits": {
			"owner_user_id TEXT 1 - 1", "document TEXT 1 - 0", "updated_ms INTEGER 1 - 0", "updated_by TEXT 1 - 0",
		},
		"font_owners": {
			"hash TEXT 1 - 1", "owner_user_id TEXT 1 - 2", "created_ms INTEGER 1 - 0",
		},
	}
	for table, w := range want {
		if got := columns(t, db, table); !slices.Equal(got, w) {
			t.Errorf("table %s columns:\n got  %q\n want %q", table, got, w)
		}
	}
}

func TestIndexesAndTriggers(t *testing.T) {
	db := openMigrated(t)

	// name -> fragments the stored SQL must contain.
	want := map[string][]string{
		"projects_state_updated_ms_index":             {"ON projects (state, updated_ms)"},
		"operations_project_id_actor_revision_index":  {"ON operations (project_id, actor, revision)"},
		"operations_undo_idx":                         {"WHERE closed_by IS NULL AND kind IN ('edit','redo')"},
		"operations_redo_idx":                         {"WHERE closed_by IS NULL AND kind = 'undo'"},
		"idempotency_keys_expires_ms_index":           {"ON idempotency_keys (expires_ms)"},
		"project_assets_project_id_asset_hash_unique": {"UNIQUE INDEX", "(project_id, asset_hash)"},
		"project_assets_asset_hash_index":             {"ON project_assets (asset_hash)"},
		"jobs_project_id_created_ms_index":            {"ON jobs (project_id, created_ms)"},
		"jobs_kind_state_index":                       {"ON jobs (kind, state)"},
		"jobs_active_ingest_unique": {
			"UNIQUE INDEX",
			"WHERE kind = 'ingest' AND state IN ('queued','running')",
		},
		"templates_created_ms_index":                 {"ON templates (created_ms)"},
		"templates_owner_user_id_created_ms_index":   {"ON templates (owner_user_id, created_ms)"},
		"document_backups_owner_kind_owner_id_index": {"ON document_backups (owner_kind, owner_id)"},
		"audit_project_id_at_ms_index":               {"ON audit (project_id, at_ms)"},
		"audit_at_ms_index":                          {"ON audit (at_ms)"},
		"operations_append_only":                     {"BEFORE UPDATE ON operations"},
		"audit_no_update":                            {"BEFORE UPDATE ON audit"},
		"audit_no_delete":                            {"BEFORE DELETE ON audit"},
		"api_keys_token_id_unique":                   {"UNIQUE INDEX", "ON api_keys (token_id)"},
		"api_keys_user_id_index":                     {"ON api_keys (user_id)"},
	}
	for name, fragments := range want {
		var sqlText string
		err := db.QueryRowContext(context.Background(), "SELECT sql FROM sqlite_master WHERE name = ?", name).
			Scan(&sqlText)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, f := range fragments {
			if !strings.Contains(sqlText, f) {
				t.Errorf("%s: SQL %q lacks %q", name, sqlText, f)
			}
		}
	}
}

func TestChecksAndForeignKeys(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	exec := func(q string, args ...any) error {
		_, err := db.ExecContext(ctx, q, args...)
		return err
	}
	if err := exec(
		`INSERT INTO projects (id, name, schema_version, document, doc_sha256, width, height, fps_num, fps_den, created_ms, updated_ms, created_by)
 VALUES ('prj_a', 'a', 1, '{}', 'x', 1, 1, 30, 1, 1, 1, 'local:ui')`,
	); err != nil {
		t.Fatal(err)
	}

	bad := map[string]string{
		"project state":     `UPDATE projects SET state = 'gone' WHERE id = 'prj_a'`,
		"operation kind":    `INSERT INTO operations VALUES ('prj_a', 1, 'nope', 'a', 0, NULL, 1, '[]', '[]', '[]', NULL, 1, 'x', NULL, NULL)`,
		"revision zero":     `INSERT INTO operations VALUES ('prj_a', 0, 'edit', 'a', 0, NULL, 1, '[]', '[]', '[]', NULL, 1, 'x', NULL, NULL)`,
		"orphan operation":  `INSERT INTO operations VALUES ('prj_zz', 1, 'edit', 'a', 0, NULL, 1, '[]', '[]', '[]', NULL, 1, 'x', NULL, NULL)`,
		"asset kind":        `INSERT INTO assets (id, kind, size_bytes, original_name, created_ms, updated_ms) VALUES ('sha256:a', 'doc', 1, 'n', 1, 1)`,
		"ingest progress":   `INSERT INTO assets (id, ingest_progress, size_bytes, original_name, created_ms, updated_ms) VALUES ('sha256:b', 2, 1, 'n', 1, 1)`,
		"job without owner": `INSERT INTO jobs (id, kind, actor, input, created_ms, updated_ms) VALUES ('job_a', 'export', 'a', '{}', 1, 1)`,
		"backup owner kind": `INSERT INTO document_backups (owner_kind, owner_id, revision, schema_version, document, reason, created_ms) VALUES ('x', 'a', 0, 1, '{}', 'r', 1)`,
	}
	for name, q := range bad {
		if err := exec(q); err == nil {
			t.Errorf("%s: the database accepted %s", name, q)
		}
	}

	if err := exec(
		`INSERT INTO operations VALUES ('prj_a', 1, 'edit', 'a', 0, NULL, 1, '[]', '[]', '[]', NULL, 1, 'x', NULL, NULL)`,
	); err != nil {
		t.Fatal(err)
	}
	if err := exec(`DELETE FROM projects WHERE id = 'prj_a'`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM operations").Scan(&n); err != nil || n != 0 {
		t.Errorf("operations after the cascade = %d, %v; want 0", n, err)
	}
}

// SDD-005 section 18 check 36: the vukem queue tables exist, and the table
// named jobs is the editor's.
func TestQueueTablesAndEditorJobs(t *testing.T) {
	db := openMigrated(t)
	for _, name := range []string{"queue_jobs", "queue_failed_jobs", "jobs"} {
		var n int
		if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).
			Scan(&n); err != nil ||
			n != 1 {
			t.Errorf("table %s: %d, %v", name, n, err)
		}
	}
	got := columns(t, db, "jobs")
	if len(got) == 0 || !strings.HasPrefix(got[1], "kind TEXT") {
		t.Errorf("the jobs table is not the editor table: %q", got)
	}
}

// Every Down must undo its Up: Reset runs them all and leaves no editor table
// behind, and the migrations can then run again on the same database.
func TestResetDropsEveryEditorTableAndUpRunsAgain(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	m, err := schema.NewMigrator(db, slices.Concat(queue.Migrations(queue.DatabaseOptions{
		Table: "queue_jobs", FailedTable: "queue_failed_jobs",
	}), All()))
	if err != nil {
		t.Fatal(err)
	}

	tables := []string{
		"projects",
		"operations",
		"idempotency_keys",
		"assets",
		"jobs",
		"templates",
		"document_backups",
		"audit",
		"api_keys",
		"brand_kits",
		"font_owners",
	}
	for _, table := range tables {
		if len(columns(t, db, table)) == 0 {
			t.Fatalf("table %s does not exist after Up: the list of this test is stale", table)
		}
	}

	rolled, err := m.Reset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rolled) != 1+len(All()) {
		t.Fatalf("Reset rolled back %d migrations, want %d", len(rolled), 1+len(All()))
	}

	for _, table := range tables {
		if got := columns(t, db, table); len(got) != 0 {
			t.Errorf("table %s still has columns after Reset: %v", table, got)
		}
	}

	again, err := m.Up(ctx)
	if err != nil || len(again) != 1+len(All()) {
		t.Fatalf("Up after Reset = %v, %v; want all %d migrations", again, err, 1+len(All()))
	}
}

func TestOperationsAppendOnly(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO projects (id, name, schema_version, document, doc_sha256, width, height, fps_num, fps_den, created_ms, updated_ms, created_by)
 VALUES ('prj_a', 'a', 1, '{}', 'x', 1, 1, 30, 1, 1, 1, 'local:ui')`,
		`INSERT INTO operations VALUES ('prj_a', 1, 'edit', 'a', 0, NULL, 1, '[]', '[]', '[]', NULL, 1, 'x', NULL, NULL)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	// SDD-005 section 5.3: DELETE is not guarded because the cascade of a hard
	// project delete needs it; every UPDATE that rewrites the record is.
	for _, q := range []string{
		`UPDATE operations SET diff = '[1]'`,
		`UPDATE operations SET actor = 'x', closed_by = 2`,
		`UPDATE operations SET closed_by = NULL`,
	} {
		if _, err := db.ExecContext(ctx, q); err == nil || !strings.Contains(err.Error(), "operations is append-only") {
			t.Errorf("%s: err = %v", q, err)
		}
	}
}

func TestAuditAppendOnly(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO audit (at_ms, actor, action) VALUES (1, 'local:ui', 'project.trash')`,
	); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE audit SET actor = 'x'`, `DELETE FROM audit`} {
		if _, err := db.ExecContext(ctx, q); err == nil || !strings.Contains(err.Error(), "audit is append-only") {
			t.Errorf("%s: err = %v", q, err)
		}
	}
}

func TestAPIKeysCascadeWithProject(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO projects (id, name, schema_version, document, doc_sha256, width, height, fps_num, fps_den, created_ms, updated_ms, created_by)
 VALUES ('prj_a', 'a', 1, '{}', 'x', 1, 1, 30, 1, 1, 1, 'u')`,
		`INSERT INTO api_keys VALUES ('key_1', 'tok_1', 'usr_1', 'prj_a', 'k', 1)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO api_keys VALUES ('key_2', 'tok_1', 'usr_1', NULL, 'k', 1)`,
	); err == nil {
		t.Error("a second key with the same token_id was accepted")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM projects WHERE id = 'prj_a'`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM api_keys").Scan(&n); err != nil || n != 0 {
		t.Errorf("api_keys after the cascade = %d, %v", n, err)
	}
}

func TestArtifactsAcceptSpeechAndBeats(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO assets (id, size_bytes, original_name, created_ms, updated_ms) VALUES ('sha256:a', 1, 'n', 1, 1)`,
	); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"probe", "speech", "beats", "frames", "chunks"} {
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO artifacts (asset_hash, kind, profile, ext, rel_path, size_bytes, sha256, created_ms) VALUES ('sha256:a', ?, 1, 'json', 'p', 1, 'x', 1)`,
			kind,
		); err != nil {
			t.Errorf("kind %q refused: %v", kind, err)
		}
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO artifacts (asset_hash, kind, profile, ext, rel_path, size_bytes, sha256, created_ms) VALUES ('sha256:a', 'other', 1, 'json', 'p', 1, 'x', 1)`,
	); err == nil {
		t.Error("an unknown artifact kind was accepted")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM assets WHERE id = 'sha256:a'`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM artifacts").Scan(&n); err != nil || n != 0 {
		t.Errorf("artifacts after the cascade = %d, %v", n, err)
	}
}

// TASK-0254: an asset may be of kind lut; the other kinds stay closed, and the rows and cascades that
// hang from assets are untouched by the change of the check.
func TestAssetsAcceptLutKind(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	exec := func(q string, args ...any) error {
		_, err := db.ExecContext(ctx, q, args...)
		return err
	}

	if err := exec(
		`INSERT INTO assets (id, kind, size_bytes, original_name, created_ms, updated_ms) VALUES ('sha256:lut', 'lut', 1, 'a.cube', 1, 1)`,
	); err != nil {
		t.Errorf("kind lut refused: %v", err)
	}

	if err := exec(
		`INSERT INTO assets (id, kind, size_bytes, original_name, created_ms, updated_ms) VALUES ('sha256:doc', 'doc', 1, 'n', 1, 1)`,
	); err == nil {
		t.Error("an unknown asset kind was accepted")
	}

	if err := exec(
		`INSERT INTO artifacts (asset_hash, kind, profile, ext, rel_path, size_bytes, sha256, created_ms) VALUES ('sha256:lut', 'probe', 1, 'json', 'p', 1, 'x', 1)`,
	); err != nil {
		t.Fatal(err)
	}

	if err := exec(`DELETE FROM assets WHERE id = 'sha256:lut'`); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM artifacts").Scan(&n); err != nil || n != 0 {
		t.Errorf("artifacts after the cascade = %d, %v", n, err)
	}
}
