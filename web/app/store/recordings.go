package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/recordings"

	"github.com/AlexGalhardo/vukem/database"
)

// recordingRepo is recordings.Store over the table recordings (SDD-005 section 5.18).
type recordingRepo struct{ ex database.Executor }

const recordingColumns = `id, project_id, state, review, source_kind, source_label, width, height, fps, encoder,
 started_ms, ended_ms, duration_ms, frames, dropped, bytes, stop_reason, error_code, error_message, asset_hash,
 asset_id, actor, created_ms, tracks, pauses, events_count, plan, marks, proposals`

// scanRecording reads a recording and opens its source label, the sealed column (SDD-005 section 5.17.2).
func scanRecording(sc interface{ Scan(...any) error }, sl account.Sealer) (recordings.Row, error) {
	var r recordings.Row

	err := sc.Scan(&r.ID, &r.ProjectID, &r.State, &r.Review, &r.SourceKind, &r.SourceLabel, &r.Width, &r.Height,
		&r.FPS, &r.Encoder, &r.StartedMs, &r.EndedMs, &r.DurationMs, &r.Frames, &r.Dropped, &r.Bytes, &r.StopReason,
		&r.ErrorCode, &r.ErrorMessage, &r.AssetHash, &r.AssetID, &r.Actor, &r.CreatedMs, &r.Tracks, &r.Pauses,
		&r.EventsCount, &r.Plan, &r.Marks, &r.Proposals)
	if err != nil {
		return r, err
	}

	r.SourceLabel, err = openField(sl, "recordings", "source_label", r.ID, r.SourceLabel)

	return r, err
}

func (r recordingRepo) Insert(ctx context.Context, row recordings.Row) error {
	label, err := sealField(sealerOf(r.ex), "recordings", "source_label", row.ID, row.SourceLabel)
	if err != nil {
		return err
	}

	_, err = r.ex.ExecContext(
		ctx,
		`INSERT INTO recordings (`+recordingColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID,
		row.ProjectID,
		row.State,
		row.Review,
		row.SourceKind,
		label,
		row.Width,
		row.Height,
		row.FPS,
		row.Encoder,
		row.StartedMs,
		row.EndedMs,
		row.DurationMs,
		row.Frames,
		row.Dropped,
		row.Bytes,
		row.StopReason,
		row.ErrorCode,
		row.ErrorMessage,
		row.AssetHash,
		row.AssetID,
		row.Actor,
		row.CreatedMs,
		jsonListOrEmpty(row.Tracks),
		jsonListOrEmpty(row.Pauses),
		row.EventsCount,
		row.Plan,
		jsonListOrEmpty(row.Marks),
		jsonListOrEmpty(row.Proposals),
	)

	return err
}

func (r recordingRepo) Get(ctx context.Context, id string) (recordings.Row, error) {
	row, err := scanRecording(
		r.ex.QueryRowContext(ctx, `SELECT `+recordingColumns+` FROM recordings WHERE id = ?`, id), sealerOf(r.ex))
	if errors.Is(err, sql.ErrNoRows) {
		return row, recordings.ErrNotFound
	}

	return row, err
}

func (r recordingRepo) Find(ctx context.Context, id string) (recordings.Row, error) {
	rows, err := r.list(ctx, false, `id = ?`, id)
	if err != nil {
		return recordings.Row{}, err
	}

	if len(rows) == 0 {
		return recordings.Row{}, recordings.ErrNotFound
	}

	return rows[0], nil
}

// Update never writes the source label: it is set once, at the insert, and a read that did not open it (the
// recovery of start-up while the studio is locked) must not be able to blank it.
func (r recordingRepo) Update(ctx context.Context, row recordings.Row) error {
	res, err := r.ex.ExecContext(ctx,
		`UPDATE recordings SET state = ?, review = ?, width = ?, height = ?, encoder = ?,
 started_ms = ?, ended_ms = ?, duration_ms = ?, frames = ?, dropped = ?, bytes = ?, stop_reason = ?, error_code = ?,
 error_message = ?, asset_hash = ?, asset_id = ?, tracks = ?, pauses = ?, events_count = ?, plan = ?, marks = ?,
 proposals = ? WHERE id = ?`,
		row.State, row.Review, row.Width, row.Height, row.Encoder, row.StartedMs, row.EndedMs, row.DurationMs,
		row.Frames, row.Dropped, row.Bytes, row.StopReason, row.ErrorCode, row.ErrorMessage, row.AssetHash,
		row.AssetID, jsonListOrEmpty(row.Tracks), jsonListOrEmpty(row.Pauses), row.EventsCount, row.Plan,
		jsonListOrEmpty(row.Marks), jsonListOrEmpty(row.Proposals), row.ID)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return recordings.ErrNotFound
	}

	return nil
}

func (r recordingRepo) list(
	ctx context.Context, openLabel bool, where string, args ...any,
) ([]recordings.Row, error) {
	rows, err := r.ex.QueryContext(ctx,
		`SELECT `+recordingColumns+` FROM recordings WHERE `+where+` ORDER BY created_ms DESC, id`, args...)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []recordings.Row

	for rows.Next() {
		sl := sealerOf(r.ex)
		if !openLabel {
			sl = account.Passthrough{}
		}

		row, err := scanRecording(rows, sl)
		if err == nil && !openLabel {
			row.SourceLabel = ""
		}

		if err != nil {
			return nil, err
		}

		out = append(out, row)
	}

	return out, rows.Err()
}

func (r recordingRepo) ListByProject(ctx context.Context, projectID string) ([]recordings.Row, error) {
	return r.list(ctx, true, `project_id = ?`, projectID)
}

func (r recordingRepo) ListByAssetHash(ctx context.Context, hash string) ([]recordings.Row, error) {
	return r.list(ctx, false, `asset_hash = ?`, hash)
}

func (r recordingRepo) Open(ctx context.Context) ([]recordings.Row, error) {
	// The label stays sealed: the recovery of start-up runs while the studio may be locked and needs none.
	return r.list(
		ctx,
		false,
		`state IN ('prepared','recording','paused','finishing') AND NOT (state = 'prepared' AND plan <> '')`,
	)
}

// jsonListOrEmpty is the list a row keeps: an empty string is the empty list.
func jsonListOrEmpty(s string) string {
	if s == "" {
		return "[]"
	}

	return s
}
