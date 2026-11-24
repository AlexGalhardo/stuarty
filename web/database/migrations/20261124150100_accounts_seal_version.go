package migrations

import "github.com/AlexGalhardo/vukem/schema"

// m20261124150100AccountsSealVersion marks how much of the workspace the account has sealed (ADR-0053, SDD-005
// section 5.17.4). An account made before this migration reads 0: the second stage of sealed columns (the documents,
// the edit log, the backups, the idempotency results and the asset notes) is still clear text, and the first unlock
// seals it and moves the version to 1. An account made by this version is created at 1. The version is the one
// resumable state of that migration, and it is one way: an older binary does not know this migration and refuses the
// workspace.
func m20261124150100AccountsSealVersion() schema.Migration {
	return schema.Migration{
		ID: "20261124150100_accounts_seal_version",
		Up: func(s *schema.Schema) error {
			return s.Raw(`ALTER TABLE accounts ADD COLUMN seal_version INTEGER NOT NULL DEFAULT 0
 CHECK (seal_version BETWEEN 0 AND 1)`)
		},
		Down: func(s *schema.Schema) error {
			return s.Raw(`ALTER TABLE accounts DROP COLUMN seal_version`)
		},
	}
}
