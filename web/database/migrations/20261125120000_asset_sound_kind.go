package migrations

import "github.com/AlexGalhardo/vukem/schema"

// m20261125120000AssetSoundKind gives the library side of an audio file the kind a person sees in the Sound Assets
// page (SDD-005 section 5.5.1, SDD-011 section 14.12): `sound_kind`, `music`, `voice` or `effect`, empty when it is
// not set. A row made before this migration keeps the empty value.
func m20261125120000AssetSoundKind() schema.Migration {
	return schema.Migration{
		ID: "20261125120000_asset_sound_kind",
		Up: func(s *schema.Schema) error {
			return s.Raw(`ALTER TABLE asset_info ADD COLUMN sound_kind TEXT NOT NULL DEFAULT ''
 CHECK (sound_kind IN ('','music','voice','effect'))`)
		},
		Down: func(s *schema.Schema) error {
			return s.Raw(`ALTER TABLE asset_info DROP COLUMN sound_kind`)
		},
	}
}
