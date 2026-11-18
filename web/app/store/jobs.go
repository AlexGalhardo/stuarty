package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/AlexGalhardo/vukem/database"

	"github.com/AlexGalhardo/vukem-video/service"
)

type jobRepo struct{ ex database.Executor }

const jobColumns = `id, kind, project_id, asset_hash, actor, state, progress, message, input, result, error,
 cancel_requested, attempts, created_ms, updated_ms, started_ms, finished_ms`

func scanJob(sc scanner) (service.JobRow, error) {
	var (
		j                 service.JobRow
		project, asset    sql.NullString
		input             string
		result, errJSON   sql.NullString
		cancel            int
		started, finished sql.NullInt64
	)
	err := sc.Scan(&j.ID, &j.Kind, &project, &asset, &j.Actor, &j.State, &j.Progress, &j.Message, &input,
		&result, &errJSON, &cancel, &j.Attempts, &j.CreatedMs, &j.UpdatedMs, &started, &finished)
	if err != nil {
		return service.JobRow{}, err
	}
	j.ProjectID, j.AssetHash = ptrString(project), ptrString(asset)
	j.Input, j.Result, j.Error = json.RawMessage(input), rawJSON(result), rawJSON(errJSON)
	j.CancelRequested = cancel != 0
	j.StartedMs, j.FinishedMs = ptrInt64(started), ptrInt64(finished)
	return j, nil
}

// CreateLimited checks the limits and inserts in the same call; run it inside
// Write so that check and insert are atomic (SDD-005 section 14.2).
func (r jobRepo) CreateLimited(ctx context.Context, j service.JobRow, lim *service.JobLimit) error {
	if lim != nil {
		if lim.Max > 0 {
			ids, err := r.activeIDs(
				ctx,
				"SELECT id FROM jobs WHERE kind = ? AND state IN ('queued','running') ORDER BY created_ms, id",
				lim.Kind,
			)
			if err != nil {
				return err
			}
			if len(ids) >= lim.Max {
				return &service.QueueFullError{Running: ids, Kind: lim.Kind, Ceiling: lim.Max}
			}
		}
		if lim.Actor != "" && lim.MaxPerActor > 0 {
			ids, err := r.activeIDs(
				ctx,
				"SELECT id FROM jobs WHERE kind = ? AND state IN ('queued','running') AND actor = ? ORDER BY created_ms, id",
				lim.Kind,
				lim.Actor,
			)
			if err != nil {
				return err
			}
			if len(ids) >= lim.MaxPerActor {
				return &service.QueueFullError{Running: ids, Kind: lim.Kind, Ceiling: lim.MaxPerActor, PerActor: true}
			}
		}
	}

	_, err := r.ex.ExecContext(ctx, `INSERT INTO jobs
 (id, kind, project_id, asset_hash, actor, state, progress, message, input, created_ms, updated_ms)
 VALUES (?, ?, ?, ?, ?, 'queued', 0, ?, ?, ?, ?)`,
		j.ID, j.Kind, j.ProjectID, j.AssetHash, j.Actor, j.Message, string(j.Input), j.CreatedMs, j.UpdatedMs)
	if isUnique(r.ex, err) {
		return service.ErrExists
	}
	return err
}

