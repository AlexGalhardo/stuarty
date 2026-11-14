package migrations

import "github.com/AlexGalhardo/vukem/schema"

func m20261114100200CreateIdempotencyKeysTable() schema.Migration {
	return schema.Migration{
		ID: "20261114100200_create_idempotency_keys_table",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`CREATE TABLE idempotency_keys (
  project_id   TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  actor        TEXT    NOT NULL,
  key          TEXT    NOT NULL,
  request_hash TEXT    NOT NULL,
  revision     INTEGER NOT NULL,
  result       TEXT    NOT NULL,
  created_ms   INTEGER NOT NULL,
  expires_ms   INTEGER NOT NULL,
  PRIMARY KEY (project_id, actor, key)
)`); err != nil {
				return err
			}
			return s.Raw(`CREATE INDEX idempotency_keys_expires_ms_index ON idempotency_keys (expires_ms)`)
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("idempotency_keys") },
	}
}
