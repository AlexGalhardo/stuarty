package migrations

import "github.com/AlexGalhardo/vukem/schema"

func m20261114100500CreateTemplatesTable() schema.Migration {
	return schema.Migration{
		ID: "20261114100500_create_templates_table",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`CREATE TABLE templates (
  id                TEXT    NOT NULL,
  name              TEXT    NOT NULL,
  description       TEXT    NOT NULL DEFAULT '',
  schema_version    INTEGER NOT NULL,
  document          TEXT    NOT NULL,
  doc_sha256        TEXT    NOT NULL,
  slots             TEXT    NOT NULL,
  width             INTEGER NOT NULL,
  height            INTEGER NOT NULL,
  fps_num           INTEGER NOT NULL,
  fps_den           INTEGER NOT NULL,
  duration_frames   INTEGER NOT NULL DEFAULT 0,
  source_project_id TEXT,
  created_ms        INTEGER NOT NULL,
  created_by        TEXT    NOT NULL,
  PRIMARY KEY (id)
)`); err != nil {
				return err
			}
			return s.Raw(`CREATE INDEX templates_created_ms_index ON templates (created_ms)`)
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("templates") },
	}
}
