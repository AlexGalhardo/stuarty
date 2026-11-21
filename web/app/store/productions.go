package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/database"
)

type productionRepo struct{ ex database.Executor }

// firstTimeline is the ref of the first active timeline item of the project p.
const firstTimeline = `(SELECT i.ref FROM project_items i
 WHERE i.project_id = p.id AND i.kind = 'timeline' AND i.state = 'active' ORDER BY i.position, i.id LIMIT 1)`

const productionColumns = `p.id, p.name, p.state, p.created_ms, p.updated_ms, p.created_by, p.owner_user_id,
 p.trashed_ms, p.trashed_by, p.description, p.goal_specific, p.goal_measurable, p.goal_achievable, p.goal_relevant,
 p.goal_time_bound, p.goal_due,
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

	err := sc.Scan(
		&h.ID,
		&h.Name,
		&h.State,
		&h.CreatedMs,
		&h.UpdatedMs,
		&h.CreatedBy,
		&owner,
		&trashedMs,
		&trashedBy,
		&h.Description,
		&h.Goal.Specific,
		&h.Goal.Measurable,
		&h.Goal.Achievable,
		&h.Goal.Relevant,
		&h.Goal.TimeBound,
		&h.Goal.Due,
		&h.Timelines,
		&tID,
		&tName,
		&tState,
		&tRev,
		&tSchema,
		&tW,
		&tH,
		&tFN,
		&tFD,
		&tDur,
		&tTracks,
		&tItems,
		&tAssets,
		&tCreated,
		&tUpd,
		&tBy,
		&tOwner,
		&tTrashedMs,
		&tTrashedBy,
		&tResync,
	)
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
 (id, name, state, created_ms, updated_ms, created_by, owner_user_id, trashed_ms, trashed_by, description,
 goal_specific, goal_measurable, goal_achievable, goal_relevant, goal_time_bound, goal_due)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID,
		h.Name,
		stateOrActive(h.State),
		h.CreatedMs,
		h.UpdatedMs,
		h.CreatedBy,
		h.OwnerUserID,
		h.TrashedMs,
		h.TrashedBy,
		h.Description,
		h.Goal.Specific,
		h.Goal.Measurable,
		h.Goal.Achievable,
		h.Goal.Relevant,
		h.Goal.TimeBound,
		h.Goal.Due,
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

func (r productionRepo) SetContext(ctx context.Context, id string, c project.Context, atMs int64) error {
	n, err := rowsAffected(r.ex.ExecContext(ctx,
		`UPDATE projects SET name = ?, description = ?, goal_specific = ?, goal_measurable = ?, goal_achievable = ?,
 goal_relevant = ?, goal_time_bound = ?, goal_due = ?, updated_ms = ? WHERE id = ?`,
		c.Name, c.Description, c.Goal.Specific, c.Goal.Measurable, c.Goal.Achievable, c.Goal.Relevant,
		c.Goal.TimeBound, c.Goal.Due, atMs, id))
	if err != nil {
		return err
	}

	if n == 0 {
		return service.ErrNotFound
	}

	return nil
}

func (r productionRepo) FollowName(ctx context.Context, id, from, to string) error {
	_, err := r.ex.ExecContext(ctx, "UPDATE projects SET name = ? WHERE id = ? AND name = ?", to, id, from)

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
	if err := deleteProjectScoped(ctx, r.ex, id); err != nil {
		return err
	}

	n, err := rowsAffected(r.ex.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", id))
	if err != nil {
		return err
	}

	if n == 0 {
		return service.ErrNotFound
	}

	return nil
}

const itemColumns = `id, project_id, kind, title, position, state, ref, body, format, created_ms, updated_ms, created_by, trashed_ms, text_kind`

func scanItem(sc scanner) (service.ItemRow, error) {
	var (
		it        service.ItemRow
		ref, form sql.NullString
		textKind  sql.NullString
		trashed   sql.NullInt64
	)

	err := sc.Scan(&it.ID, &it.ProjectID, &it.Kind, &it.Title, &it.Position, &it.State, &ref, &it.Body, &form,
		&it.CreatedMs, &it.UpdatedMs, &it.CreatedBy, &trashed, &textKind)
	if err != nil {
		return service.ItemRow{}, err
	}

	it.Ref, it.Format, it.TrashedMs, it.TextKind = ref.String, form.String, ptrInt64(trashed), textKind.String

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
	if it.Kind == "text" && it.TextKind == "" {
		it.TextKind = "copy"
	}

	_, err := r.ex.ExecContext(ctx, `INSERT INTO project_items
 (id, project_id, kind, title, position, state, ref, body, format, created_ms, updated_ms, created_by, trashed_ms,
 text_kind)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.ID, it.ProjectID, it.Kind, it.Title, it.Position, stateOrActive(it.State), nullString(it.Ref), it.Body,
		nullString(it.Format), it.CreatedMs, it.UpdatedMs, it.CreatedBy, it.TrashedMs, nullString(it.TextKind))
	if isUnique(r.ex, err) {
		return service.ErrExists
	}

	if err != nil || it.Kind != "text" {
		return err
	}

	// A text item never exists without a revision: its first one is the body it was made with (SDD-019 section 3).
	_, err = r.ex.ExecContext(ctx, `INSERT INTO text_revisions (item_id, revision, body, actor, source, created_ms)
 VALUES (?, 1, ?, ?, 'created', ?)`, it.ID, it.Body, it.CreatedBy, it.CreatedMs)

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

