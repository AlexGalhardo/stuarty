package migrations

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/vukem/schema"
)

const (
	targetStatesBefore = `'draft','waiting_approval','approved','scheduled','publishing','published','failed','missed','cancelled'`
	targetStatesAfter  = `'draft','waiting_approval','approved','scheduled','publishing','published','sent_as_draft','failed','missed','cancelled'`
)

// rebuildPublicationTargets recreates publication_targets with the given list of states, keeping every row
// and every index. SQLite cannot change a CHECK in place; nothing references publication_targets, so the copy,
// drop and rename are safe with foreign keys on. A row in a state the list does not hold takes the closest
// state it does (a draft sent to TikTok goes back to published).
func rebuildPublicationTargets(s *schema.Schema, states string) error {
	cols := `id, publication_id, position, connection_id, network, fields, state, scheduled_ms, zone, late_minutes,
  send_now, approval_id, snapshot, payload_sha256, attempts, next_attempt_ms, lease_owner, lease_until_ms, remote_id,
  url, remote_state, error, published_ms, created_ms, updated_ms`

	stateColumn := `state`
	if states == targetStatesBefore {
		stateColumn = `CASE WHEN state = 'sent_as_draft' THEN 'published' ELSE state END`
	}

	stmts := []string{
		fmt.Sprintf(`CREATE TABLE publication_targets_rebuild (
  id              TEXT    NOT NULL,
  publication_id  TEXT    NOT NULL REFERENCES publications (id) ON DELETE CASCADE,
  position        INTEGER NOT NULL,
  connection_id   TEXT    NOT NULL,
  network         TEXT    NOT NULL,
  fields          TEXT    NOT NULL,
  state           TEXT    NOT NULL CHECK (state IN (%s)),
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
)`, states),
		fmt.Sprintf(`INSERT INTO publication_targets_rebuild (%s) SELECT %s FROM publication_targets`, cols,
			strings.Replace(cols, " state,", " "+stateColumn+" AS state,", 1)),
		`DROP TABLE publication_targets`,
		`ALTER TABLE publication_targets_rebuild RENAME TO publication_targets`,
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
}

// A post that TikTok took as a draft is not published: its target ends in the state sent_as_draft (SDD-020
// section 38, SDD-005 section 5.15). The table is rebuilt with the longer CHECK.
func m20261120100000PublicationTargetsSentAsDraft() schema.Migration {
	return schema.Migration{
		ID:   "20261120100000_publication_targets_sent_as_draft",
		Up:   func(s *schema.Schema) error { return rebuildPublicationTargets(s, targetStatesAfter) },
		Down: func(s *schema.Schema) error { return rebuildPublicationTargets(s, targetStatesBefore) },
	}
}
