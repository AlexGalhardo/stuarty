package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/database"
)

type productionRepo struct{ ex database.Executor }

// firstTimeline is the ref of the first active timeline item of the project p.
const firstTimeline = `(SELECT i.ref FROM project_items i
 WHERE i.project_id = p.id AND i.kind = 'timeline' AND i.state = 'active' ORDER BY i.position, i.id LIMIT 1)`

const productionColumns = `p.id, p.name, p.state, p.created_ms, p.updated_ms, p.created_by, p.owner_user_id,
 p.trashed_ms, p.trashed_by,
 (SELECT count(*) FROM project_items i WHERE i.project_id = p.id AND i.kind = 'timeline' AND i.state = 'active'),
 t.id, t.name, t.state, t.revision, t.schema_version, t.width, t.height, t.fps_num, t.fps_den,
 t.duration_frames, t.track_count, t.item_count, t.asset_count, t.created_ms, t.updated_ms, t.created_by,
 t.owner_user_id, t.trashed_ms, t.trashed_by, t.resync_floor`

const productionFrom = ` FROM projects p LEFT JOIN timelines t ON t.id = ` + firstTimeline

func scanProduction(sc scanner) (service.ProductionHead, error) {
	var (
		h                       service.ProductionHead
		owner, trashedBy        sql.NullString
		trashedMs               sql.NullInt64
		tID, tName, tState      sql.NullString
		tRev, tSchema           sql.NullInt64
		tW, tH, tFN, tFD        sql.NullInt64
		tDur, tTracks, tItems   sql.NullInt64
		tAssets, tCreated, tUpd sql.NullInt64
		tBy, tOwner, tTrashedBy sql.NullString
		tTrashedMs, tResync     sql.NullInt64
	)

	err := sc.Scan(&h.ID, &h.Name, &h.State, &h.CreatedMs, &h.UpdatedMs, &h.CreatedBy, &owner, &trashedMs, &trashedBy,
		&h.Timelines, &tID, &tName, &tState, &tRev, &tSchema, &tW, &tH, &tFN, &tFD, &tDur, &tTracks, &tItems,
		&tAssets, &tCreated, &tUpd, &tBy, &tOwner, &tTrashedMs, &tTrashedBy, &tResync)
	if err != nil {
		return service.ProductionHead{}, err
	}

	h.OwnerUserID, h.TrashedMs, h.TrashedBy = ptrString(owner), ptrInt64(trashedMs), ptrString(trashedBy)

	if tID.Valid {
		h.Timeline = service.ProjectHead{
			ID: tID.String, Name: tName.String, State: service.ProjectState(tState.String), Revision: tRev.Int64,
			SchemaVersion: int(tSchema.Int64),
			Summary: service.DocSummary{
				Width: int(tW.Int64), Height: int(tH.Int64), FPSNum: int(tFN.Int64), FPSDen: int(tFD.Int64),
				DurationFrames: tDur.Int64, Tracks: int(tTracks.Int64), Items: int(tItems.Int64),
				Assets: int(tAssets.Int64),
			},
			CreatedMs: tCreated.Int64, UpdatedMs: tUpd.Int64, CreatedBy: tBy.String,
			OwnerUserID: ptrString(tOwner), TrashedMs: ptrInt64(tTrashedMs), TrashedBy: ptrString(tTrashedBy),
			ResyncFloor: tResync.Int64,
		}
	}

	return h, nil
}

// productionFilter is the WHERE of a listing without its cursor, over the alias p.
func productionFilter(q service.ProductionQuery) ([]string, []any) {
	var (
		where []string
		args  []any
	)

	switch {
	case q.IncludeTrashed:
	case q.State != "":
		where, args = append(where, "p.state = ?"), append(args, string(q.State))
	default:
		where = append(where, "p.state = 'active'")
	}

	if q.OwnerUserID != "" {
		where, args = append(where, "p.owner_user_id = ?"), append(args, q.OwnerUserID)
	}

	if q.ProjectID != "" {
		where, args = append(where, "p.id = ?"), append(args, q.ProjectID)
	}

	return where, args
}