const (
	revisionColumns = `item_id, revision, actor, source, reverted_to, provider, model, cost_micros, currency, job_id,
 created_ms`
	defaultTextLimit = 50
	maxTextLimit     = 100
)

func scanRevision(sc scanner, withBody bool) (service.TextRevisionRow, error) {
	var (
		r                                service.TextRevisionRow
		reverted, cost                   sql.NullInt64
		provider, model, currency, jobID sql.NullString
		body                             string
		dest                             []any
	)

	dest = append(dest, &r.ItemID, &r.Revision)
	if withBody {
		dest = append(dest, &body)
	}

	dest = append(dest, &r.Actor, &r.Source, &reverted, &provider, &model, &cost, &currency, &jobID, &r.CreatedMs)

	if err := sc.Scan(dest...); err != nil {
		return service.TextRevisionRow{}, err
	}

	r.Body, r.RevertedTo, r.CostMicros = body, ptrInt(reverted), ptrInt64(cost)
	r.Provider, r.Model, r.Currency, r.JobID = provider.String, model.String, currency.String, jobID.String

	return r, nil
}

const revisionWithBody = `item_id, revision, body, actor, source, reverted_to, provider, model, cost_micros, currency,
 job_id, created_ms`

func (r productionRepo) TextRevisions(
	ctx context.Context, itemID string, before, limit int,
) ([]service.TextRevisionRow, error) {
	if limit <= 0 {
		limit = defaultTextLimit
	}

	if before <= 0 {
		before = int(^uint(0) >> 1)
	}

	rows, err := r.ex.QueryContext(ctx, "SELECT "+revisionColumns+
		" FROM text_revisions WHERE item_id = ? AND revision < ? ORDER BY revision DESC LIMIT ?",
		itemID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []service.TextRevisionRow

	for rows.Next() {
		rev, err := scanRevision(rows, false)
		if err != nil {
			return nil, err
		}

		out = append(out, rev)
	}

	return out, rows.Err()
}

func (r productionRepo) TextRevision(
	ctx context.Context,
	itemID string,
	revision int,
) (service.TextRevisionRow, error) {
	rev, err := scanRevision(r.ex.QueryRowContext(ctx, "SELECT "+revisionWithBody+
		" FROM text_revisions WHERE item_id = ? AND revision = ?", itemID, revision), true)

	return rev, mapNoRows(err)
}

func (r productionRepo) TextLatest(ctx context.Context, itemID string) (service.TextRevisionRow, error) {
	rev, err := scanRevision(r.ex.QueryRowContext(ctx, "SELECT "+revisionWithBody+
		" FROM text_revisions WHERE item_id = ? ORDER BY revision DESC LIMIT 1", itemID), true)

	return rev, mapNoRows(err)
}

func (r productionRepo) AddTextRevision(ctx context.Context, rev service.TextRevisionRow, pruneBefore int) error {
	_, err := r.ex.ExecContext(ctx, `INSERT INTO text_revisions
 (item_id, revision, body, actor, source, reverted_to, provider, model, cost_micros, currency, job_id, created_ms)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rev.ItemID, rev.Revision, rev.Body, rev.Actor, rev.Source, rev.RevertedTo, nullString(rev.Provider),
		nullString(rev.Model), rev.CostMicros, nullString(rev.Currency), nullString(rev.JobID), rev.CreatedMs)
	if isUnique(r.ex, err) {
		return service.ErrExists
	}

	if err != nil {
		return err
	}

	if pruneBefore > 0 {
		if _, err := r.ex.ExecContext(ctx, "DELETE FROM text_revisions WHERE item_id = ? AND revision < ?",
			rev.ItemID, pruneBefore); err != nil {
			return err
		}
	}

	n, err := rowsAffected(r.ex.ExecContext(ctx, "UPDATE project_items SET body = ?, updated_ms = ? WHERE id = ?",
		rev.Body, rev.CreatedMs, rev.ItemID))
	if err != nil {
		return err
	}

	if n == 0 {
		return service.ErrNotFound
	}

	return nil
}

func (r productionRepo) TextItems(ctx context.Context, q service.TextQuery) ([]service.TextHead, string, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultTextLimit
	}

	limit = min(limit, maxTextLimit)

	where := []string{"i.kind = 'text'", "p.state = 'active'"}
	args := []any{}

	state := string(q.State)
	if state == "" {
		state = string(service.ProjectActive)
	}

	where, args = append(where, "i.state = ?"), append(args, state)

	if q.ProjectID != "" {
		where, args = append(where, "p.id = ?"), append(args, q.ProjectID)
	}

	if q.OwnerUserID != "" {
		where, args = append(where, "p.owner_user_id = ?"), append(args, q.OwnerUserID)
	}

	if q.TextKind != "" {
		where, args = append(where, "i.text_kind = ?"), append(args, q.TextKind)
	}

	if q.After != "" {
		ms, id, err := decodeCursor(q.After)
		if err != nil {
			return nil, "", err
		}

		where, args = append(where, "(i.updated_ms, i.id) < (?, ?)"), append(args, ms, id)
	}

	cols := "i.id, i.project_id, i.kind, i.title, i.position, i.state, i.ref, i.body, i.format, i.created_ms," +
		" i.updated_ms, i.created_by, i.trashed_ms, i.text_kind"

	rows, err := r.ex.QueryContext(ctx, "SELECT "+cols+`, p.name, p.owner_user_id,
 COALESCE(r.revision, 0), COALESCE(r.actor, ''), COALESCE(r.source, '')
 FROM project_items i JOIN projects p ON p.id = i.project_id
 LEFT JOIN text_revisions r ON r.item_id = i.id
  AND r.revision = (SELECT max(x.revision) FROM text_revisions x WHERE x.item_id = i.id)
 WHERE `+strings.Join(where, " AND ")+" ORDER BY i.updated_ms DESC, i.id DESC LIMIT ?", append(args, limit+1)...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var out []service.TextHead

	for rows.Next() {
		var (
			h               service.TextHead
			ref, form, kind sql.NullString
			trashed         sql.NullInt64
			owner           sql.NullString
		)

		it := &h.Item
		if err := rows.Scan(&it.ID, &it.ProjectID, &it.Kind, &it.Title, &it.Position, &it.State, &ref, &it.Body, &form,
			&it.CreatedMs, &it.UpdatedMs, &it.CreatedBy, &trashed, &kind, &h.ProjectName, &owner, &h.Revision,
			&h.RevisionBy, &h.RevisionSrc); err != nil {
			return nil, "", err
		}

		it.Ref, it.Format, it.TrashedMs, it.TextKind = ref.String, form.String, ptrInt64(trashed), kind.String
		h.ProjectOwner = ptrString(owner)
		out = append(out, h)
	}

	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	next := ""

	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1].Item
		next = encodeCursor(last.UpdatedMs, last.ID)
	}

	return out, next, nil
}

// deleteProjectScoped deletes the rows that name a project or a timeline by id with no foreign key: the jobs, the
// links to the files and the keys limited to it (SDD-005 section 5.12.3). Emptying a project does it for the
// project and for each of its timelines, as the cascade of the foreign keys did before epic 56.
func deleteProjectScoped(ctx context.Context, ex database.Executor, id string) error {
	for _, q := range []string{
		"DELETE FROM jobs WHERE project_id = ?",
		"DELETE FROM project_assets WHERE project_id = ?",
		"DELETE FROM api_keys WHERE project_id = ?",
	} {
		if _, err := ex.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}

	return nil
}
