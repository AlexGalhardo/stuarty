package migrations

import (
	"context"
	"testing"
)

// The days and the slots of the calendar (SDD-005 section 5.15.2): one row per local date and owner, a theme from the
// fixed list, a weekday from the fixed list, and a time that is five characters.
func TestCalendarTablesAcceptWhatTheSpecificationNamesAndNothingElse(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	execAll(
		t,
		db,
		`INSERT INTO calendar_days (owner_user_id, local_date, zone, note, theme, quiet, updated_ms)
 VALUES ('', '2026-12-25', 'America/Sao_Paulo', 'long video day', 'amber', 1, 1)`,
		`INSERT INTO calendar_days (owner_user_id, local_date, zone, updated_ms) VALUES ('usr_a', '2026-12-25', 'UTC', 1)`,
		`INSERT INTO calendar_slots (id, network, weekday, local_time, zone, created_ms, updated_ms)
 VALUES ('slt_a', 'instagram', 'tue', '18:00', 'America/Sao_Paulo', 1, 1)`,
	)

	for name, q := range map[string]string{
		"a second row for one date and owner": `INSERT INTO calendar_days (owner_user_id, local_date, zone, updated_ms)
 VALUES ('', '2026-12-25', 'UTC', 1)`,
		"a theme that is a colour": `INSERT INTO calendar_days (owner_user_id, local_date, zone, theme, updated_ms)
 VALUES ('', '2026-12-26', 'UTC', '#ff0000', 1)`,
		"a quiet mark that is not a flag": `INSERT INTO calendar_days (owner_user_id, local_date, zone, quiet, updated_ms)
 VALUES ('', '2026-12-26', 'UTC', 2, 1)`,
		"a date that is not ten characters": `INSERT INTO calendar_days (owner_user_id, local_date, zone, updated_ms)
 VALUES ('', '2026-12-6', 'UTC', 1)`,
		"a weekday that is not one": `INSERT INTO calendar_slots (id, network, weekday, local_time, zone, created_ms, updated_ms)
 VALUES ('slt_b', 'x', 'tuesday', '18:00', 'UTC', 1, 1)`,
		"a time that is not five characters": `INSERT INTO calendar_slots (id, network, weekday, local_time, zone, created_ms, updated_ms)
 VALUES ('slt_c', 'x', 'tue', '6pm', 'UTC', 1, 1)`,
	} {
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	if n := count(t, db, `SELECT count(*) FROM calendar_days`); n != 2 {
		t.Errorf("%d day rows, want the two of two owners", n)
	}
}
