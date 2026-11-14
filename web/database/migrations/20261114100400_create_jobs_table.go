package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The editor owns the table named jobs (ADR-0018); vukem's queue uses
// queue_jobs and queue_failed_jobs.
func m20261114100400CreateJobsTable() schema.Migration {
	return schema.Migration{
		ID: "20261114100400_create_jobs_table",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE jobs (
  id               TEXT    NOT NULL,
  kind             TEXT    NOT NULL CHECK (kind IN ('ingest','transcribe','export','frame')),
  project_id       TEXT    REFERENCES projects (id) ON DELETE CASCADE,
  asset_hash       TEXT    REFERENCES assets (id) ON DELETE CASCADE,
  actor            TEXT    NOT NULL,
  state            TEXT    NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','running','succeeded','failed','cancelled')),
  progress         REAL    NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 1),
  message          TEXT    NOT NULL DEFAULT '',
  input            TEXT    NOT NULL,
  result           TEXT,
  error            TEXT,
  cancel_requested INTEGER NOT NULL DEFAULT 0,
  attempts         INTEGER NOT NULL DEFAULT 0,
  created_ms       INTEGER NOT NULL,
  updated_ms       INTEGER NOT NULL,
  started_ms       INTEGER,
  finished_ms      INTEGER,
  PRIMARY KEY (id),
  CHECK ((kind = 'ingest' AND asset_hash IS NOT NULL) OR (kind <> 'ingest' AND project_id IS NOT NULL))
)`,
				`CREATE INDEX jobs_project_id_created_ms_index ON jobs (project_id, created_ms)`,
				`CREATE INDEX jobs_kind_state_index ON jobs (kind, state)`,
				`CREATE UNIQUE INDEX jobs_active_ingest_unique ON jobs (asset_hash) WHERE kind = 'ingest' AND state IN ('queued','running')`,
			}
			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("jobs") },
	}
}
