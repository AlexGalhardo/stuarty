package migrations

import "github.com/AlexGalhardo/vukem/schema"

// A video with an alpha channel keeps it in its normalized file; the asset row says so, so the export
// composes the clip over the layers below instead of drawing it opaque (SDD-011 section 4.3.5,
// SDD-005 section 5.5).
func m20261116100000AssetsHasAlpha() schema.Migration {
	return schema.Migration{
		ID: "20261116100000_assets_has_alpha",
		Up: func(s *schema.Schema) error {
			return s.Raw(`ALTER TABLE assets ADD COLUMN has_alpha INTEGER NOT NULL DEFAULT 0`)
		},
		Down: func(s *schema.Schema) error {
			return s.Raw(`ALTER TABLE assets DROP COLUMN has_alpha`)
		},
	}
}
