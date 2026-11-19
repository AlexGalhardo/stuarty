package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The index of the library: memory, skills and prompts (SDD-018 section 9, ADR-0040). The files of
// the workspace are the truth and these two tables can be dropped and rebuilt from them with
// LibraryReindex; nothing in them is read as the content of an entry. library_fts is an FTS5 table
// whose key column is stored and not indexed.
func m20261119120000CreateLibraryIndex() schema.Migration {
	return schema.Migration{
		ID: "20261119120000_create_library_index",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`CREATE TABLE library_entries (
  key           TEXT    NOT NULL,
  kind          TEXT    NOT NULL,
  name          TEXT    NOT NULL,
  description   TEXT    NOT NULL,
  scope         TEXT    NOT NULL,
  origin        TEXT    NOT NULL,
  reviewed      INTEGER NOT NULL,
  kept          INTEGER NOT NULL,
  actor         TEXT    NOT NULL,
  tags          TEXT    NOT NULL,
  state         TEXT    NOT NULL,
  error         TEXT    NOT NULL,
  path          TEXT    NOT NULL,
  sig           TEXT    NOT NULL,
  lint_errors   INTEGER NOT NULL,
  lint_warnings INTEGER NOT NULL,
  has_scripts   INTEGER NOT NULL,
  created_ms    INTEGER NOT NULL,
  updated_ms    INTEGER NOT NULL,
  PRIMARY KEY (key),
  CHECK (kind IN ('memory','skill','prompt')),
  CHECK (state IN ('ok','invalid'))
)`); err != nil {
				return err
			}

			if err := s.Raw(`CREATE INDEX library_entries_kind ON library_entries (kind, name)`); err != nil {
				return err
			}

			return s.Raw(`CREATE VIRTUAL TABLE library_fts USING fts5(
  key UNINDEXED, name, description, tags, body, tokenize = 'unicode61 remove_diacritics 2'
)`)
		},
		Down: func(s *schema.Schema) error {
			if err := s.Raw(`DROP TABLE IF EXISTS library_fts`); err != nil {
				return err
			}

			return s.DropIfExists("library_entries")
		},
	}
}
