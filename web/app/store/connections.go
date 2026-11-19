package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/AlexGalhardo/stuarty/service/connections"

	"github.com/AlexGalhardo/vukem/database"
)

// connectionRepo is connections.Store over the table connections (SDD-014 section 8).
type connectionRepo struct{ ex database.Executor }

const connectionColumns = `id, kind, label, endpoint, hosts, accepts, secret_ref, created_ms, created_by, last_used_ms`

func scanConnection(sc interface{ Scan(...any) error }) (connections.Row, error) {
	var (
		r              connections.Row
		hosts, accepts string
		used           sql.NullInt64
	)

	if err := sc.Scan(
		&r.ID, &r.Kind, &r.Label, &r.Endpoint, &hosts, &accepts, &r.SecretRef, &r.CreatedMs, &r.CreatedBy, &used,
	); err != nil {
		return r, err
	}

	if err := json.Unmarshal([]byte(hosts), &r.Hosts); err != nil {
		return r, err
	}

	if err := json.Unmarshal([]byte(accepts), &r.Accepts); err != nil {
		return r, err
	}

	if used.Valid {
		r.LastUsedMs = &used.Int64
	}

	return r, nil
}

func (r connectionRepo) List(ctx context.Context) ([]connections.Row, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT `+connectionColumns+` FROM connections ORDER BY created_ms, id`)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []connections.Row

	for rows.Next() {
		row, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, row)
	}

	return out, rows.Err()
}

func (r connectionRepo) Get(ctx context.Context, id string) (connections.Row, error) {
	row, err := scanConnection(
		r.ex.QueryRowContext(ctx, `SELECT `+connectionColumns+` FROM connections WHERE id = ?`, id),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return row, connections.ErrNotFound
	}

	return row, err
}

func (r connectionRepo) Count(ctx context.Context) (int, error) {
	var n int

	err := r.ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM connections`).Scan(&n)

	return n, err
}

func (r connectionRepo) Insert(ctx context.Context, row connections.Row) error {
	hosts, err := json.Marshal(row.Hosts)
	if err != nil {
		return err
	}

	accepts, err := json.Marshal(row.Accepts)
	if err != nil {
		return err
	}

	_, err = r.ex.ExecContext(
		ctx,
		`INSERT INTO connections (`+connectionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID, row.Kind, row.Label, row.Endpoint, string(hosts), string(accepts), row.SecretRef,
		row.CreatedMs, row.CreatedBy, row.LastUsedMs,
	)

	return err
}

func (r connectionRepo) Delete(ctx context.Context, id string) error {
	res, err := r.ex.ExecContext(ctx, `DELETE FROM connections WHERE id = ?`, id)
	if err != nil {
		return err
	}

	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return connections.ErrNotFound
	}

	return nil
}

func (r connectionRepo) Touch(ctx context.Context, id string, atMs int64) error {
	_, err := r.ex.ExecContext(ctx, `UPDATE connections SET last_used_ms = ? WHERE id = ?`, atMs, id)

	return err
}
