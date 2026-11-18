package store

import (
	"context"
	"errors"

	"github.com/AlexGalhardo/vukem/database"

	"github.com/AlexGalhardo/vukem-video/service"
)

type fontRepo struct{ ex database.Executor }

func (r fontRepo) Record(ctx context.Context, owner, hash string, atMs int64) error {
	_, err := r.ex.ExecContext(
		ctx,
		`INSERT INTO font_owners (hash, owner_user_id, created_ms) VALUES (?, ?, ?)
 ON CONFLICT (hash, owner_user_id) DO NOTHING`,
		hash, owner, atMs,
	)

	return err
}

func (r fontRepo) Owns(ctx context.Context, owner, hash string) (bool, error) {
	var one int

	err := r.ex.QueryRowContext(
		ctx, `SELECT 1 FROM font_owners WHERE hash = ? AND owner_user_id = ?`, hash, owner,
	).Scan(&one)
	if err != nil {
		if errors.Is(mapNoRows(err), service.ErrNotFound) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (r fontRepo) Owned(ctx context.Context, owner string) ([]string, error) {
	rows, err := r.ex.QueryContext(
		ctx, `SELECT hash FROM font_owners WHERE owner_user_id = ? ORDER BY created_ms, hash`, owner,
	)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []string

	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}

		out = append(out, h)
	}

	return out, rows.Err()
}

type brandKitRepo struct{ ex database.Executor }

func (r brandKitRepo) Get(ctx context.Context, owner string) (service.BrandKitRow, error) {
	var (
		row service.BrandKitRow
		doc string
	)

	err := r.ex.QueryRowContext(
		ctx,
		`SELECT owner_user_id, document, updated_ms, updated_by FROM brand_kits WHERE owner_user_id = ?`,
		owner,
	).Scan(&row.OwnerUserID, &doc, &row.UpdatedMs, &row.UpdatedBy)
	if err != nil {
		return service.BrandKitRow{}, mapNoRows(err)
	}

	row.Document = []byte(doc)

	return row, nil
}

func (r brandKitRepo) Put(ctx context.Context, row service.BrandKitRow) error {
	_, err := r.ex.ExecContext(
		ctx,
		`INSERT INTO brand_kits (owner_user_id, document, updated_ms, updated_by) VALUES (?, ?, ?, ?)
 ON CONFLICT (owner_user_id) DO UPDATE SET
  document = excluded.document, updated_ms = excluded.updated_ms, updated_by = excluded.updated_by`,
		row.OwnerUserID, string(row.Document), row.UpdatedMs, row.UpdatedBy,
	)

	return err
}