func (r productionRepo) List(ctx context.Context, q service.ProductionQuery) ([]service.ProductionHead, string, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultProjectLimit
	}

	limit = min(limit, maxProjectLimit)

	where, args := productionFilter(q)

	if q.After != "" {
		ms, id, err := decodeCursor(q.After)
		if err != nil {
			return nil, "", err
		}

		where, args = append(where, "(p.updated_ms, p.id) < (?, ?)"), append(args, ms, id)
	}

	query := "SELECT " + productionColumns + productionFrom
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	query += " ORDER BY p.updated_ms DESC, p.id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := r.ex.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var out []service.ProductionHead

	for rows.Next() {
		h, err := scanProduction(rows)
		if err != nil {
			return nil, "", err
		}

		out = append(out, h)
	}

	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	next := ""

	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		next = encodeCursor(last.UpdatedMs, last.ID)
	}

	return out, next, nil
}

func (r productionRepo) Count(ctx context.Context, q service.ProductionQuery) (int, error) {
	where, args := productionFilter(q)
	query := "SELECT count(*) FROM projects p"

	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	var n int

	err := r.ex.QueryRowContext(ctx, query, args...).Scan(&n)

	return n, err
}

func (r productionRepo) Get(ctx context.Context, id string) (service.ProductionHead, error) {
	h, err := scanProduction(
		r.ex.QueryRowContext(ctx, "SELECT "+productionColumns+productionFrom+" WHERE p.id = ?", id),
	)

	return h, mapNoRows(err)
}

func (r productionRepo) Create(ctx context.Context, h service.ProductionHead) error {
	_, err := r.ex.ExecContext(
		ctx,
		`INSERT INTO projects
 (id, name, state, created_ms, updated_ms, created_by, owner_user_id, trashed_ms, trashed_by)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID,
		h.Name,
		stateOrActive(h.State),
		h.CreatedMs,
		h.UpdatedMs,
		h.CreatedBy,
		h.OwnerUserID,
		h.TrashedMs,
		h.TrashedBy,
	)
	if isUnique(r.ex, err) {
		return service.ErrExists
	}

	return err
}

func (r productionRepo) SetState(ctx context.Context, id string, st service.ProjectState, by string, atMs int64) error {
	var (
		n    int64
		err  error
		mirr string
		args []any
	)

	if st == service.ProjectTrashed {
		n, err = rowsAffected(r.ex.ExecContext(ctx,
			"UPDATE projects SET state = 'trashed', trashed_ms = ?, trashed_by = ?, updated_ms = ? WHERE id = ?",
			atMs, by, atMs, id))
		mirr = "UPDATE timelines SET state = 'trashed', trashed_ms = ?, trashed_by = ?, updated_ms = ?"
		args = []any{atMs, by, atMs}
	} else {
		n, err = rowsAffected(r.ex.ExecContext(ctx,
			"UPDATE projects SET state = 'active', trashed_ms = NULL, trashed_by = NULL, updated_ms = ? WHERE id = ?",
			atMs, id))
		mirr = "UPDATE timelines SET state = 'active', trashed_ms = NULL, trashed_by = NULL, updated_ms = ?"
		args = []any{atMs}
	}

	if err != nil {
		return err
	}

	if n == 0 {
		return service.ErrNotFound
	}

	_, err = r.ex.ExecContext(
		ctx,
		mirr+" WHERE id IN (SELECT ref FROM project_items WHERE project_id = ? AND kind = 'timeline' AND state = 'active')",
		append(args, id)...)

	return err
}

func (r productionRepo) Touch(ctx context.Context, id string, atMs int64) error {
	n, err := rowsAffected(r.ex.ExecContext(ctx, "UPDATE projects SET updated_ms = ? WHERE id = ?", atMs, id))
	if err != nil {
		return err
	}

	if n == 0 {
		return service.ErrNotFound
	}

	return nil
}

func (r productionRepo) Delete(ctx context.Context, id string) error {
	n, err := rowsAffected(r.ex.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", id))
	if err != nil {
		return err
	}

	if n == 0 {
		return service.ErrNotFound
	}

	return nil
}

const itemColumns = `id, project_id, kind, title, position, state, ref, body, format, created_ms, updated_ms, created_by, trashed_ms`

func scanItem(sc scanner) (service.ItemRow, error) {
	var (
		it        service.ItemRow
		ref, form sql.NullString
		trashed   sql.NullInt64
	)

	err := sc.Scan(&it.ID, &it.ProjectID, &it.Kind, &it.Title, &it.Position, &it.State, &ref, &it.Body, &form,
		&it.CreatedMs, &it.UpdatedMs, &it.CreatedBy, &trashed)
	if err != nil {
		return service.ItemRow{}, err
	}

	it.Ref, it.Format, it.TrashedMs = ref.String, form.String, ptrInt64(trashed)

	return it, nil
}

func (r productionRepo) Items(ctx context.Context, projectID string) ([]service.ItemRow, error) {
	rows, err := r.ex.QueryContext(ctx,
		"SELECT "+itemColumns+" FROM project_items WHERE project_id = ? ORDER BY position, id", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []service.ItemRow

	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, it)
	}

	return out, rows.Err()
}

func (r productionRepo) Item(ctx context.Context, itemID string) (service.ItemRow, error) {
	it, err := scanItem(r.ex.QueryRowContext(ctx, "SELECT "+itemColumns+" FROM project_items WHERE id = ?", itemID))

	return it, mapNoRows(err)
}

func (r productionRepo) CreateItem(ctx context.Context, it service.ItemRow) error {
	_, err := r.ex.ExecContext(ctx, `INSERT INTO project_items
 (id, project_id, kind, title, position, state, ref, body, format, created_ms, updated_ms, created_by, trashed_ms)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.ID, it.ProjectID, it.Kind, it.Title, it.Position, stateOrActive(it.State), nullString(it.Ref), it.Body,
		nullString(it.Format), it.CreatedMs, it.UpdatedMs, it.CreatedBy, it.TrashedMs)
	if isUnique(r.ex, err) {
		return service.ErrExists
	}

	return err
}