func (r jobRepo) activeIDs(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := r.ex.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r jobRepo) Get(ctx context.Context, id string) (service.JobRow, error) {
	j, err := scanJob(r.ex.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id = ?", id))
	return j, mapNoRows(err)
}

const defaultJobListLimit = 100

func (r jobRepo) List(ctx context.Context, projectID string, states []string, limit int) ([]service.JobRow, error) {
	if limit <= 0 {
		limit = defaultJobListLimit
	}
	query := "SELECT " + jobColumns + " FROM jobs WHERE project_id = ?"
	args := []any{projectID}
	if len(states) > 0 {
		query += " AND state IN (?" + strings.Repeat(", ?", len(states)-1) + ")"
		for _, s := range states {
			args = append(args, s)
		}
	}
	query += " ORDER BY created_ms DESC, id DESC LIMIT ?"
	args = append(args, limit)
	return r.query(ctx, query, args...)
}

func (r jobRepo) query(ctx context.Context, query string, args ...any) ([]service.JobRow, error) {
	rows, err := r.ex.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []service.JobRow
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (r jobRepo) ActiveIngestForHash(ctx context.Context, hash string) (service.JobRow, error) {
	j, err := scanJob(r.ex.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs
 WHERE asset_hash = ? AND kind = 'ingest' AND state IN ('queued','running') LIMIT 1`, hash))
	return j, mapNoRows(err)
}

// Claim, Progress, Finish and Requeue are single conditional updates
// (SDD-005 section 14.3); false means the row was not in the expected state.

func (r jobRepo) Claim(ctx context.Context, id string, nowMs int64) (bool, error) {
	n, err := rowsAffected(r.ex.ExecContext(ctx, `UPDATE jobs SET state = 'running', started_ms = ?, updated_ms = ?,
 attempts = attempts + 1, progress = 0, message = ''
 WHERE id = ? AND state = 'queued' AND cancel_requested = 0`, nowMs, nowMs, id))
	return n == 1, err
}

func (r jobRepo) Progress(ctx context.Context, id string, progress float64, message string, nowMs int64) (bool, error) {
	n, err := rowsAffected(r.ex.ExecContext(
		ctx,
		"UPDATE jobs SET progress = ?, message = ?, updated_ms = ? WHERE id = ? AND state = 'running'",
		progress,
		message,
		nowMs,
		id,
	))
	return n == 1, err
}

func (r jobRepo) Finish(
	ctx context.Context,
	id, state string,
	result, errJSON json.RawMessage,
	nowMs int64,
) (bool, error) {
	// A job that succeeded is complete; the others keep the progress they reached.
	n, err := rowsAffected(r.ex.ExecContext(ctx, `UPDATE jobs SET state = ?,
 progress = CASE WHEN ? = 'succeeded' THEN 1 ELSE progress END,
 result = ?, error = ?, finished_ms = ?, updated_ms = ?
 WHERE id = ? AND state = 'running'`, state, state, nullJSON(result), nullJSON(errJSON), nowMs, nowMs, id))
	return n == 1, err
}

func (r jobRepo) Requeue(ctx context.Context, id, message string, nowMs int64) (bool, error) {
	n, err := rowsAffected(r.ex.ExecContext(ctx, `UPDATE jobs SET state = 'queued', message = ?, progress = 0,
 started_ms = NULL, updated_ms = ? WHERE id = ? AND state = 'running'`, message, nowMs, id))
	return n == 1, err
}

// cancelledError is the typed error stored on a job cancelled before it ran.
const cancelledError = `{"code":"E_CANCELLED","message":"The job was cancelled.","retryable":false}`

func (r jobRepo) RequestCancel(ctx context.Context, id string, nowMs int64) (service.JobRow, error) {
	if _, err := r.ex.ExecContext(
		ctx,
		"UPDATE jobs SET cancel_requested = 1, updated_ms = ? WHERE id = ? AND state IN ('queued','running')",
		nowMs,
		id,
	); err != nil {
		return service.JobRow{}, err
	}
	// A queued job never runs once cancel_requested is set (Claim refuses it),
	// so it can be finished here.
	if _, err := r.ex.ExecContext(
		ctx,
		"UPDATE jobs SET state = 'cancelled', error = ?, finished_ms = ?, updated_ms = ? WHERE id = ? AND state = 'queued' AND cancel_requested = 1",
		cancelledError,
		nowMs,
		nowMs,
		id,
	); err != nil {
		return service.JobRow{}, err
	}
	return r.Get(ctx, id)
}

func (r jobRepo) Running(ctx context.Context) ([]service.JobRow, error) {
	return r.query(ctx, "SELECT "+jobColumns+" FROM jobs WHERE state = 'running' ORDER BY created_ms, id")
}

func (r jobRepo) QueuedBefore(ctx context.Context, createdBeforeMs int64) ([]service.JobRow, error) {
	return r.query(
		ctx,
		"SELECT "+jobColumns+" FROM jobs WHERE state = 'queued' AND created_ms < ? ORDER BY created_ms, id",
		createdBeforeMs,
	)
}
