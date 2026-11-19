package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The image documents and their log of batches (SDD-005 section 5.12, SDD-021). They follow projects and
// operations with the columns an image needs and no more; owner_user_id is NULL in local mode.
func m20261119100000CreateImageDocumentsAndOperations() schema.Migration {
	return schema.Migration{
		ID: "20261119100000_create_image_documents_and_operations",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE image_documents (
  id             TEXT    NOT NULL,
  name           TEXT    NOT NULL,
  template       INTEGER NOT NULL DEFAULT 0 CHECK (template IN (0,1)),
  revision       INTEGER NOT NULL DEFAULT 0,
  schema_version INTEGER NOT NULL,
  document       TEXT    NOT NULL,
  doc_sha256     TEXT    NOT NULL,
  width          INTEGER NOT NULL,
  height         INTEGER NOT NULL,
  layers         INTEGER NOT NULL DEFAULT 0,
  created_ms     INTEGER NOT NULL,
  updated_ms     INTEGER NOT NULL,
  created_by     TEXT    NOT NULL,
  owner_user_id  TEXT,
  PRIMARY KEY (id)
)`,
				`CREATE INDEX image_documents_updated_ms_index ON image_documents (updated_ms)`,
				`CREATE TABLE image_operations (
  image_id        TEXT    NOT NULL REFERENCES image_documents (id) ON DELETE CASCADE,
  revision        INTEGER NOT NULL CHECK (revision >= 1),
  kind            TEXT    NOT NULL CHECK (kind IN ('edit','undo','redo')),
  actor           TEXT    NOT NULL,
  base_revision   INTEGER NOT NULL,
  target_revision INTEGER,
  at_ms           INTEGER NOT NULL,
  ops             TEXT    NOT NULL,
  inverse         TEXT    NOT NULL,
  diff            TEXT    NOT NULL,
  skipped         TEXT,
  doc_sha256      TEXT    NOT NULL,
  closed_by       INTEGER,
  idempotency_key TEXT,
  request_hash    TEXT,
  result          TEXT,
  PRIMARY KEY (image_id, revision)
)`,
				`CREATE INDEX image_operations_actor_index ON image_operations (image_id, actor, revision)`,
				`CREATE UNIQUE INDEX image_operations_key_index ON image_operations (image_id, actor, idempotency_key)
  WHERE idempotency_key IS NOT NULL`,
				`CREATE TRIGGER image_operations_append_only
BEFORE UPDATE ON image_operations
FOR EACH ROW
WHEN OLD.closed_by IS NOT NULL
  OR NEW.image_id IS NOT OLD.image_id
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
  OR NEW.doc_sha256 IS NOT OLD.doc_sha256
  OR NEW.idempotency_key IS NOT OLD.idempotency_key
  OR NEW.request_hash IS NOT OLD.request_hash
  OR NEW.result IS NOT OLD.result
BEGIN
  SELECT RAISE(ABORT, 'image_operations is append-only');
END`,
			}

			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			if err := s.DropIfExists("image_operations"); err != nil {
				return err
			}

			return s.DropIfExists("image_documents")
		},
	}
}
