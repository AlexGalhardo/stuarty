package migrations

import "github.com/AlexGalhardo/vukem/schema"

// A disabled user keeps the row (their projects and keys stay attributable) but cannot log in, hold
// a session or use a key; disabled_ms is when an administrator disabled them, NULL while enabled
// (SDD-007 section 4.1).
func m20261115120000AddUsersDisabledMS() schema.Migration {
	return schema.Migration{
		ID: "20261115120000_add_users_disabled_ms",
		Up: func(s *schema.Schema) error {
			return s.Raw(`ALTER TABLE users ADD COLUMN disabled_ms INTEGER`)
		},
		Down: func(s *schema.Schema) error {
			return s.Raw(`ALTER TABLE users DROP COLUMN disabled_ms`)
		},
	}
}
