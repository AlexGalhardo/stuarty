package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The checkpoints of a timeline (SDD-005 section 5.14, RF-121): a name and a note on a revision, made by a person or an
// agent, or by the service before an agent's first edit of the day. A checkpoint points at a revision of the append-only
// log and holds no copy of the document, so it has no foreign key to `operations`.
func m20261119220000CreateCheckpoints() schema.Migration {
	return schema.Migration{
		ID: "20261119220000_create_checkpoints",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE checkpoints (
  project_id TEXT    NOT NULL REFERENCES timelines (id) ON DELETE CASCADE,
  name       TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 240),
  note       TEXT    NOT NULL DEFAULT '',
  revision   INTEGER NOT NULL CHECK (revision >= 1),
  actor      TEXT    NOT NULL,
  auto       INTEGER NOT NULL DEFAULT 0 CHECK (auto IN (0,1)),
  at_ms      INTEGER NOT NULL,
  PRIMARY KEY (project_id, name)
)`,
				`CREATE INDEX checkpoints_revision_index ON checkpoints (project_id, revision DESC)`,
			}

			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error { return s.DropIfExists("checkpoints") },
	}
}
