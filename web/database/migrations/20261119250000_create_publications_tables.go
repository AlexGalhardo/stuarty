package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The calendar of publications and their targets (SDD-020 sections 23 to 29, SDD-005 section 5.15). A target
// holds the instant in UTC with the zone the person chose, its state, the lease a scheduler holds while it
// sends, the snapshot of the post that was approved and the result. No table holds a secret.
func m20261119250000CreatePublicationsTables() schema.Migration {
	return schema.Migration{
		ID: "20261119250000_create_publications_tables",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE publications (
  id            TEXT    NOT NULL,
  project_id    TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  title         TEXT    NOT NULL,
  text          TEXT    NOT NULL DEFAULT '',
  export_job    TEXT    NOT NULL,
  thumbnail_job TEXT    NOT NULL DEFAULT '',
  created_ms    INTEGER NOT NULL,
  updated_ms    INTEGER NOT NULL,
  created_by    TEXT    NOT NULL,
  owner_user_id TEXT,
  PRIMARY KEY (id)
)`,
				`CREATE INDEX publications_project_id_index ON publications (project_id, updated_ms)`,
				`CREATE TABLE publication_targets (
  id              TEXT    NOT NULL,
  publication_id  TEXT    NOT NULL REFERENCES publications (id) ON DELETE CASCADE,
  position        INTEGER NOT NULL,
  connection_id   TEXT    NOT NULL,
  network         TEXT    NOT NULL,
  fields          TEXT    NOT NULL,
  state           TEXT    NOT NULL CHECK (state IN ('draft','waiting_approval','approved','scheduled','publishing','published','failed','missed','cancelled')),
  scheduled_ms    INTEGER,
  zone            TEXT    NOT NULL DEFAULT '',
  late_minutes    INTEGER NOT NULL DEFAULT 0 CHECK (late_minutes BETWEEN 0 AND 1440),
  send_now        INTEGER NOT NULL DEFAULT 0 CHECK (send_now IN (0,1)),
  approval_id     TEXT,
  snapshot        TEXT,
  payload_sha256  TEXT,
  attempts        INTEGER NOT NULL DEFAULT 0,
  next_attempt_ms INTEGER,
  lease_owner     TEXT,
  lease_until_ms  INTEGER,
  remote_id       TEXT,
  url             TEXT,
  remote_state    TEXT,
  error           TEXT,
  published_ms    INTEGER,
  created_ms      INTEGER NOT NULL,
  updated_ms      INTEGER NOT NULL,
  PRIMARY KEY (id)
)`,
				`CREATE INDEX publication_targets_publication_id_index ON publication_targets (publication_id, position)`,
				`CREATE INDEX publication_targets_state_scheduled_ms_index ON publication_targets (state, scheduled_ms)`,
				`CREATE INDEX publication_targets_approval_id_index ON publication_targets (approval_id)`,
			}
			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			for _, t := range []string{"publication_targets", "publications"} {
				if err := s.DropIfExists(t); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
