package store

import (
	"context"
	"encoding/json"

	"github.com/AlexGalhardo/vukem/database"

	"github.com/AlexGalhardo/vukem-video/service"
)

type idempotencyRepo struct{ ex database.Executor }

func (r idempotencyRepo) Get(
	ctx context.Context,
	projectID, actor, key string,
	nowMs int64,
) (service.IdempotencyRow, error) {
	var (
		row    service.IdempotencyRow
		result string
	)
	err := r.ex.QueryRowContext(ctx, `SELECT project_id, actor, key, request_hash, revision, result, created_ms, expires_ms
 FROM idempotency_keys WHERE project_id = ? AND actor = ? AND key = ? AND expires_ms > ?`,
		projectID, actor, key, nowMs).
		Scan(&row.ProjectID, &row.Actor, &row.Key, &row.RequestHash,
			&row.Revision, &result, &row.CreatedMs, &row.ExpiresMs)
	if err != nil {
		return service.IdempotencyRow{}, mapNoRows(err)
	}
	row.Result = json.RawMessage(result)
	return row, nil
}

// Put inserts the row, or replaces one with the same key that has expired.
// A live row with the key is left alone and ErrExists is returned.
func (r idempotencyRepo) Put(ctx context.Context, row service.IdempotencyRow) error {
	n, err := rowsAffected(r.ex.ExecContext(
		ctx,
		`INSERT INTO idempotency_keys
 (project_id, actor, key, request_hash, revision, result, created_ms, expires_ms)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT (project_id, actor, key) DO UPDATE SET
   request_hash = excluded.request_hash, revision = excluded.revision, result = excluded.result,
   created_ms = excluded.created_ms, expires_ms = excluded.expires_ms
 WHERE idempotency_keys.expires_ms <= excluded.created_ms`,
		row.ProjectID,
		row.Actor,
		row.Key,
		row.RequestHash,
		row.Revision,
		string(row.Result),
		row.CreatedMs,
		row.ExpiresMs,
	))
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrExists
	}
	return nil
}

func (r idempotencyRepo) PurgeProjectExpired(ctx context.Context, projectID string, nowMs int64) (int64, error) {
	return rowsAffected(r.ex.ExecContext(ctx,
		"DELETE FROM idempotency_keys WHERE project_id = ? AND expires_ms <= ?", projectID, nowMs))
}

func (r idempotencyRepo) CountLive(
	ctx context.Context,
	projectID, actor string,
	nowMs int64,
) (n int, firstExpiresMs int64, err error) {
	err = r.ex.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(MIN(expires_ms), 0) FROM idempotency_keys
 WHERE project_id = ? AND actor = ? AND expires_ms > ?`, projectID, actor, nowMs).Scan(&n, &firstExpiresMs)

	return n, firstExpiresMs, err
}

func (r idempotencyRepo) PurgeExpired(ctx context.Context, nowMs int64) (int64, error) {
	return rowsAffected(r.ex.ExecContext(ctx, "DELETE FROM idempotency_keys WHERE expires_ms <= ?", nowMs))
}
