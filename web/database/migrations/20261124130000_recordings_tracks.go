package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The tracks, the pauses and the pointer events of a recording (SDD-025 section 16, SDD-005 section 5.18). SQLite
// cannot change a CHECK, and the state list gains `paused`, so the table is made again beside the old one, the rows
// are copied and the old one is dropped. Three columns are added: the JSON lists of tracks and pauses, and the
// number of pointer events. None holds a device name or a pointer position.
func m20261124130000RecordingsTracks() schema.Migration {
	const columns = `id, project_id, state, review, source_kind, source_label, width, height, fps, encoder, started_ms,
 ended_ms, duration_ms, frames, dropped, bytes, stop_reason, error_code, error_message, asset_hash, asset_id, actor,
 created_ms`

	table := func(name, states, extra string) string {
		return `CREATE TABLE ` + name + ` (
  id             TEXT    NOT NULL,
  project_id     TEXT    NOT NULL,
  state          TEXT    NOT NULL CHECK (state IN (` + states + `)),
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
  created_ms     INTEGER NOT NULL` + extra + `,
  PRIMARY KEY (id)
)`
	}

	indexes := []string{
		`CREATE INDEX recordings_project_index ON recordings (project_id, created_ms)`,
		`CREATE INDEX recordings_state_index ON recordings (state)`,
	}

	rebuild := func(s *schema.Schema, states, extra, copyCols, selectCols string) error {
		stmts := []string{
			table("recordings_next", states, extra),
			`INSERT INTO recordings_next (` + copyCols + `) SELECT ` + selectCols + ` FROM recordings`,
			`DROP TABLE recordings`,
			`ALTER TABLE recordings_next RENAME TO recordings`,
		}
		stmts = append(stmts, indexes...)

		for _, q := range stmts {
			if err := s.Raw(q); err != nil {
				return err
			}
		}

		return nil
	}

	return schema.Migration{
		ID: "20261124130000_recordings_tracks",
		Up: func(s *schema.Schema) error {
			return rebuild(s,
				`'prepared','recording','paused','finishing','ready','failed','cancelled'`,
				`,
  tracks         TEXT    NOT NULL DEFAULT '[]',
  pauses         TEXT    NOT NULL DEFAULT '[]',
  events_count   INTEGER NOT NULL DEFAULT 0`,
				columns, columns)
		},
		Down: func(s *schema.Schema) error {
			// A paused recording goes back to being a recording; the three columns are dropped with the table.
			return rebuild(s,
				`'prepared','recording','finishing','ready','failed','cancelled'`, ``,
				columns, `id, project_id, CASE WHEN state = 'paused' THEN 'recording' ELSE state END, review,
 source_kind, source_label, width, height, fps, encoder, started_ms, ended_ms, duration_ms, frames, dropped, bytes,
 stop_reason, error_code, error_message, asset_hash, asset_id, actor, created_ms`)
		},
	}
}
