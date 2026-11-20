package migrations

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// hasColumnNamed says whether a list of columns, each "name TYPE ...", has one called name.
func hasColumnNamed(cols []string, name string) bool {
	return slices.ContainsFunc(cols, func(c string) bool { return strings.HasPrefix(c, name+" ") })
}

// The text kind post: the check grows, and no row, revision or index is lost by the rebuild of project_items.
func TestTextKindPostKeepsEveryRowAndRevision(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	m, err := newMigrator(db)
	if err != nil {
		t.Fatal(err)
	}

	// Go back to before the migration, put a text with two revisions in, then run it forward again.
	for {
		step, rerr := m.Rollback(ctx, 1)
		if rerr != nil || len(step) != 1 {
			t.Fatalf("Rollback = %v, %v", step, rerr)
		}

		if step[0] == "20261120110000_text_kind_post" {
			break
		}
	}

	execAll(
		t,
		db,
		`INSERT INTO projects (id, name, created_ms, updated_ms, created_by) VALUES ('prj_a', 'a', 1, 1, 'u')`,
		`INSERT INTO project_items (id, project_id, kind, title, position, body, created_ms, updated_ms, created_by, text_kind)
 VALUES ('pit_a', 'prj_a', 'text', 'A caption', 1, 'body', 1, 2, 'u', 'caption')`,
		`INSERT INTO text_revisions (item_id, revision, body, actor, source, created_ms) VALUES ('pit_a', 1, 'one', 'u', 'created', 1)`,
		`INSERT INTO text_revisions (item_id, revision, body, actor, source, created_ms) VALUES ('pit_a', 2, 'two', 'u', 'write', 2)`,
	)

	if _, err := db.ExecContext(ctx, `UPDATE project_items SET text_kind = 'post' WHERE id = 'pit_a'`); err == nil {
		t.Fatal("post was a kind before the migration")
	}

	if ran, uerr := m.Up(ctx); uerr != nil || !slices.Contains(ran, "20261120110000_text_kind_post") {
		t.Fatalf("Up = %v, %v", ran, uerr)
	}

	checkNoOrphans(t, db)

	if n := count(
		t,
		db,
		`SELECT count(*) FROM project_items WHERE id = 'pit_a' AND text_kind = 'caption' AND body = 'body'`,
	); n != 1 {
		t.Errorf("the item changed: %d", n)
	}

	if n := count(t, db, `SELECT count(*) FROM text_revisions WHERE item_id = 'pit_a'`); n != 2 {
		t.Errorf("%d revisions after the rebuild, want 2", n)
	}

	for _, kind := range []string{"post", "script", "copy", "caption"} {
		if _, err := db.ExecContext(
			ctx,
			`UPDATE project_items SET text_kind = ? WHERE id = 'pit_a'`,
			kind,
		); err != nil {
			t.Errorf("kind %s: %v", kind, err)
		}
	}

	if _, err := db.ExecContext(ctx, `UPDATE project_items SET text_kind = 'poem' WHERE id = 'pit_a'`); err == nil {
		t.Error("a kind that does not exist was accepted")
	}

	// The indexes of the table are back: one brief per project, and a timeline ref once.
	execAll(
		t,
		db,
		`INSERT INTO project_items (id, project_id, kind, title, position, created_ms, updated_ms, created_by)
 VALUES ('pit_b', 'prj_a', 'brief', 'B', 2, 1, 1, 'u')`,
	)

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO project_items (id, project_id, kind, title, position, created_ms, updated_ms, created_by)
 VALUES ('pit_c', 'prj_a', 'brief', 'B2', 3, 1, 1, 'u')`,
	); err == nil {
		t.Error("a second active brief was accepted: the unique index is gone")
	}

	// The revisions still go with their item.
	execAll(t, db, `DELETE FROM project_items WHERE id = 'pit_a'`)

	if n := count(t, db, `SELECT count(*) FROM text_revisions`); n != 0 {
		t.Errorf("%d revisions of a deleted item are left", n)
	}
}

func TestTextKindPostDownTurnsPostsIntoCopies(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	execAll(
		t,
		db,
		`INSERT INTO projects (id, name, created_ms, updated_ms, created_by) VALUES ('prj_a', 'a', 1, 1, 'u')`,
		`INSERT INTO project_items (id, project_id, kind, title, position, body, created_ms, updated_ms, created_by, text_kind)
 VALUES ('pit_a', 'prj_a', 'text', 'A post', 1, 'body', 1, 2, 'u', 'post')`,
		`INSERT INTO text_revisions (item_id, revision, body, actor, source, created_ms) VALUES ('pit_a', 1, 'one', 'u', 'created', 1)`,
	)

	m, err := newMigrator(db)
	if err != nil {
		t.Fatal(err)
	}

	for {
		step, rerr := m.Rollback(ctx, 1)
		if rerr != nil || len(step) != 1 {
			t.Fatalf("Rollback = %v, %v", step, rerr)
		}

		if step[0] == "20261120110000_text_kind_post" {
			break
		}
	}

	if n := count(
		t,
		db,
		`SELECT count(*) FROM project_items WHERE id = 'pit_a' AND text_kind = 'copy' AND body = 'body'`,
	); n != 1 {
		t.Errorf("the post did not become a copy: %d", n)
	}

	if n := count(t, db, `SELECT count(*) FROM text_revisions WHERE item_id = 'pit_a'`); n != 1 {
		t.Errorf("the revision was lost: %d", n)
	}
}

// The blog tables: the post of a publication, one published row per slug, one remote id per connection and post.
func TestBlogTables(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	if got := columns(t, db, "publications"); !hasColumnNamed(got, "post_item_id") {
		t.Errorf("publications has no post_item_id: %v", got)
	}

	execAll(
		t,
		db,
		`INSERT INTO published_posts (slug, post_item_id, project_id, connection_id, title, status, markdown, html, published_ms, updated_ms)
 VALUES ('a', 'pit_a', 'prj_a', 'con_a', 'A', 'published', 'm', 'h', 1, 1)`,
		`INSERT INTO blog_remote_posts (connection_id, post_item_id, remote_id, updated_ms) VALUES ('con_a', 'pit_a', '7', 1)`,
	)

	for name, q := range map[string]string{
		"a slug twice": `INSERT INTO published_posts (slug, post_item_id, project_id, connection_id, title, status, markdown, html, published_ms, updated_ms)
 VALUES ('a', 'pit_b', 'prj_a', 'con_a', 'A', 'published', 'm', 'h', 1, 1)`,
		"a post twice for one connection": `INSERT INTO published_posts (slug, post_item_id, project_id, connection_id, title, status, markdown, html, published_ms, updated_ms)
 VALUES ('b', 'pit_a', 'prj_a', 'con_a', 'A', 'published', 'm', 'h', 1, 1)`,
		"a status that is not one": `INSERT INTO published_posts (slug, post_item_id, project_id, connection_id, title, status, markdown, html, published_ms, updated_ms)
 VALUES ('c', 'pit_c', 'prj_a', 'con_a', 'A', 'public', 'm', 'h', 1, 1)`,
		"a remote id twice": `INSERT INTO blog_remote_posts (connection_id, post_item_id, remote_id, updated_ms) VALUES ('con_a', 'pit_a', '8', 1)`,
	} {
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Errorf("%s was accepted", name)
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

		if step[0] == "20261120110100_create_blog_tables" {
			break
		}
	}

	for _, table := range []string{"published_posts", "blog_remote_posts"} {
		if got := columns(t, db, table); len(got) != 0 {
			t.Errorf("%s still exists after the down: %v", table, got)
		}
	}

	if got := columns(t, db, "publications"); hasColumnNamed(got, "post_item_id") {
		t.Errorf("publications still has post_item_id after the down: %v", got)
	}
}
