package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/database"
)

type operationRepo struct{ ex database.Executor }

const operationColumns = `project_id, revision, kind, actor, base_revision, target_revision, at_ms,
 ops, inverse, diff, skipped, schema_version, doc_sha256, closed_by, idempotency_key`

func scanOperation(sc scanner) (service.OperationRow, error) {
	var (
		o                    service.OperationRow
		target, closed       sql.NullInt64
		ops, inverse, diff   string
		skipped, idempotency sql.NullString
	)
	err := sc.Scan(&o.ProjectID, &o.Revision, &o.Kind, &o.Actor, &o.BaseRevision, &target, &o.AtMs,
		&ops, &inverse, &diff, &skipped, &o.SchemaVersion, &o.DocSHA256, &closed, &idempotency)
	if err != nil {
		return service.OperationRow{}, err
	}
	o.TargetRevision, o.ClosedBy, o.IdempotencyKey = ptrInt64(target), ptrInt64(closed), ptrString(idempotency)
	o.Ops, o.Inverse, o.Diff, o.Skipped = json.RawMessage(
		ops,
	), json.RawMessage(
		inverse,
	), json.RawMessage(
		diff,
	), rawJSON(
		skipped,
	)
	return o, nil
}

// Count implements service.OperationRepo.
func (r operationRepo) Count(ctx context.Context, projectID, actor string) (int64, error) {
	var n int64

	err := r.ex.QueryRowContext(
		ctx,
		"SELECT count(*) FROM operations WHERE project_id = ? AND (? = '' OR actor = ?)",
		projectID,
		actor,
		actor,
	).Scan(&n)

	return n, err
}

func (r operationRepo) Append(ctx context.Context, o service.OperationRow) error {
	_, err := r.ex.ExecContext(ctx, `INSERT INTO operations (`+operationColumns+`)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.ProjectID, o.Revision, string(o.Kind), o.Actor, o.BaseRevision, o.TargetRevision, o.AtMs,
		string(o.Ops), string(o.Inverse), string(o.Diff), nullJSON(o.Skipped), o.SchemaVersion, o.DocSHA256,
		o.ClosedBy, o.IdempotencyKey)
	if isUnique(r.ex, err) {
		return service.ErrExists
	}
	return err
}

func (r operationRepo) Get(ctx context.Context, projectID string, revision int64) (service.OperationRow, error) {
	o, err := scanOperation(r.ex.QueryRowContext(ctx,
		"SELECT "+operationColumns+" FROM operations WHERE project_id = ? AND revision = ?", projectID, revision))
	return o, mapNoRows(err)
}

const defaultSinceLimit = 1000

func (r operationRepo) Since(
	ctx context.Context,
	projectID string,
	after int64,
	f service.OperationFilter,
) ([]service.OperationRow, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultSinceLimit
	}

	query := "SELECT " + operationColumns + " FROM operations WHERE project_id = ? AND revision > ?"
	args := []any{projectID, after}
	if f.ExcludeActor != "" {
		query += " AND actor <> ?"
		args = append(args, f.ExcludeActor)
	}
	query += " ORDER BY revision ASC LIMIT ?"
	args = append(args, limit)

	rows, err := r.ex.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		out   []service.OperationRow
		bytes int
	)
	for rows.Next() {
		o, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
		bytes += len(o.Ops) + len(o.Inverse) + len(o.Diff)
		if f.MaxBytes > 0 && bytes > f.MaxBytes {
			break
		}
	}
	return out, rows.Err()
}

func (r operationRepo) Page(
	ctx context.Context,
	projectID string,
	q service.HistoryQuery,
) ([]service.OperationRow, bool, error) {
	limit := min(max(q.Limit, 1), maxProjectLimit)

	rows, err := r.ex.QueryContext(ctx, `SELECT `+operationColumns+` FROM operations
 WHERE project_id = ? AND (? = '' OR actor = ?) AND (? = 0 OR revision < ?)
 ORDER BY revision DESC LIMIT ?`,
		projectID, q.Actor, q.Actor, q.Before, q.Before, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var out []service.OperationRow
	for rows.Next() {
		o, err := scanOperation(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// The WHERE of both candidate queries repeats the text of the partial index
// literally, because SQLite uses a partial index only when the query's WHERE
// implies the index's (SDD-005 section 12.1).

func (r operationRepo) UndoCandidate(ctx context.Context, projectID, actor string) (service.OperationRow, error) {
	o, err := scanOperation(r.ex.QueryRowContext(ctx, `SELECT `+operationColumns+` FROM operations
 WHERE project_id = ? AND actor = ? AND closed_by IS NULL AND kind IN ('edit','redo')
 ORDER BY revision DESC LIMIT 1`, projectID, actor))
	return o, mapNoRows(err)
}

func (r operationRepo) RedoCandidate(ctx context.Context, projectID, actor string) (service.OperationRow, error) {
	o, err := scanOperation(r.ex.QueryRowContext(ctx, `SELECT `+operationColumns+` FROM operations
 WHERE project_id = ? AND actor = ? AND closed_by IS NULL AND kind = 'undo'
 ORDER BY revision DESC LIMIT 1`, projectID, actor))
	return o, mapNoRows(err)
}

// The three UPDATE statements below are the only ones on the log, and each
// only moves closed_by from NULL to a revision (RF-134, SDD-005 section 5.3).

func (r operationRepo) Close(ctx context.Context, projectID string, revision, by int64) error {
	n, err := rowsAffected(r.ex.ExecContext(
		ctx,
		"UPDATE operations SET closed_by = ? WHERE project_id = ? AND revision = ? AND closed_by IS NULL",
		by,
		projectID,
		revision,
	))
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrStale
	}
	return nil
}

func (r operationRepo) CloseOpenRedo(ctx context.Context, projectID, actor string, by int64) error {
	_, err := r.ex.ExecContext(
		ctx,
		"UPDATE operations SET closed_by = ? WHERE project_id = ? AND actor = ? AND closed_by IS NULL AND kind = 'undo'",
		by,
		projectID,
		actor,
	)
	return err
}

func (r operationRepo) CloseAllOpen(ctx context.Context, projectID string, by int64) error {
	_, err := r.ex.ExecContext(ctx,
		"UPDATE operations SET closed_by = ? WHERE project_id = ? AND closed_by IS NULL", by, projectID)
	return err
}
