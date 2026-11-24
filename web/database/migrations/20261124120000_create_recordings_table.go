package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The screen recordings (SDD-025, SDD-005 section 5.18): one row per recording, with its state and the decision of a
// person about it. The title of a captured window is sealed (SDD-005 section 5.17.2). No frame is stored here.
// Its id sorts after 20261124110000_project_kind (epic 78), so a workspace that ran that one first still opens.
func m20261124120000CreateRecordingsTable() schema.Migration {
	return schema.Migration{
		ID: "20261124120000_create_recordings_table",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE recordings (
  id             TEXT    NOT NULL,
  project_id     TEXT    NOT NULL,
  state          TEXT    NOT NULL CHECK (state IN ('prepared','recording','finishing','ready','failed','cancelled')),
  review         TEXT    NOT NULL DEFAULT '' CHECK (review IN ('','pending','added','dismissed')),
  source_kind    TEXT    NOT NULL,
  source_label   TEXT    NOT NULL DEFAULT '',
  width          INTEGER NOT NULL DEFAULT 0,
  height         INTEGER NOT NULL DEFAULT 0,
  fps            INTEGER NOT NULL,
  encoder        TEXT    NOT NULL DEFAULT '',
  started_ms     INTEGER NOT NULL DEFAULT 0,
  ended_ms       INTEGER NOT NULL DEFAULT 0,
  duration_ms    INTEGER NOT NULL DEFAULT 0,
  frames         INTEGER NOT NULL DEFAULT 0,
  dropped        INTEGER NOT NULL DEFAULT 0,
  bytes          INTEGER NOT NULL DEFAULT 0,
  stop_reason    TEXT    NOT NULL DEFAULT '',
  error_code     TEXT    NOT NULL DEFAULT '',
  error_message  TEXT    NOT NULL DEFAULT '',
  asset_hash     TEXT    NOT NULL DEFAULT '',
  asset_id       TEXT    NOT NULL DEFAULT '',
  actor          TEXT    NOT NULL,
  created_ms     INTEGER NOT NULL,
  PRIMARY KEY (id)
)`,
				`CREATE INDEX recordings_project_index ON recordings (project_id, created_ms)`,
				`CREATE INDEX recordings_state_index ON recordings (state)`,
			}

			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("recordings") },
	}
}
