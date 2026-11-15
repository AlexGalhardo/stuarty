package migrations

import "github.com/AlexGalhardo/vukem/schema"

// access_tokens is the table of vukem's auth.Tokens: the secret of an API key
// is stored as its SHA-256 only (SDD-007 section 4.2). api_keys maps the
// product's key id to a row of this table.
func m20261115110100CreateAccessTokensTable() schema.Migration {
	return schema.Migration{
		ID: "20261115110100_create_access_tokens_table",
		Up: func(s *schema.Schema) error {
			return s.Create("access_tokens", func(t *schema.Table) {
				t.String("id").Unique()
				t.String("user_id").Index()
				t.String("name")
				t.String("hash")
				t.Text("abilities")
				t.Timestamp("last_used_at").Nullable()
				t.Timestamp("expires_at").Nullable()
				t.Timestamp("revoked_at").Nullable()
				t.Timestamp("created_at")
			})
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("access_tokens") },
	}
}
