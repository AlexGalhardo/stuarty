package migrations

import "github.com/AlexGalhardo/vukem/schema"

// jobsColumns are the columns shared by the table before and after the rebuild.
const jobsColumns = `id, kind, project_id, asset_hash, actor, state, progress, message, input, result, error,
  cancel_requested, attempts, created_ms, updated_ms, started_ms, finished_ms`

// A job of a kind that needs approval waits in the state waiting_approval until a person or a rule
// approves it (SDD-015 section 5), and a feature epic registers its own kinds, so the closed list of
// kinds goes (the service validates the kind). The table also gains approval_id, the request the job
// waits on. SQLite cannot change a CHECK in place, and nothing references jobs, so the table is
// rebuilt: copy, drop, rename and the indexes again.
func m20261119140100JobsWaitingApproval() schema.Migration {
	return schema.Migration{
		ID:   "20261119140100_jobs_waiting_approval",
		Up:   func(s *schema.Schema) error { return rebuildJobs(s, true) },
		Down: func(s *schema.Schema) error { return rebuildJobs(s, false) },
	}
}

// rebuildJobs is the rebuild both ways. Down gives back the closed list of kinds that the table had before, which
// includes export_batch (20261119120050, which sorts before this one), so the parents of export batches survive it.
func rebuildJobs(s *schema.Schema, approval bool) error {
	kind := `CHECK (kind IN ('ingest','transcribe','export','export_batch','frame'))`
	state := `CHECK (state IN ('queued','running','succeeded','failed','cancelled'))`
	extra := ""
	keep := `WHERE kind IN ('ingest','transcribe','export','export_batch','frame') AND state <> 'waiting_approval'`

	if approval {
		kind = ""
		state = `CHECK (state IN ('queued','waiting_approval','running','succeeded','failed','cancelled'))`
		extra = "\n  approval_id      TEXT,"
		keep = ""
	}

	stmts := []string{
		`CREATE TABLE jobs_rebuild (
  id               TEXT    NOT NULL,
  kind             TEXT    NOT NULL ` + kind + `,
  project_id       TEXT    REFERENCES timelines (id) ON DELETE CASCADE,
  asset_hash       TEXT    REFERENCES assets (id) ON DELETE CASCADE,
  actor            TEXT    NOT NULL,
  state            TEXT    NOT NULL DEFAULT 'queued' ` + state + `,
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
  finished_ms      INTEGER,` + extra + `
  PRIMARY KEY (id),
  CHECK ((kind = 'ingest' AND asset_hash IS NOT NULL) OR (kind <> 'ingest' AND project_id IS NOT NULL))
)`,
		`INSERT INTO jobs_rebuild (` + jobsColumns + `) SELECT ` + jobsColumns + ` FROM jobs ` + keep,
		`DROP TABLE jobs`,
		`ALTER TABLE jobs_rebuild RENAME TO jobs`,
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
}
