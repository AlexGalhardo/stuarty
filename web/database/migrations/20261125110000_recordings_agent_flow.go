package migrations

import "github.com/AlexGalhardo/vukem/schema"

// m20261125110000RecordingsAgentFlow gives a recording what an agent prepares, marks and proposes (SDD-025 section
// 19.6, SDD-005 section 5.18): `plan`, the JSON of a prepared recording a person has still to start (empty for
// every other recording, so a row made before this migration is not "prepared"), and the JSON lists `marks` and
// `proposals`. None holds a window title, a device name or a pointer position.
func m20261125110000RecordingsAgentFlow() schema.Migration {
	return schema.Migration{
		ID: "20261125110000_recordings_agent_flow",
		Up: func(s *schema.Schema) error {
			for _, stmt := range []string{
				`ALTER TABLE recordings ADD COLUMN plan TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE recordings ADD COLUMN marks TEXT NOT NULL DEFAULT '[]'`,
				`ALTER TABLE recordings ADD COLUMN proposals TEXT NOT NULL DEFAULT '[]'`,
			} {
				if err := s.Raw(stmt); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			// A prepared recording has no file and no meaning without its plan: it goes with the column.
			if err := s.Raw(`DELETE FROM recordings WHERE state = 'prepared' AND plan <> ''`); err != nil {
				return err
			}

			for _, col := range []string{"proposals", "marks", "plan"} {
				if err := s.Raw(`ALTER TABLE recordings DROP COLUMN ` + col); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
