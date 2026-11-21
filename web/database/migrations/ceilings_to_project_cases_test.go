package migrations

import (
	"context"
	"reflect"
	"testing"

	"github.com/AlexGalhardo/vukem/database"
	"github.com/AlexGalhardo/vukem/schema"
)

// upToCeilings migrates a new database to the release before the ceilings migration.
func upToCeilings(t *testing.T, db *database.DB) {
	t.Helper()

	var upTo []schema.Migration

	for _, m := range allMigrations() {
		if m.ID < "20261121184500_ceilings_to_project" {
			upTo = append(upTo, m)
		}
	}

	old, err := schema.NewMigrator(db, upTo)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := old.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// D-132 in network mode: a ceiling goes to the project that holds its timeline and to no project of another user; an
// id that names a project stays on it even when a timeline of another user carries the same id; and a project in the
// trash keeps what was written for it.
func TestCeilingsToProject_OwnershipDoesNotCrossUsers(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	upToCeilings(t, db)

	execAll(
		t,
		db,
		`INSERT INTO users (id, email, password, role, created_ms, updated_ms) VALUES
 ('usr_a', 'a@example.test', 'x', 'member', 1, 1), ('usr_b', 'b@example.test', 'x', 'member', 1, 1)`,
		`INSERT INTO projects (id, name, state, created_ms, updated_ms, created_by, owner_user_id) VALUES
 ('prj_A', 'A', 'active', 1, 1, 'u', 'usr_a'),
 ('prj_B', 'B', 'active', 1, 1, 'u', 'usr_b'),
 ('prj_T', 'T', 'trashed', 1, 1, 'u', 'usr_b'),
 ('prj_Q', 'Q', 'active', 1, 1, 'u', 'usr_b')`,
		`INSERT INTO project_items (id, project_id, kind, title, position, state, ref, created_ms, updated_ms, created_by)
 VALUES
 ('pit_a2', 'prj_A', 'timeline', 'a2', 1, 'active', 'tl_A2', 1, 1, 'u'),
 ('pit_b2', 'prj_B', 'timeline', 'b2', 1, 'active', 'tl_B2', 1, 1, 'u'),
 ('pit_t2', 'prj_T', 'timeline', 't2', 1, 'trashed', 'tl_T2', 1, 1, 'u'),
 ('pit_aq', 'prj_A', 'timeline', 'aq', 2, 'active', 'prj_Q', 1, 1, 'u')`,
		`INSERT INTO cost_ceilings (project_id, currency, per_item_micros, per_day_micros, updated_by, updated_ms) VALUES
 ('tl_A2', 'USD', 10, NULL, 'p', 1),
 ('tl_B2', 'USD', 20, 200, 'p', 1),
 ('tl_T2', 'USD', 30, 300, 'p', 1),
 ('prj_Q', 'USD', 7, 7, 'p', 1)`,
	)

	full, err := schema.NewMigrator(db, allMigrations())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := full.Up(ctx); err != nil {
		t.Fatal(err)
	}

	want := map[string]ceilingRow{
		"prj_A/USD": {limit(10), none},
		"prj_B/USD": {limit(20), limit(200)},
		"prj_T/USD": {limit(30), limit(300)},
		"prj_Q/USD": {limit(7), limit(7)},
	}
	if got := ceilings(t, db); !reflect.DeepEqual(got, want) {
		t.Errorf("ceilings = %+v, want %+v", got, want)
	}
}

// The migration is one transaction: when a statement fails, every ceiling is where it was and it is not recorded as
// run, so the next start runs it whole.
func TestCeilingsToProject_AFailureLosesNoCeiling(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	upToCeilings(t, db)

	execAll(
		t,
		db,
		`INSERT INTO projects (id, name, created_ms, updated_ms, created_by) VALUES ('prj_A', 'A', 1, 1, 'u')`,
		`INSERT INTO project_items (id, project_id, kind, title, position, state, ref, created_ms, updated_ms, created_by)
 VALUES ('pit_a2', 'prj_A', 'timeline', 'a2', 1, 'active', 'tl_A2', 1, 1, 'u')`,
		`INSERT INTO cost_ceilings (project_id, currency, per_item_micros, per_day_micros, updated_by, updated_ms) VALUES
 ('prj_A', 'USD', 400, NULL, 'p', 1), ('tl_A2', 'USD', 300, 900, 'p', 1)`,
		// The move has already written the project's row when the delete of the timeline's row fails.
		`CREATE TRIGGER fail_the_move BEFORE DELETE ON cost_ceilings BEGIN SELECT RAISE(ABORT, 'interrupted'); END`,
	)

	before := ceilings(t, db)

	full, err := schema.NewMigrator(db, allMigrations())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := full.Up(ctx); err == nil {
		t.Fatal("the migration ran through a failing statement")
	}

	if got := ceilings(t, db); !reflect.DeepEqual(got, before) {
		t.Errorf("after the failure ceilings = %+v, want them as they were: %+v", got, before)
	}

	if n := count(t, db, `SELECT count(*) FROM migrations WHERE id = '20261121184500_ceilings_to_project'`); n != 0 {
		t.Error("a migration that failed is recorded as run")
	}

	execAll(t, db, `DROP TRIGGER fail_the_move`)

	if _, err := full.Up(ctx); err != nil {
		t.Fatalf("the migration does not run again after the failure: %v", err)
	}

	want := map[string]ceilingRow{"prj_A/USD": {limit(300), limit(900)}}
	if got := ceilings(t, db); !reflect.DeepEqual(got, want) {
		t.Errorf("after the second run ceilings = %+v, want %+v", got, want)
	}
}
