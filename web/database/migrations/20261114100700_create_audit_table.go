package migrations

import "github.com/AlexGalhardo/vukem/schema"

func m20261114100700CreateAuditTable() schema.Migration {
	return schema.Migration{
		ID: "20261114100700_create_audit_table",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE audit (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  at_ms      INTEGER NOT NULL,
  actor      TEXT    NOT NULL,
  action     TEXT    NOT NULL,
  project_id TEXT,
  subject_id TEXT,
  details    TEXT
)`,
				`CREATE INDEX audit_project_id_at_ms_index ON audit (project_id, at_ms)`,
				`CREATE INDEX audit_at_ms_index ON audit (at_ms)`,
			}
			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("audit") },
	}
}
