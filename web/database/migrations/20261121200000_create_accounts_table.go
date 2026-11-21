package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The local account (ADR-0049, SDD-005 section 5.16): one row, at most. It holds the username and the e-mail in
// clear, the Argon2id parameters and salt, the verifier of the password and the keys, each wrapped; never the
// password, the recovery key or a clear key.
func m20261121200000CreateAccountsTable() schema.Migration {
	return schema.Migration{
		ID: "20261121200000_create_accounts_table",
		Up: func(s *schema.Schema) error {
			return s.Raw(`CREATE TABLE accounts (
  id                INTEGER NOT NULL CHECK (id = 1),
  username          TEXT    NOT NULL,
  email             TEXT    NOT NULL,
  kdf               TEXT    NOT NULL,
  verifier          BLOB    NOT NULL,
  key_by_password   BLOB    NOT NULL,
  key_by_recovery   BLOB    NOT NULL,
  vault_key         BLOB    NOT NULL,
  idle_lock_minutes INTEGER NOT NULL DEFAULT 0 CHECK (idle_lock_minutes BETWEEN 0 AND 1440),
  created_ms        INTEGER NOT NULL,
  updated_ms        INTEGER NOT NULL,
  PRIMARY KEY (id)
)`)
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("accounts") },
	}
}
