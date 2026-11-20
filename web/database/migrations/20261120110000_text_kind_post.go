package migrations

import (
	"fmt"

	"github.com/AlexGalhardo/vukem/schema"
)

const (
	textKindsBefore = `'script','copy','caption'`
	textKindsAfter  = `'script','copy','caption','post'`
)

// rebuildProjectItems recreates project_items with the given list of text kinds. SQLite cannot change a CHECK in
// place, and dropping the table with foreign keys on deletes the revisions that reference it (ON DELETE CASCADE),
// so the revisions are kept in a side table and put back. Every row, index and revision stays.
func rebuildProjectItems(s *schema.Schema, kinds string) error {
	stmts := []string{
		`CREATE TABLE text_revisions_keep AS SELECT * FROM text_revisions`,
		fmt.Sprintf(`CREATE TABLE project_items_rebuild (
  id         TEXT    NOT NULL,
  project_id TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL CHECK (kind IN ('brief','text','timeline','output')),
  title      TEXT    NOT NULL,
  position   INTEGER NOT NULL,
  state      TEXT    NOT NULL DEFAULT 'active' CHECK (state IN ('active','trashed')),
  ref        TEXT,
  body       TEXT    NOT NULL DEFAULT '',
  format     TEXT,
  created_ms INTEGER NOT NULL,
  updated_ms INTEGER NOT NULL,
  created_by TEXT    NOT NULL,
  trashed_ms INTEGER,
  text_kind  TEXT CHECK (text_kind IN (%s)),
  PRIMARY KEY (id)
)`, kinds),
		`INSERT INTO project_items_rebuild (id, project_id, kind, title, position, state, ref, body, format, created_ms,
 updated_ms, created_by, trashed_ms, text_kind)
SELECT id, project_id, kind, title, position, state, ref, body, format, created_ms, updated_ms, created_by, trashed_ms,
 text_kind FROM project_items`,
		`DROP TABLE project_items`,
		`ALTER TABLE project_items_rebuild RENAME TO project_items`,
		`CREATE INDEX project_items_project_id_position_index ON project_items (project_id, position)`,
		`CREATE UNIQUE INDEX project_items_timeline_ref_unique ON project_items (ref) WHERE kind = 'timeline'`,
		`CREATE UNIQUE INDEX project_items_brief_unique ON project_items (project_id) WHERE kind = 'brief' AND state = 'active'`,
		`DELETE FROM text_revisions`,
		`INSERT INTO text_revisions SELECT * FROM text_revisions_keep`,
		`DROP TABLE text_revisions_keep`,
	}

	for _, q := range stmts {
		if err := s.Raw(q); err != nil {
			return err
		}
	}

	return nil
}

// The text kind `post` (SDD-019 section 13, SDD-005 section 5.12): a blog post is a text of its own kind, so the check
// of project_items.text_kind grows by one value. Down turns every post into a `copy`, which keeps its body and its
// revisions, and puts the shorter check back.
func m20261120110000TextKindPost() schema.Migration {
	return schema.Migration{
		ID: "20261120110000_text_kind_post",
		Up: func(s *schema.Schema) error { return rebuildProjectItems(s, textKindsAfter) },
		Down: func(s *schema.Schema) error {
			if err := s.Raw(`UPDATE project_items SET text_kind = 'copy' WHERE text_kind = 'post'`); err != nil {
				return err
			}

			return rebuildProjectItems(s, textKindsBefore)
		},
	}
}
