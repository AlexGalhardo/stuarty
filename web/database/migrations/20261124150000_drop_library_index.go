package migrations

import "github.com/AlexGalhardo/vukem/schema"

// m20261124150000DropLibraryIndex removes the two tables of the index of the library (ADR-0053, SDD-018 section 9):
// the index lives in memory, is built at the unlock and is dropped at lock, so no index of the notes, prompts and
// skills is in db.sqlite. The files are the truth, so nothing is lost; Down recreates the tables empty.
func m20261124150000DropLibraryIndex() schema.Migration {
	return schema.Migration{
		ID: "20261124150000_drop_library_index",
		Up: func(s *schema.Schema) error {
			if err := s.Raw(`DROP TABLE IF EXISTS library_fts`); err != nil {
				return err
			}

			return s.DropIfExists("library_entries")
		},
		Down: func(s *schema.Schema) error {
			return m20261119130000CreateLibraryIndex().Up(s)
		},
	}
}
