package migrations

import "github.com/AlexGalhardo/vukem/schema"

// api_keys maps the product's key identifier to vukem's token (SDD-005
// section 5.10). It stays empty in local mode.
func m20261114100800CreateAPIKeysTable() schema.Migration {
	return schema.Migration{
		ID: "20261114100800_create_api_keys_table",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE api_keys (
  id         TEXT    NOT NULL,
  token_id   TEXT    NOT NULL,
  user_id    TEXT    NOT NULL,
  project_id TEXT    REFERENCES projects (id) ON DELETE CASCADE,
  name       TEXT    NOT NULL,
  created_ms INTEGER NOT NULL,
  PRIMARY KEY (id)
)`,
				`CREATE UNIQUE INDEX api_keys_token_id_unique ON api_keys (token_id)`,
				`CREATE INDEX api_keys_user_id_index ON api_keys (user_id)`,
			}
			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("api_keys") },
	}
}
