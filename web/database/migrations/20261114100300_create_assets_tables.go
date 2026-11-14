package migrations

import "github.com/AlexGalhardo/vukem/schema"

func m20261114100300CreateAssetsTables() schema.Migration {
	return schema.Migration{
		ID: "20261114100300_create_assets_tables",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE assets (
  id                 TEXT    NOT NULL,
  kind               TEXT    CHECK (kind IS NULL OR kind IN ('video','audio','image')),
  size_bytes         INTEGER NOT NULL,
  original_name      TEXT    NOT NULL,
  ingest_state       TEXT    NOT NULL DEFAULT 'pending' CHECK (ingest_state IN ('pending','ingesting','ready','failed')),
  ingest_progress    REAL    NOT NULL DEFAULT 0 CHECK (ingest_progress >= 0 AND ingest_progress <= 1),
  ingest_error       TEXT,
  normalized_version INTEGER,
  duration_ms        INTEGER,
  width              INTEGER,
  height             INTEGER,
  has_audio          INTEGER NOT NULL DEFAULT 0,
  frame_rate_num     INTEGER,
  frame_rate_den     INTEGER,
  frame_count        INTEGER,
  sample_count       INTEGER,
  created_ms         INTEGER NOT NULL,
  updated_ms         INTEGER NOT NULL,
  PRIMARY KEY (id)
)`,
				`CREATE TABLE project_assets (
  project_id  TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
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
				`CREATE UNIQUE INDEX project_assets_project_id_asset_hash_unique ON project_assets (project_id, asset_hash)`,
				`CREATE INDEX project_assets_asset_hash_index ON project_assets (asset_hash)`,
				`CREATE TABLE artifacts (
  asset_hash TEXT    NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL CHECK (kind IN ('probe','normalized','proxy','thumbnails','peaks','silence','loudness','transcript')),
  profile    INTEGER NOT NULL,
  variant    TEXT    NOT NULL DEFAULT '',
  ext        TEXT    NOT NULL,
  rel_path   TEXT    NOT NULL,
  size_bytes INTEGER NOT NULL,
  sha256     TEXT    NOT NULL,
  meta       TEXT,
  created_ms INTEGER NOT NULL,
  PRIMARY KEY (asset_hash, kind, profile, variant, ext)
)`,
			}
			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(s *schema.Schema) error {
			for _, t := range []string{"artifacts", "project_assets", "assets"} {
				if err := s.DropIfExists(t); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
