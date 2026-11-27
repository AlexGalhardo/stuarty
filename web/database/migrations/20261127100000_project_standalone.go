package migrations

import "github.com/AlexGalhardo/vukem/schema"

// m20261127100000ProjectStandalone adds the standalone flag on projects (SDD-005 section 5.12.5, epic 105)
// so standalone video editor sessions share the timeline engine without cluttering production listings.
func m20261127100000ProjectStandalone() schema.Migration {
	return schema.Migration{
		ID: "20261127100000_project_standalone",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(
				`ALTER TABLE projects ADD COLUMN standalone INTEGER NOT NULL DEFAULT 0 CHECK (standalone IN (0,1))`,
			); err != nil {
				return err
			}

			return s.Raw(
				`CREATE INDEX projects_standalone_state_updated_ms_index ON projects (standalone, state, updated_ms)`,
			)
		},
		Down: func(s *schema.Schema) error {
			if err := s.Raw(`DROP INDEX IF EXISTS projects_standalone_state_updated_ms_index`); err != nil {
				return err
			}

			return s.Raw(`ALTER TABLE projects DROP COLUMN standalone`)
		},
	}
}
