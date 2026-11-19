package migrations

import (
	"context"
	"testing"
)

// The tables of the calendar (SDD-005 section 5.15): a publication leaves with its project, a target leaves
// with its publication, and a target holds only the states of SDD-020 section 24.
func TestPublicationsTablesCascadeAndCheck(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	execAll(
		t,
		db,
		`INSERT INTO projects (id, name, created_ms, updated_ms, created_by) VALUES ('prj_a', 'a', 1, 1, 'u')`,
		`INSERT INTO publications (id, project_id, title, export_job, created_ms, updated_ms, created_by)
 VALUES ('pub_a', 'prj_a', 't', 'job_1', 1, 1, 'u')`,
		`INSERT INTO publication_targets (id, publication_id, position, connection_id, network, fields, state, created_ms, updated_ms)
 VALUES ('tgt_a', 'pub_a', 0, 'con_1', 'youtube', '{}', 'draft', 1, 1)`,
	)

	for name, q := range map[string]string{
		"a state that is not one": `INSERT INTO publication_targets (id, publication_id, position, connection_id, network, fields, state, created_ms, updated_ms)
 VALUES ('tgt_b', 'pub_a', 1, 'con_1', 'youtube', '{}', 'sent', 1, 1)`,
		"a late window over a day": `INSERT INTO publication_targets (id, publication_id, position, connection_id, network, fields, state, late_minutes, created_ms, updated_ms)
 VALUES ('tgt_c', 'pub_a', 1, 'con_1', 'youtube', '{}', 'draft', 1441, 1, 1)`,
		"a publication of no project": `INSERT INTO publications (id, project_id, title, export_job, created_ms, updated_ms, created_by)
 VALUES ('pub_b', 'prj_none', 't', 'job_1', 1, 1, 'u')`,
	} {
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	for _, s := range []string{
		"draft", "waiting_approval", "approved", "scheduled", "publishing", "published", "sent_as_draft", "failed",
		"missed", "cancelled",
	} {
		if _, err := db.ExecContext(ctx, `UPDATE publication_targets SET state = ? WHERE id = 'tgt_a'`, s); err != nil {
			t.Errorf("state %s: %v", s, err)
		}
	}

	execAll(t, db, `DELETE FROM projects WHERE id = 'prj_a'`)

	if n := count(
		t,
		db,
		`SELECT count(*) FROM publications`,
	) + count(
		t,
		db,
		`SELECT count(*) FROM publication_targets`,
	); n != 0 {
		t.Errorf("rows left after the project went: %d", n)
	}
}

// The migration that adds sent_as_draft keeps every row and every index, and its down turns a draft back into
// a published target, since the older table has no such state (SDD-005 section 5.15).
func TestSentAsDraftMigrationKeepsRowsAndIndexesBothWays(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	execAll(
		t,
		db,
		`INSERT INTO projects (id, name, created_ms, updated_ms, created_by) VALUES ('prj_a', 'a', 1, 1, 'u')`,
		`INSERT INTO publications (id, project_id, title, export_job, created_ms, updated_ms, created_by)
 VALUES ('pub_a', 'prj_a', 't', 'job_1', 1, 1, 'u')`,
		`INSERT INTO publication_targets (id, publication_id, position, connection_id, network, fields, state, remote_id, created_ms, updated_ms)
 VALUES ('tgt_a', 'pub_a', 0, 'con_1', 'tiktok', '{}', 'sent_as_draft', 'v_inbox_file~v2.1', 1, 1),
        ('tgt_b', 'pub_a', 1, 'con_1', 'x', '{}', 'published', '1234', 1, 1)`,
	)

	for _, idx := range []string{
		"publication_targets_publication_id_index", "publication_targets_state_scheduled_ms_index",
		"publication_targets_approval_id_index",
	} {
		if n := count(t, db, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = '`+idx+`'`); n != 1 {
			t.Errorf("the index %s is gone", idx)
		}
	}

	m, err := newMigrator(db)
	if err != nil {
		t.Fatal(err)
	}

	for {
		step, rerr := m.Rollback(ctx, 1)
		if rerr != nil || len(step) != 1 {
			t.Fatalf("Rollback = %v, %v", step, rerr)
		}

		if step[0] == "20261120100000_publication_targets_sent_as_draft" {
			break
		}
	}

	if n := count(t, db, `SELECT count(*) FROM publication_targets WHERE state = 'published'`); n != 2 {
		t.Errorf("after the down %d targets are published, want 2 (the draft is published again)", n)
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE publication_targets SET state = 'sent_as_draft' WHERE id = 'tgt_a'`,
	); err == nil {
		t.Error("the older table accepted sent_as_draft")
	}

	if n := count(
		t,
		db,
		`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name LIKE 'publication_targets_%'`,
	); n != 3 {
		t.Errorf("after the down %d indexes are left, want 3", n)
	}

	if _, err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE publication_targets SET state = 'sent_as_draft' WHERE id = 'tgt_a'`,
	); err != nil {
		t.Errorf("after the up again: %v", err)
	}
}

func TestPublicationsMigrationDownDropsBothTables(t *testing.T) {
	db := openMigrated(t)

	m, err := newMigrator(db)
	if err != nil {
		t.Fatal(err)
	}

	// Later migrations of other epics come off first.
	for {
		step, rerr := m.Rollback(context.Background(), 1)
		if rerr != nil || len(step) != 1 {
			t.Fatalf("Rollback = %v, %v", step, rerr)
		}

		if step[0] == "20261119250000_create_publications_tables" {
			break
		}
	}

	for _, table := range []string{"publications", "publication_targets"} {
		if got := columns(t, db, table); len(got) != 0 {
			t.Errorf("%s still exists after the down: %v", table, got)
		}
	}
}
