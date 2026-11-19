package migrations

import "github.com/AlexGalhardo/vukem/schema"

// A project becomes the container of a production (ADR-0039, SDD-005 section 5.12). The table that held one
// timeline document per project is renamed timelines (SQLite rewrites the foreign keys that name it, so the
// log, the assets, the jobs and the keys stay attached to the same ids); a new projects table holds the
// production and project_items its items. Every former project becomes a project with one timeline item whose
// ids are the old ids, so nothing a surface exposed changes.
//
// Down undoes it: a timeline that is not the first of its project (its id is not its project's) is deleted
// with its log, and the briefs, texts and outputs are lost. The database copy that Open takes before a
// migration (SDD-005 section 7 step 3) is what makes that acceptable.
func m20261119100000ProjectItems() schema.Migration {
	return schema.Migration{
		ID: "20261119100000_project_items",
		Up: func(s *schema.Schema) error {
			for _, q := range projectItemsUp {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			for _, q := range projectItemsDown {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
	}
}

var projectItemsUp = []string{
	`ALTER TABLE projects RENAME TO timelines`,
	`DROP INDEX projects_state_updated_ms_index`,
	`CREATE INDEX timelines_state_updated_ms_index ON timelines (state, updated_ms)`,
	`CREATE TABLE projects (
  id            TEXT    NOT NULL,
  name          TEXT    NOT NULL,
  state         TEXT    NOT NULL DEFAULT 'active' CHECK (state IN ('active','trashed')),
  created_ms    INTEGER NOT NULL,
  updated_ms    INTEGER NOT NULL,
  created_by    TEXT    NOT NULL,
  owner_user_id TEXT,
  trashed_ms    INTEGER,
  trashed_by    TEXT,
  PRIMARY KEY (id)
)`,
	`CREATE INDEX projects_state_updated_ms_index ON projects (state, updated_ms)`,
	`CREATE TABLE project_items (
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
  PRIMARY KEY (id)
)`,
	`CREATE INDEX project_items_project_id_position_index ON project_items (project_id, position)`,
	`CREATE UNIQUE INDEX project_items_timeline_ref_unique ON project_items (ref) WHERE kind = 'timeline'`,
	`CREATE UNIQUE INDEX project_items_brief_unique ON project_items (project_id) WHERE kind = 'brief' AND state = 'active'`,
	`INSERT INTO projects (id, name, state, created_ms, updated_ms, created_by, owner_user_id, trashed_ms, trashed_by)
SELECT id, name, state, created_ms, updated_ms, created_by, owner_user_id, trashed_ms, trashed_by FROM timelines`,
	// The item id is a function of the row, so the migration gives the same answer every time it runs on the
	// same data; hex digits are inside the alphabet of an identifier.
	`INSERT INTO project_items (id, project_id, kind, title, position, state, ref, body, created_ms, updated_ms, created_by, trashed_ms)
SELECT printf('pit_%010x', rowid), id, 'timeline', name, 0, 'active', id, '', created_ms, updated_ms, created_by, NULL
FROM timelines`,
}

var projectItemsDown = []string{
	`DELETE FROM timelines WHERE id NOT IN (SELECT id FROM projects)`,
	`DROP TABLE project_items`,
	`DROP TABLE projects`,
	`DROP INDEX timelines_state_updated_ms_index`,
	`ALTER TABLE timelines RENAME TO projects`,
	`CREATE INDEX projects_state_updated_ms_index ON projects (state, updated_ms)`,
}
