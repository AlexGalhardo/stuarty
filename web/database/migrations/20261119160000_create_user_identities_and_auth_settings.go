package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The optional Google sign-in of network mode (SDD-007 section 4.7, SDD-014 section 12). user_identities
// holds the identity a user linked: the provider, the stable subject identifier and the e-mail the provider
// vouched for, which is shown and never used to find a user. A user has one identity per provider and an
// identity belongs to one user. No token is stored. auth_settings holds the owner's settings of the
// feature (on or off, the connection, the allowed e-mails) as key and value.
func m20261119160000CreateUserIdentitiesAndAuthSettings() schema.Migration {
	return schema.Migration{
		ID: "20261119160000_create_user_identities_and_auth_settings",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`CREATE TABLE user_identities (
  provider   TEXT    NOT NULL,
  subject    TEXT    NOT NULL,
  user_id    TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  email      TEXT    NOT NULL,
  created_ms INTEGER NOT NULL,
  PRIMARY KEY (provider, subject)
)`); err != nil {
				return err
			}

			if err := s.Raw(
				`CREATE UNIQUE INDEX user_identities_user_provider ON user_identities (user_id, provider)`,
			); err != nil {
				return err
			}

			return s.Raw(`CREATE TABLE auth_settings (
  key        TEXT    NOT NULL,
  value      TEXT    NOT NULL,
  updated_ms INTEGER NOT NULL,
  PRIMARY KEY (key)
)`)
		},
		Down: func(s *schema.Schema) error {
			if err := s.DropIfExists("user_identities"); err != nil {
				return err
			}

			return s.DropIfExists("auth_settings")
		},
	}
}