func (r productionRepo) UpdateItem(ctx context.Context, it service.ItemRow) error {
	n, err := rowsAffected(r.ex.ExecContext(ctx,
		"UPDATE project_items SET title = ?, body = ?, state = ?, trashed_ms = ?, updated_ms = ? WHERE id = ?",
		it.Title, it.Body, stateOrActive(it.State), it.TrashedMs, it.UpdatedMs, it.ID))
	if err != nil {
		return err
	}

	if n == 0 {
		return service.ErrNotFound
	}

	return nil
}

func (r productionRepo) SetPositions(ctx context.Context, projectID string, ids []string) error {
	for i, id := range ids {
		if _, err := r.ex.ExecContext(ctx, "UPDATE project_items SET position = ? WHERE id = ? AND project_id = ?",
			i, id, projectID); err != nil {
			return err
		}
	}

	return nil
}

func (r productionRepo) ProjectOfTimeline(ctx context.Context, timelineID string) (string, error) {
	var id string

	err := r.ex.QueryRowContext(ctx,
		"SELECT project_id FROM project_items WHERE kind = 'timeline' AND ref = ?", timelineID).Scan(&id)

	return id, mapNoRows(err)
}

func (r productionRepo) ActiveTimelines(ctx context.Context, projectID string) ([]service.TimelineRef, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT id, ref, title FROM project_items
 WHERE project_id = ? AND kind = 'timeline' AND state = 'active' ORDER BY position, id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []service.TimelineRef

	for rows.Next() {
		var t service.TimelineRef
		if err := rows.Scan(&t.ItemID, &t.TimelineID, &t.Title); err != nil {
			return nil, err
		}

		out = append(out, t)
	}

	return out, rows.Err()
}
