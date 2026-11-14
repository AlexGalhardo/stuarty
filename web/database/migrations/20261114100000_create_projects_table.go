package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The tables of SDD-005 section 5 are written as SQL so that the effective
// definition is the one the specification shows (checks, partial indexes,
// REAL columns, ON DELETE actions).

func m20261114100000CreateProjectsTable() schema.Migration {
	return schema.Migration{
		ID: "20261114100000_create_projects_table",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`CREATE TABLE projects (
  id              TEXT    NOT NULL,
  name            TEXT    NOT NULL,
  state           TEXT    NOT NULL DEFAULT 'active' CHECK (state IN ('active','trashed')),
  revision        INTEGER NOT NULL DEFAULT 0,
  schema_version  INTEGER NOT NULL,
  document        TEXT    NOT NULL,
  doc_sha256      TEXT    NOT NULL,
  width           INTEGER NOT NULL,
  height          INTEGER NOT NULL,
  fps_num         INTEGER NOT NULL,
  fps_den         INTEGER NOT NULL,
  duration_frames INTEGER NOT NULL DEFAULT 0,
  track_count     INTEGER NOT NULL DEFAULT 0,
  item_count      INTEGER NOT NULL DEFAULT 0,
  asset_count     INTEGER NOT NULL DEFAULT 0,
  resync_floor    INTEGER NOT NULL DEFAULT 0,
  created_ms      INTEGER NOT NULL,
  updated_ms      INTEGER NOT NULL,
  created_by      TEXT    NOT NULL,
  owner_user_id   TEXT,
  trashed_ms      INTEGER,
  trashed_by      TEXT,
  PRIMARY KEY (id)
)`); err != nil {
				return err
			}
			return s.Raw(`CREATE INDEX projects_state_updated_ms_index ON projects (state, updated_ms)`)
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("projects") },
	}
}
