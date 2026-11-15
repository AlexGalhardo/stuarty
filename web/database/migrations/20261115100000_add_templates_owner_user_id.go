package migrations

import "github.com/AlexGalhardo/vukem/schema"

// Templates belong to the user who saved them in network mode, like projects
// (SDD-005 section 5.10); the column stays NULL in local mode.
func m20261115100000AddTemplatesOwnerUserID() schema.Migration {
	return schema.Migration{
		ID: "20261115100000_add_templates_owner_user_id",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`ALTER TABLE templates ADD COLUMN owner_user_id TEXT`); err != nil {
				return err
			}

			return s.Raw(
				`CREATE INDEX templates_owner_user_id_created_ms_index ON templates (owner_user_id, created_ms)`,
			)
		},
		Down: func(s *schema.Schema) error {
			if err := s.Raw(`DROP INDEX IF EXISTS templates_owner_user_id_created_ms_index`); err != nil {
				return err
			}

			return s.Raw(`ALTER TABLE templates DROP COLUMN owner_user_id`)
		},
	}
}
