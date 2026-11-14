package migrations

import "github.com/AlexGalhardo/vukem/schema"

func m20261114100100CreateOperationsTable() schema.Migration {
	return schema.Migration{
		ID: "20261114100100_create_operations_table",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE operations (
  project_id      TEXT    NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  revision        INTEGER NOT NULL CHECK (revision >= 1),
  kind            TEXT    NOT NULL CHECK (kind IN ('edit','undo','redo','migrate','import')),
  actor           TEXT    NOT NULL,
  base_revision   INTEGER NOT NULL,
  target_revision INTEGER,
  at_ms           INTEGER NOT NULL,
  ops             TEXT    NOT NULL,
  inverse         TEXT    NOT NULL,
  diff            TEXT    NOT NULL,
  skipped         TEXT,
  schema_version  INTEGER NOT NULL,
  doc_sha256      TEXT    NOT NULL,
  closed_by       INTEGER,
  idempotency_key TEXT,
  PRIMARY KEY (project_id, revision)
)`,
				`CREATE INDEX operations_project_id_actor_revision_index ON operations (project_id, actor, revision)`,
				`CREATE INDEX operations_undo_idx ON operations (project_id, actor, revision)
  WHERE closed_by IS NULL AND kind IN ('edit','redo')`,
				`CREATE INDEX operations_redo_idx ON operations (project_id, actor, revision)
  WHERE closed_by IS NULL AND kind = 'undo'`,
				// SDD-007 T-28: the log refuses an UPDATE that changes anything but
				// closed_by, and closed_by only moves from NULL to a revision.
				// Deleting is left to the cascade of a hard project delete.
				`CREATE TRIGGER operations_append_only BEFORE UPDATE ON operations
WHEN OLD.closed_by IS NOT NULL
  OR NEW.closed_by IS NULL
  OR NEW.project_id IS NOT OLD.project_id
  OR NEW.revision IS NOT OLD.revision
  OR NEW.kind IS NOT OLD.kind
  OR NEW.actor IS NOT OLD.actor
  OR NEW.base_revision IS NOT OLD.base_revision
  OR NEW.target_revision IS NOT OLD.target_revision
  OR NEW.at_ms IS NOT OLD.at_ms
  OR NEW.ops IS NOT OLD.ops
  OR NEW.inverse IS NOT OLD.inverse
  OR NEW.diff IS NOT OLD.diff
  OR NEW.skipped IS NOT OLD.skipped
  OR NEW.schema_version IS NOT OLD.schema_version
  OR NEW.doc_sha256 IS NOT OLD.doc_sha256
  OR NEW.idempotency_key IS NOT OLD.idempotency_key
BEGIN
  SELECT RAISE(ABORT, 'operations is append-only');
END`,
			}
			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("operations") },
	}
}
