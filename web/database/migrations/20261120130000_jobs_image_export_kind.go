package migrations

import "github.com/AlexGalhardo/vukem/schema"

// jobsAllColumns are the columns of jobs as 20261119140100 left it, the approval column included.
const jobsAllColumns = jobsColumns + `, approval_id`

// An image export is recorded as a job of the kind image.export so that a publication can take its file as a
// thumbnail (SDD-021 section 6, SDD-020 section 4, D-111). An image belongs to no project, so such a job has no
// project and no asset: the table accepted only an ingest with an asset, or another kind with a project. SQLite cannot
// change a CHECK in place and nothing references jobs, so the table is rebuilt as 20261119140100 did.
func m20261120130000JobsImageExportKind() schema.Migration {
	return schema.Migration{
		ID:   "20261120130000_jobs_image_export_kind",
		Up:   func(s *schema.Schema) error { return rebuildJobsForImageExport(s, true) },
		Down: func(s *schema.Schema) error { return rebuildJobsForImageExport(s, false) },
	}
}

// rebuildJobsForImageExport is the rebuild both ways; Down drops the rows of the image.export kind, which the
// previous table cannot hold.
func rebuildJobsForImageExport(s *schema.Schema, imageExport bool) error {
	owner := `CHECK ((kind = 'ingest' AND asset_hash IS NOT NULL) OR (kind <> 'ingest' AND project_id IS NOT NULL))`
	keep := `WHERE kind <> 'image.export'`

	if imageExport {
		owner = `CHECK ((kind = 'ingest' AND asset_hash IS NOT NULL)
    OR (kind = 'image.export' AND project_id IS NULL AND asset_hash IS NULL)
    OR (kind NOT IN ('ingest','image.export') AND project_id IS NOT NULL))`
		keep = ""
	}

	stmts := []string{
		`CREATE TABLE jobs_rebuild (
  id               TEXT    NOT NULL,
  kind             TEXT    NOT NULL,
  project_id       TEXT    REFERENCES timelines (id) ON DELETE CASCADE,
  asset_hash       TEXT    REFERENCES assets (id) ON DELETE CASCADE,
  actor            TEXT    NOT NULL,
  state            TEXT    NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','waiting_approval','running','succeeded','failed','cancelled')),
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
  approval_id      TEXT,
  PRIMARY KEY (id),
  ` + owner + `
)`,
		`INSERT INTO jobs_rebuild (` + jobsAllColumns + `) SELECT ` + jobsAllColumns + ` FROM jobs ` + keep,
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
