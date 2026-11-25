package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The days and the recurring slots of the publishing calendar (SDD-020 section 30A, SDD-005 section 5.15.2). A day is
// a local date with the zone it was planned in, a note, a theme and a quiet mark; a slot is a weekday and a local time
// that is expanded to instants when a range is read, so no instant is stored. No table holds a secret.
func m20261125100000CreateCalendarTables() schema.Migration {
	return schema.Migration{
		ID: "20261125100000_create_calendar_tables",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE calendar_days (
  owner_user_id TEXT    NOT NULL DEFAULT '',
  local_date    TEXT    NOT NULL CHECK (length(local_date) = 10),
  zone          TEXT    NOT NULL,
  note          TEXT    NOT NULL DEFAULT '',
  theme         TEXT    NOT NULL DEFAULT '' CHECK (theme IN ('','blue','green','amber','red','teal','gray')),
  quiet         INTEGER NOT NULL DEFAULT 0 CHECK (quiet IN (0,1)),
  updated_ms    INTEGER NOT NULL,
  PRIMARY KEY (owner_user_id, local_date)
)`,
				`CREATE TABLE calendar_slots (
  id            TEXT    NOT NULL,
  owner_user_id TEXT    NOT NULL DEFAULT '',
  network       TEXT    NOT NULL,
  weekday       TEXT    NOT NULL CHECK (weekday IN ('mon','tue','wed','thu','fri','sat','sun')),
  local_time    TEXT    NOT NULL CHECK (length(local_time) = 5),
  zone          TEXT    NOT NULL,
  connection_id TEXT    NOT NULL DEFAULT '',
  created_ms    INTEGER NOT NULL,
  updated_ms    INTEGER NOT NULL,
  PRIMARY KEY (id)
)`,
				`CREATE INDEX calendar_slots_owner_user_id_index ON calendar_slots (owner_user_id, weekday)`,
			}
			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			for _, t := range []string{"calendar_slots", "calendar_days"} {
				if err := s.DropIfExists(t); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
