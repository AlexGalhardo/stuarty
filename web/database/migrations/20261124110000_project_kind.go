package migrations

import "github.com/AlexGalhardo/vukem/schema"

// m20261124110000ProjectKind adds the kind of a project and the steps of its workflow that a person skipped
// (SDD-005 section 5.12.4, epic 78). It changes no row: a project made before it reads the default kind, undecided,
// and its effective kind comes from its items at read time, so no data of an existing workspace moves.
func m20261124110000ProjectKind() schema.Migration {
	return schema.Migration{
		ID: "20261124110000_project_kind",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`ALTER TABLE projects ADD COLUMN kind TEXT NOT NULL DEFAULT 'undecided'
 CHECK (kind IN ('undecided','text','video','image'))`); err != nil {
				return err
			}

			return s.Raw(`ALTER TABLE projects ADD COLUMN skipped_steps TEXT NOT NULL DEFAULT ''`)
		},
		Down: func(s *schema.Schema) error {
			if err := s.Raw(`ALTER TABLE projects DROP COLUMN skipped_steps`); err != nil {
				return err
			}

			return s.Raw(`ALTER TABLE projects DROP COLUMN kind`)
		},
	}
}
