package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The library side of an asset (SDD-005 section 5.5.1, SDD-011 section 14): where a file came from, the user's
// record of its rights, the reuse mark, the tags and the full-text index over them. The bytes are one per hash, but
// the library side belongs to an owner: the key is (owner_user_id, asset_hash), the empty owner is local mode, so
// two users who hold the same bytes keep their own record (VV-254). Every asset that is used gets an imported row
// for each owner whose projects use it; a file no project uses gets one for the empty owner.
func m20261119150000AssetLibrary() schema.Migration {
	return schema.Migration{
		ID: "20261119150000_asset_library",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE asset_info (
  owner_user_id TEXT  NOT NULL DEFAULT '',
  asset_hash  TEXT    NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
  origin_kind TEXT    NOT NULL DEFAULT 'imported' CHECK (origin_kind IN ('imported','generated','fetched_by_agent','rendered_externally')),
  source      TEXT    NOT NULL DEFAULT '',
  provider    TEXT    NOT NULL DEFAULT '',
  model       TEXT    NOT NULL DEFAULT '',
  prompt      TEXT    NOT NULL DEFAULT '',
  cost_micros INTEGER CHECK (cost_micros IS NULL OR cost_micros >= 0),
  rights_note TEXT    NOT NULL DEFAULT '',
  description TEXT    NOT NULL DEFAULT '',
  reusable    INTEGER NOT NULL DEFAULT 0 CHECK (reusable IN (0,1)),
  created_by  TEXT    NOT NULL,
  created_ms  INTEGER NOT NULL,
  updated_ms  INTEGER NOT NULL,
  PRIMARY KEY (owner_user_id, asset_hash)
)`,
				`CREATE INDEX asset_info_reusable_index ON asset_info (owner_user_id, asset_hash) WHERE reusable = 1`,
				`CREATE TABLE asset_tags (
  owner_user_id TEXT NOT NULL DEFAULT '',
  asset_hash TEXT NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
  tag        TEXT NOT NULL,
  PRIMARY KEY (owner_user_id, asset_hash, tag)
)`,
				`CREATE INDEX asset_tags_tag_index ON asset_tags (owner_user_id, tag, asset_hash)`,
				`CREATE VIRTUAL TABLE asset_search USING fts5(
  owner_user_id UNINDEXED, asset_hash UNINDEXED, name, tags, description, origin, tokenize = 'unicode61 remove_diacritics 2'
)`,
				`CREATE TEMP TABLE asset_owners AS
 SELECT DISTINCT COALESCE(pr.owner_user_id, '') AS owner, p.asset_hash AS hash
 FROM project_assets p JOIN projects pr ON pr.id = p.project_id
 UNION
 SELECT '', a.id FROM assets a WHERE NOT EXISTS (SELECT 1 FROM project_assets p WHERE p.asset_hash = a.id)`,
				`INSERT INTO asset_info (owner_user_id, asset_hash, created_by, created_ms, updated_ms)
 SELECT o.owner, o.hash,
        COALESCE((SELECT p.imported_by FROM project_assets p JOIN projects pr ON pr.id = p.project_id
                  WHERE p.asset_hash = o.hash AND COALESCE(pr.owner_user_id, '') = o.owner
                  ORDER BY p.imported_ms, p.asset_id LIMIT 1), 'migration'),
        a.created_ms, a.created_ms
 FROM asset_owners o JOIN assets a ON a.id = o.hash`,
				`INSERT INTO asset_search (owner_user_id, asset_hash, name, tags, description, origin)
 SELECT o.owner, o.hash,
        COALESCE((SELECT group_concat(DISTINCT p.name) FROM project_assets p JOIN projects pr ON pr.id = p.project_id
                  WHERE p.asset_hash = o.hash AND COALESCE(pr.owner_user_id, '') = o.owner), a.original_name),
        '', '', ''
 FROM asset_owners o JOIN assets a ON a.id = o.hash`,
				`DROP TABLE asset_owners`,
			}

			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			for _, t := range []string{"asset_search", "asset_tags", "asset_info"} {
				if err := s.DropIfExists(t); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
