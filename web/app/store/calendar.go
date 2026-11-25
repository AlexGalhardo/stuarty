package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

// The CalendarStore half of publicationRepo: the days and the recurring slots of SDD-005 section 5.15.2. The note of a
// day is a sealed column (5.17.2), bound to the row by "owner/date".

const dayColumns = `owner_user_id, local_date, zone, note, theme, quiet, updated_ms`

func dayKey(owner, date string) string { return owner + "/" + date }

func (r publicationRepo) scanDay(sc scanner) (pub.Day, error) {
	var (
		owner string
		d     pub.Day
		quiet int
		upd   int64
	)

	if err := sc.Scan(&owner, &d.Date, &d.Zone, &d.Note, &d.Theme, &quiet, &upd); err != nil {
		return d, err
	}

	note, err := openField(sealerOf(r.ex), "calendar_days", "note", dayKey(owner, d.Date), d.Note)
	if err != nil {
		return d, err
	}

	d.Note, d.Quiet, d.UpdatedAt = note, quiet != 0, time.UnixMilli(upd).UTC()

	return d, nil
}

func (r publicationRepo) Days(ctx context.Context, owner, fromDate, toDate string) ([]pub.Day, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT `+dayColumns+` FROM calendar_days
 WHERE owner_user_id = ? AND local_date >= ? AND local_date <= ? ORDER BY local_date`, owner, fromDate, toDate)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []pub.Day

	for rows.Next() {
		d, err := r.scanDay(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, d)
	}

	return out, rows.Err()
}

func (r publicationRepo) Day(ctx context.Context, owner, date string) (pub.Day, bool, error) {
	d, err := r.scanDay(r.ex.QueryRowContext(ctx,
		`SELECT `+dayColumns+` FROM calendar_days WHERE owner_user_id = ? AND local_date = ?`, owner, date))
	if errors.Is(err, sql.ErrNoRows) {
		return pub.Day{Date: date}, false, nil
	}

	return d, err == nil, err
}

func (r publicationRepo) PutDay(ctx context.Context, owner string, d pub.Day) error {
	if d.Empty() {
		_, err := r.ex.ExecContext(
			ctx,
			`DELETE FROM calendar_days WHERE owner_user_id = ? AND local_date = ?`,
			owner,
			d.Date,
		)

		return err
	}

	note, err := sealField(sealerOf(r.ex), "calendar_days", "note", dayKey(owner, d.Date), d.Note)
	if err != nil {
		return err
	}

	_, err = r.ex.ExecContext(ctx, `INSERT INTO calendar_days (`+dayColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT (owner_user_id, local_date) DO UPDATE SET zone = excluded.zone, note = excluded.note,
 theme = excluded.theme, quiet = excluded.quiet, updated_ms = excluded.updated_ms`,
		owner, d.Date, d.Zone, note, d.Theme, boolInt(d.Quiet), d.UpdatedAt.UnixMilli())

	return err
}

func (r publicationRepo) Slots(ctx context.Context, owner string) ([]core.Rule, error) {
	rows, err := r.ex.QueryContext(
		ctx,
		`SELECT id, network, weekday, local_time, zone, connection_id FROM calendar_slots
 WHERE owner_user_id = ? ORDER BY id`,
		owner,
	)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []core.Rule

	for rows.Next() {
		var s core.Rule

		if err := rows.Scan(&s.ID, &s.Network, &s.Weekday, &s.Time, &s.Zone, &s.ConnectionID); err != nil {
			return nil, err
		}

		out = append(out, s)
	}

	return out, rows.Err()
}

func (r publicationRepo) PutSlot(ctx context.Context, owner string, s core.Rule, now time.Time) error {
	res, err := r.ex.ExecContext(ctx, `INSERT INTO calendar_slots
 (id, owner_user_id, network, weekday, local_time, zone, connection_id, created_ms, updated_ms)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT (id) DO UPDATE SET network = excluded.network, weekday = excluded.weekday,
 local_time = excluded.local_time, zone = excluded.zone, connection_id = excluded.connection_id,
 updated_ms = excluded.updated_ms WHERE calendar_slots.owner_user_id = excluded.owner_user_id`,
		s.ID, owner, s.Network, s.Weekday, s.Time, s.Zone, s.ConnectionID, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return pub.ErrNotFound
	}

	return nil
}

func (r publicationRepo) DeleteSlot(ctx context.Context, owner, id string) (bool, error) {
	res, err := r.ex.ExecContext(ctx, `DELETE FROM calendar_slots WHERE id = ? AND owner_user_id = ?`, id, owner)
	if err != nil {
		return false, err
	}

	n, _ := res.RowsAffected()

	return n > 0, nil
}
