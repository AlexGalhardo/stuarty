package migrations

import (
	"github.com/AlexGalhardo/vukem/auth"
	"github.com/AlexGalhardo/vukem/schema"
)

// access_tokens is the table of vukem's auth.Tokens: the secret of an API key
// is stored as its SHA-256 only (SDD-007 section 4.2). api_keys maps the
// product's key id to a row of this table. The id is the one this migration
// had when it was a copy of the columns, so a database that ran it does not
// run it again.
func m20261115110100CreateAccessTokensTable() schema.Migration {
	return auth.TokensMigration("20261115110100_create_access_tokens_table")
}
