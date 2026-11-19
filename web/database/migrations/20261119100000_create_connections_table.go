package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The destinations the user connected (SDD-014 section 8, ADR-0032). The table holds no secret:
// secret_ref is the entry of the vault file that holds the credential. hosts and accepts are
// JSON arrays of strings.
func m20261119100000CreateConnectionsTable() schema.Migration {
	return schema.Migration{
		ID: "20261119100000_create_connections_table",
		Up: func(s *schema.Schema) error {
			return s.Raw(`CREATE TABLE connections (
  id           TEXT    NOT NULL,
  kind         TEXT    NOT NULL,
  label        TEXT    NOT NULL,
  endpoint     TEXT    NOT NULL,
  hosts        TEXT    NOT NULL,
  accepts      TEXT    NOT NULL,
  secret_ref   TEXT    NOT NULL,
  created_ms   INTEGER NOT NULL,
  created_by   TEXT    NOT NULL,
  last_used_ms INTEGER,
  PRIMARY KEY (id)
)`)
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("connections") },
	}
}
