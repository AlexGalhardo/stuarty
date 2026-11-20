package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The origin of a file rendered by another tool names the engine, its version and the key the agent chose for the
// animation, which stays the same when it is rendered again (SDD-005 section 5.5.1, SDD-011 section 15.2). They sit
// in asset_info beside the rest of the origin; an existing row keeps empty values.
func m20261119210000AssetRenderOrigin() schema.Migration {
	return schema.Migration{
		ID: "20261119210000_asset_render_origin",
		Up: func(s *schema.Schema) error {
			for _, col := range []string{"engine", "engine_version", "render_key"} {
				if err := s.Raw(`ALTER TABLE asset_info ADD COLUMN ` + col + ` TEXT NOT NULL DEFAULT ''`); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			for _, col := range []string{"render_key", "engine_version", "engine"} {
				if err := s.Raw(`ALTER TABLE asset_info DROP COLUMN ` + col); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
