package migrations

import "github.com/AlexGalhardo/vukem/schema"

func m20261114100600CreateDocumentBackupsTable() schema.Migration {
	return schema.Migration{
		ID: "20261114100600_create_document_backups_table",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`CREATE TABLE document_backups (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_kind     TEXT    NOT NULL CHECK (owner_kind IN ('project','template')),
  owner_id       TEXT    NOT NULL,
  revision       INTEGER NOT NULL,
  schema_version INTEGER NOT NULL,
  document       TEXT    NOT NULL,
  reason         TEXT    NOT NULL,
  created_ms     INTEGER NOT NULL
)`); err != nil {
				return err
			}
			return s.Raw(
				`CREATE INDEX document_backups_owner_kind_owner_id_index ON document_backups (owner_kind, owner_id)`,
			)
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("document_backups") },
	}
}
