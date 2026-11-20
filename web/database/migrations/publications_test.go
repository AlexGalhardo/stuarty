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
		"draft", "waiting_approval", "approved", "scheduled", "publishing", "published", "failed", "missed", "cancelled",
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

		if step[0] == "20261119160000_create_publications_tables" {
			break
		}
	}

	for _, table := range []string{"publications", "publication_targets"} {
		if got := columns(t, db, table); len(got) != 0 {
			t.Errorf("%s still exists after the down: %v", table, got)
		}
	}
}
