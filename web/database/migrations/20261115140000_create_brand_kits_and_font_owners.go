package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The brand kit and the record of who imported or brought each font file (SDD-005 section 5.11,
// RF-053). owner_user_id is the empty string in local mode, so the primary key of brand_kits
// stays NOT NULL.
func m20261115140000CreateBrandKitsAndFontOwners() schema.Migration {
	return schema.Migration{
		ID: "20261115140000_create_brand_kits_and_font_owners",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`CREATE TABLE brand_kits (
  owner_user_id TEXT    NOT NULL,
  document      TEXT    NOT NULL,
  updated_ms    INTEGER NOT NULL,
  updated_by    TEXT    NOT NULL,
  PRIMARY KEY (owner_user_id)
)`); err != nil {
				return err
			}

			return s.Raw(`CREATE TABLE font_owners (
  hash          TEXT    NOT NULL,
  owner_user_id TEXT    NOT NULL,
  created_ms    INTEGER NOT NULL,
  PRIMARY KEY (hash, owner_user_id)
)`)
		},
		Down: func(s *schema.Schema) error {
			if err := s.DropIfExists("font_owners"); err != nil {
				return err
			}

			return s.DropIfExists("brand_kits")
		},
	}
}
