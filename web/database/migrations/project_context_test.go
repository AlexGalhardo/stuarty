package migrations

import (
	"context"
	"testing"
)

// A project made before epic 56 has no description and no goal, and the migration invents none (SDD-005 section
// 5.12.2): the columns exist, hold the empty text, and take the context of a new project.
func TestProjectContextColumns(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	execAll(t, db, `INSERT INTO projects (id, name, created_ms, updated_ms, created_by)
 VALUES ('prj_0000000001', 'Old project', 1, 1, 'u')`)

	var description, specific, due string

	err := db.QueryRowContext(ctx, `SELECT description, goal_specific, goal_due FROM projects WHERE id = 'prj_0000000001'`).
		Scan(&description, &specific, &due)
	if err != nil {
		t.Fatal(err)
	}

	if description != "" || specific != "" || due != "" {
		t.Fatalf("the migration invented a context: %q %q %q", description, specific, due)
	}

	execAll(t, db, `UPDATE projects SET description = 'd', goal_specific = 's', goal_measurable = 'm',
 goal_achievable = 'a', goal_relevant = 'r', goal_time_bound = 't', goal_due = '2027-01-01'
 WHERE id = 'prj_0000000001'`)

	if n := count(t, db, `SELECT count(*) FROM projects WHERE goal_due = '2027-01-01'`); n != 1 {
		t.Fatalf("the context was not stored: %d", n)
	}
}
