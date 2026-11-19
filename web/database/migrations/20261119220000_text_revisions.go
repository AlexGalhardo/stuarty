package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The kind and the revisions of a text (SDD-005 section 5.12, SDD-019 section 3). A text item gets a text kind
// (the texts of epic 06 become `copy`, free text) and every text that exists gets a revision 1 holding its body, so
// that no text item is ever without one. Down drops the revisions and the column: a body stays in project_items.
func m20261119220000TextRevisions() schema.Migration {
	return schema.Migration{
		ID: "20261119220000_text_revisions",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`ALTER TABLE project_items ADD COLUMN text_kind TEXT CHECK (text_kind IN ('script','copy','caption'))`,
				`UPDATE project_items SET text_kind = 'copy' WHERE kind = 'text'`,
				`CREATE TABLE text_revisions (
  item_id     TEXT    NOT NULL REFERENCES project_items (id) ON DELETE CASCADE,
  revision    INTEGER NOT NULL CHECK (revision >= 1),
  body        TEXT    NOT NULL,
  actor       TEXT    NOT NULL,
  source      TEXT    NOT NULL CHECK (source IN ('created','write','generated','revert')),
  reverted_to INTEGER,
  provider    TEXT,
  model       TEXT,
  cost_micros INTEGER CHECK (cost_micros IS NULL OR cost_micros >= 0),
  currency    TEXT,
  job_id      TEXT,
  created_ms  INTEGER NOT NULL,
  PRIMARY KEY (item_id, revision)
)`,
				`INSERT INTO text_revisions (item_id, revision, body, actor, source, created_ms)
 SELECT id, 1, body, created_by, 'created', updated_ms FROM project_items WHERE kind = 'text'`,
			}

			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			if err := s.DropIfExists("text_revisions"); err != nil {
				return err
			}

			return s.Raw(`ALTER TABLE project_items DROP COLUMN text_kind`)
		},
	}
}
