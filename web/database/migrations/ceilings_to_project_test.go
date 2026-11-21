package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/AlexGalhardo/vukem/schema"
)

type ceilingRow struct {
	item, day sql.NullInt64
}

var none sql.NullInt64

func limit(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

func ceilings(t *testing.T, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) map[string]ceilingRow {
	t.Helper()

	rows, err := db.QueryContext(context.Background(),
		`SELECT project_id || '/' || currency, per_item_micros, per_day_micros FROM cost_ceilings`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	out := map[string]ceilingRow{}

	for rows.Next() {
		var (
			key string
			row ceilingRow
		)

		if err := rows.Scan(&key, &row.item, &row.day); err != nil {
			t.Fatal(err)
		}

		out[key] = row
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	return out
}

// VV-366, author decision D-132: a ceiling stored under the id of a later timeline moves to the project that owns
// it, keeping the lowest limit of each field when the project already has one in the same currency; a row whose id
// matches nothing stays as it was; and an approval rule is never touched.
func TestCeilingsToProjectMigration(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

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

	if _, err := old.Up(ctx); err != nil {
		t.Fatal(err)
	}

	execAll(
		t,
		db,
		`INSERT INTO projects (id, name, created_ms, updated_ms, created_by) VALUES
 ('prj_A', 'A', 1, 1, 'u'), ('prj_B', 'B', 1, 1, 'u'), ('prj_C', 'C', 1, 1, 'u'), ('prj_D', 'D', 1, 1, 'u')`,
		`INSERT INTO project_items (id, project_id, kind, title, position, state, ref, created_ms, updated_ms, created_by)
 VALUES
 ('pit_a1', 'prj_A', 'timeline', 'first', 0, 'active', 'prj_A', 1, 1, 'u'),
 ('pit_a2', 'prj_A', 'timeline', 'second', 1, 'active', 'tl_A2', 1, 1, 'u'),
 ('pit_a3', 'prj_A', 'timeline', 'third, in the trash', 2, 'trashed', 'tl_A3', 1, 1, 'u'),
 ('pit_b2', 'prj_B', 'timeline', 'second', 1, 'active', 'tl_B2', 1, 1, 'u'),
 ('pit_c2', 'prj_C', 'timeline', 'second', 1, 'active', 'tl_C2', 1, 1, 'u')`,
		`INSERT INTO cost_ceilings (project_id, currency, per_item_micros, per_day_micros, updated_by, updated_ms) VALUES
 ('', 'USD', 1, 1, 'p', 1),
 ('prj_A', 'USD', 400, 2000, 'p', 1),
 ('tl_A2', 'USD', 500, NULL, 'p', 1),
 ('tl_A3', 'USD', 300, 9000, 'p', 1),
 ('tl_A2', 'EUR', 100, NULL, 'p', 1),
 ('tl_B2', 'USD', NULL, 700, 'p', 1),
 ('prj_C', 'USD', NULL, 100, 'p', 1),
 ('tl_C2', 'USD', 50, 900, 'p', 1),
 ('prj_D', 'USD', 8, 8, 'p', 1),
 ('tl_gone', 'USD', 10, 10, 'p', 1)`,
		`INSERT INTO approval_rules (id, kind, project_id, currency, per_item_micros, created_by, created_ms)
 VALUES ('rul_1', 'k', 'tl_A2', 'USD', 1000, 'p', 1)`,
	)

	full, err := schema.NewMigrator(db, allMigrations())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := full.Up(ctx); err != nil {
		t.Fatal(err)
	}

	want := map[string]ceilingRow{
		"/USD":        {limit(1), limit(1)},
		"prj_A/USD":   {limit(300), limit(2000)},
		"prj_A/EUR":   {limit(100), none},
		"prj_B/USD":   {none, limit(700)},
		"prj_C/USD":   {limit(50), limit(100)},
		"prj_D/USD":   {limit(8), limit(8)},
		"tl_gone/USD": {limit(10), limit(10)},
	}

	check := func(when string) {
		t.Helper()

		got := ceilings(t, db)
		if len(got) != len(want) {
			t.Errorf("%s: ceilings = %v, want %v", when, got, want)
		}

		for k, w := range want {
			if got[k] != w {
				t.Errorf("%s: ceiling %s = %+v, want %+v", when, k, got[k], w)
			}
		}
	}

	check("after Up")

	if n := count(t, db, `SELECT count(*) FROM approval_rules WHERE id = 'rul_1' AND project_id = 'tl_A2'`); n != 1 {
		t.Error("the rule written with the id of a later timeline was changed; it must stay as it was")
	}

	// Down cannot say which project a limit came from, so it leaves the merged rows; running Up again changes nothing.
	rolled, err := full.Rollback(ctx, 1)
	if err != nil || len(rolled) != 1 {
		t.Fatalf("Rollback = %v, %v", rolled, err)
	}

	check("after Down")

	if _, err := full.Up(ctx); err != nil {
		t.Fatal(err)
	}

	check("after Up again")
}
