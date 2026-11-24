package migrations

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem/database"
	"github.com/AlexGalhardo/vukem/schema"
)

const projectKindID = "20261124110000_project_kind"

// migrateUntil runs the migrations of the list whose id passes keep, on db, and returns the ids that ran.
func migrateUntil(t *testing.T, db *database.DB, keep func(id string) bool) (*schema.Migrator, []string) {
	t.Helper()

	var migs []schema.Migration

	for _, m := range allMigrations() {
		if keep(m.ID) {
			migs = append(migs, m)
		}
	}

	m, err := schema.NewMigrator(db, migs)
	if err != nil {
		t.Fatal(err)
	}

	ran, err := m.Up(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	return m, ran
}

// snapshot is every row of the tables a project hangs from, as text, so that two snapshots tell whether any data
// moved. The columns of the projects table are those it had before the kind.
func snapshot(t *testing.T, db *database.DB) string {
	t.Helper()

	queries := []string{
		`SELECT id, name, state, created_ms, updated_ms, created_by, owner_user_id, trashed_ms, trashed_by, description,
 goal_specific, goal_measurable, goal_achievable, goal_relevant, goal_time_bound, goal_due FROM projects ORDER BY id`,
		`SELECT id, project_id, kind, title, position, state, ref, body, format, text_kind FROM project_items ORDER BY id`,
		`SELECT id, name, state, revision, doc_sha256 FROM timelines ORDER BY id`,
	}

	var out strings.Builder

	for _, q := range queries {
		out.WriteString(rowsAsText(t, db, q))
	}

	return out.String()
}

func rowsAsText(t *testing.T, db *database.DB, q string) string {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), q)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}

	defer rows.Close()

	cols, _ := rows.Columns()

	var out strings.Builder

	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))

		for i := range vals {
			ptrs[i] = &vals[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}

		for _, v := range vals {
			out.WriteString(v.String + "|")
		}

		out.WriteString("\n")
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	return out.String()
}

// An upgrade from a real older workspace: the workspaces of the releases the fixtures of project_items stand for
// are brought to the migration just before the kind, and then through it. No row changes, every project reads
// undecided with no skipped step (its effective kind comes from its items at read time, SDD-005 section 5.12.4),
// and the migration goes down and up again.
func TestProjectKindUpgradesAnExistingWorkspace(t *testing.T) {
	for _, r := range releases {
		t.Run(r.name, func(t *testing.T) {
			ctx := context.Background()
			db := workspaceOf(t, r)

			migrateUntil(t, db, func(id string) bool { return id < projectKindID })

			before := snapshot(t, db)
			if strings.TrimSpace(before) == "" {
				t.Fatal("the fixture holds no project")
			}

			m, ran := migrateUntil(t, db, func(id string) bool { return id <= projectKindID })
			if len(ran) != 1 || ran[0] != projectKindID {
				t.Fatalf("the upgrade ran %v, want only the kind", ran)
			}

			if after := snapshot(t, db); after != before {
				t.Fatalf("the migration moved data:\n--- before\n%s\n--- after\n%s", before, after)
			}

			if n := count(t, db, `SELECT count(*) FROM projects`); n != 3 {
				t.Fatalf("%d projects, want the 3 of the fixture", n)
			}

			if n := count(
				t,
				db,
				`SELECT count(*) FROM projects WHERE kind = 'undecided' AND skipped_steps = ''`,
			); n != 3 {
				t.Fatalf("%d projects read undecided with no skipped step, want 3", n)
			}

			// Down drops both columns and keeps the data; Up puts them back.
			if rolled, err := m.Rollback(ctx, 1); err != nil || len(rolled) != 1 {
				t.Fatalf("rollback: %v %v", rolled, err)
			}

			if got := columns(t, db, "projects"); listsColumn(got, "kind") || listsColumn(got, "skipped_steps") {
				t.Fatalf("down left a column: %v", got)
			}

			if again := snapshot(t, db); again != before {
				t.Fatal("down moved data")
			}

			if _, err := m.Up(ctx); err != nil {
				t.Fatal(err)
			}

			if n := count(t, db, `SELECT count(*) FROM projects WHERE kind = 'undecided'`); n != 3 {
				t.Fatalf("after up again: %d", n)
			}
		})
	}
}

// listsColumn reports whether the description of columns lists the column; each entry starts with its name.
func listsColumn(described []string, name string) bool {
	return slices.ContainsFunc(described, func(d string) bool { return strings.HasPrefix(d, name+" ") })
}

// A new project takes the default, and the kind is held to the four of SDD-000.
func TestProjectKindColumn(t *testing.T) {
	db := openMigrated(t)

	execAll(t, db, `INSERT INTO projects (id, name, created_ms, updated_ms, created_by)
 VALUES ('prj_0000000001', 'New project', 1, 1, 'u')`)

	var kind, skipped string
	if err := db.QueryRowContext(context.Background(), `SELECT kind, skipped_steps FROM projects`).
		Scan(&kind, &skipped); err != nil {
		t.Fatal(err)
	}

	if kind != "undecided" || skipped != "" {
		t.Fatalf("defaults: %q %q", kind, skipped)
	}

	for _, k := range []string{"text", "video", "image", "undecided"} {
		execAll(t, db, `UPDATE projects SET kind = '`+k+`'`)
	}

	if _, err := db.ExecContext(context.Background(), `UPDATE projects SET kind = 'audio'`); err == nil {
		t.Fatal("a kind that is not one was stored")
	}

	if _, err := db.ExecContext(context.Background(), `UPDATE projects SET kind = NULL`); err == nil {
		t.Fatal("a project with no kind was stored")
	}
}
