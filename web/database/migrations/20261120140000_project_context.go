package migrations

import "github.com/AlexGalhardo/vukem/schema"

// projectContextColumns are the columns epic 56 adds to projects (SDD-005 section 5.12.2): the description and
// the SMART goal. A project made before the migration keeps empty text: no default is invented for it.
var projectContextColumns = []string{
	"description",
	"goal_specific",
	"goal_measurable",
	"goal_achievable",
	"goal_relevant",
	"goal_time_bound",
	"goal_due",
}

func m20261120140000ProjectContext() schema.Migration {
	return schema.Migration{
		ID: "20261120140000_project_context",
		Up: func(s *schema.Schema) error {
			for _, c := range projectContextColumns {
				if err := s.Raw(`ALTER TABLE projects ADD COLUMN ` + c + ` TEXT NOT NULL DEFAULT ''`); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			for _, c := range projectContextColumns {
				if err := s.Raw(`ALTER TABLE projects DROP COLUMN ` + c); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
