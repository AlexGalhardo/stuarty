package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The checkpoints of an image document (SDD-005 section 5.14.1, RF-121, epic 50): a name and a note on a revision of
// the append-only log `image_operations`, made by a person or an agent, or by the service before an agent's first edit
// of the day. They are the table of section 5.14 for an image, with the same rules; a checkpoint holds no copy of the
// document and no foreign key to the log, only to its image, which removes it with its own row.
func m20261120120000CreateImageCheckpoints() schema.Migration {
	return schema.Migration{
		ID: "20261120120000_create_image_checkpoints",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE image_checkpoints (
  image_id TEXT    NOT NULL REFERENCES image_documents (id) ON DELETE CASCADE,
  name     TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 240),
  note     TEXT    NOT NULL DEFAULT '',
  revision INTEGER NOT NULL CHECK (revision >= 1),
  actor    TEXT    NOT NULL,
  auto     INTEGER NOT NULL DEFAULT 0 CHECK (auto IN (0,1)),
  at_ms    INTEGER NOT NULL,
  PRIMARY KEY (image_id, name)
)`,
				`CREATE INDEX image_checkpoints_revision_index ON image_checkpoints (image_id, revision DESC)`,
			}

			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("image_checkpoints") },
	}
}
