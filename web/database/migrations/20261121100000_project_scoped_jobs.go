package migrations

import "github.com/AlexGalhardo/vukem/schema"

// A project made since epic 56 has no timeline until a video is added, and a person generates an image, a voice,
// a text or a video before that. The rows that such a generation writes (the job, the asset it attaches, and the
// key limited to the project) name the project, not a timeline row (SDD-005 section 5.12.3), so their project_id
// no longer references timelines (id): it holds a project id for a project-wide row and a timeline id for a
// timeline-bound one, and the first timeline has the id of its project. The cascade that the foreign key gave an
// emptied project is done by the store (productionRepo.Delete and projectRepo.Delete). SQLite cannot drop a
// foreign key in place, so the three tables are rebuilt, as 20261119140100 did.
func m20261121100000ProjectScopedJobs() schema.Migration {
	return schema.Migration{
		ID:   "20261121100000_project_scoped_jobs",
		Up:   func(s *schema.Schema) error { return rebuildProjectScoped(s, false) },
		Down: func(s *schema.Schema) error { return rebuildProjectScoped(s, true) },
	}
}

// rebuildProjectScoped rebuilds jobs, project_assets and api_keys with project_id free (references false) or
// referencing timelines (id) again. Down first drops the rows that name a project that has no timeline row,
// which the foreign key cannot hold: a generation of a project with no timeline, and a key limited to one.
func rebuildProjectScoped(s *schema.Schema, references bool) error {
	ref := func(on string) string {
		if references {
			return " REFERENCES timelines (id) ON DELETE " + on
		}

		return ""
	}

	var stmts []string

	if references {
		stmts = append(stmts,
			`DELETE FROM jobs WHERE project_id IS NOT NULL AND project_id NOT IN (SELECT id FROM timelines)`,
			`DELETE FROM project_assets WHERE project_id NOT IN (SELECT id FROM timelines)`,
			`DELETE FROM api_keys WHERE project_id IS NOT NULL AND project_id NOT IN (SELECT id FROM timelines)`,
		)
	}

	stmts = append(
		stmts,
		`CREATE TABLE jobs_rebuild (
  id               TEXT    NOT NULL,
  kind             TEXT    NOT NULL,
  project_id       TEXT`+ref("CASCADE")+`,
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
  CHECK ((kind = 'ingest' AND asset_hash IS NOT NULL)
    OR (kind = 'image.export' AND project_id IS NULL AND asset_hash IS NULL)
    OR (kind NOT IN ('ingest','image.export') AND project_id IS NOT NULL))
)`,
		`INSERT INTO jobs_rebuild (`+jobsAllColumns+`) SELECT `+jobsAllColumns+` FROM jobs`,
		`DROP TABLE jobs`,
		`ALTER TABLE jobs_rebuild RENAME TO jobs`,
		`CREATE INDEX jobs_project_id_created_ms_index ON jobs (project_id, created_ms)`,
		`CREATE INDEX jobs_kind_state_index ON jobs (kind, state)`,
		`CREATE UNIQUE INDEX jobs_active_ingest_unique ON jobs (asset_hash) WHERE kind = 'ingest' AND state IN ('queued','running')`,
		`CREATE TABLE project_assets_rebuild (
  project_id  TEXT    NOT NULL`+ref("CASCADE")+`,
  asset_id    TEXT    NOT NULL,
  asset_hash  TEXT    NOT NULL REFERENCES assets (id) ON DELETE RESTRICT,
  state       TEXT    NOT NULL CHECK (state IN ('pending','active','trashed')),
  name        TEXT    NOT NULL,
  snapshot    TEXT,
  imported_by TEXT    NOT NULL,
  imported_ms INTEGER NOT NULL,
  trashed_by  TEXT,
  trashed_ms  INTEGER,
  PRIMARY KEY (project_id, asset_id)
)`,
		`INSERT INTO project_assets_rebuild (project_id, asset_id, asset_hash, state, name, snapshot, imported_by,
 imported_ms, trashed_by, trashed_ms)
SELECT project_id, asset_id, asset_hash, state, name, snapshot, imported_by, imported_ms, trashed_by, trashed_ms
FROM project_assets`,
		`DROP TABLE project_assets`,
		`ALTER TABLE project_assets_rebuild RENAME TO project_assets`,
		`CREATE UNIQUE INDEX project_assets_project_id_asset_hash_unique ON project_assets (project_id, asset_hash)`,
		`CREATE INDEX project_assets_asset_hash_index ON project_assets (asset_hash)`,
		`CREATE TABLE api_keys_rebuild (
  id         TEXT    NOT NULL,
  token_id   TEXT    NOT NULL,
  user_id    TEXT    NOT NULL,
  project_id TEXT`+ref("CASCADE")+`,
  name       TEXT    NOT NULL,
  created_ms INTEGER NOT NULL,
  PRIMARY KEY (id)
)`,
		`INSERT INTO api_keys_rebuild (id, token_id, user_id, project_id, name, created_ms)
SELECT id, token_id, user_id, project_id, name, created_ms FROM api_keys`,
		`DROP TABLE api_keys`,
		`ALTER TABLE api_keys_rebuild RENAME TO api_keys`,
		`CREATE UNIQUE INDEX api_keys_token_id_unique ON api_keys (token_id)`,
		`CREATE INDEX api_keys_user_id_index ON api_keys (user_id)`,
	)

	for _, q := range stmts {
		if err := s.Raw(q); err != nil {
			return err
		}
	}

	return nil
}
