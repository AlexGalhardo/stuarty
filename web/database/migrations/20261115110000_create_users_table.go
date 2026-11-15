package migrations

import "github.com/AlexGalhardo/vukem/schema"

// users holds the accounts of network mode (SDD-007 section 4.1). The password
// column is an argon2id hash; session_version ends every browser session of the
// user when it changes. The table stays empty in local mode.
func m20261115110000CreateUsersTable() schema.Migration {
	return schema.Migration{
		ID: "20261115110000_create_users_table",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`CREATE TABLE users (
  id              TEXT    NOT NULL,
  email           TEXT    NOT NULL,
  password        TEXT    NOT NULL,
  role            TEXT    NOT NULL CHECK (role IN ('admin', 'member')),
  session_version INTEGER NOT NULL DEFAULT 0,
  created_ms      INTEGER NOT NULL,
  updated_ms      INTEGER NOT NULL,
  PRIMARY KEY (id)
)`); err != nil {
				return err
			}

			return s.Raw(`CREATE UNIQUE INDEX users_email_unique ON users (email)`)
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("users") },
	}
}
