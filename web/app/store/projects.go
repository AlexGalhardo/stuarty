package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/database"
)

type projectRepo struct{ ex database.Executor }

const projectHeadColumns = `id, name, state, revision, schema_version, width, height, fps_num, fps_den,
 duration_frames, track_count, item_count, asset_count, created_ms, updated_ms, created_by,
 owner_user_id, trashed_ms, trashed_by, resync_floor`

type scanner interface{ Scan(dest ...any) error }

func scanProjectHead(sc scanner, extra ...any) (service.ProjectHead, error) {
	var (
		h        service.ProjectHead
		owner    sql.NullString
		trashedM sql.NullInt64
		trashedB sql.NullString
	)
	dest := []any{
		&h.ID, &h.Name, &h.State, &h.Revision, &h.SchemaVersion,
		&h.Summary.Width, &h.Summary.Height, &h.Summary.FPSNum, &h.Summary.FPSDen,
		&h.Summary.DurationFrames, &h.Summary.Tracks, &h.Summary.Items, &h.Summary.Assets,
		&h.CreatedMs, &h.UpdatedMs, &h.CreatedBy, &owner, &trashedM, &trashedB, &h.ResyncFloor,
	}
	if err := sc.Scan(append(dest, extra...)...); err != nil {
		return service.ProjectHead{}, err
	}
	h.OwnerUserID, h.TrashedMs, h.TrashedBy = ptrString(owner), ptrInt64(trashedM), ptrString(trashedB)
	return h, nil
}

const defaultProjectLimit, maxProjectLimit = 50, 200

// projectFilter is the WHERE of a listing without its cursor: the state, the
// owner and the single project, all in SQL so that pages and totals agree.
func projectFilter(q service.ProjectQuery) ([]string, []any) {
	var (
		where []string
		args  []any
	)
	switch {
	case q.IncludeTrashed:
	case q.State != "":
		where, args = append(where, "state = ?"), append(args, string(q.State))
	default:
		where = append(where, "state = 'active'")
	}
	if q.OwnerUserID != "" {
		where, args = append(where, "owner_user_id = ?"), append(args, q.OwnerUserID)
	}
	if q.ProjectID != "" {
		// A key limited to one project reaches every timeline of it (SDD-003 section 1.7 rule 4), not only the
		// one whose id is the project's.
		where = append(where,
			"(id = ? OR id IN (SELECT i.ref FROM project_items i WHERE i.kind = 'timeline' AND i.project_id = ?))")
		args = append(args, q.ProjectID, q.ProjectID)
	}
	return where, args
}

func (r projectRepo) List(ctx context.Context, q service.ProjectQuery) ([]service.ProjectHead, string, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultProjectLimit
	}
	limit = min(limit, maxProjectLimit)

	where, args := projectFilter(q)
	if q.After != "" {
		ms, id, err := decodeCursor(q.After)
		if err != nil {
			return nil, "", err
		}
		where, args = append(where, "(updated_ms, id) < (?, ?)"), append(args, ms, id)
	}

	query := "SELECT " + projectHeadColumns + " FROM timelines"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY updated_ms DESC, id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := r.ex.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var out []service.ProjectHead
	for rows.Next() {
		h, err := scanProjectHead(rows)
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

// ErrBadCursor is returned for a cursor the store did not produce.
var ErrBadCursor = service.ErrBadCursor

// Count implements service.ProjectRepo.
func (r projectRepo) Count(ctx context.Context, q service.ProjectQuery) (int, error) {
	where, args := projectFilter(q)
	query := "SELECT count(*) FROM timelines"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	var n int
	err := r.ex.QueryRowContext(ctx, query, args...).Scan(&n)

	return n, err
}

func encodeCursor(ms int64, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(ms, 10) + ":" + id))
}

func decodeCursor(c string) (int64, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return 0, "", ErrBadCursor
	}
	msText, id, ok := strings.Cut(string(raw), ":")
	ms, perr := strconv.ParseInt(msText, 10, 64)
	if !ok || perr != nil || id == "" {
		return 0, "", ErrBadCursor
	}
	return ms, id, nil
}

func (r projectRepo) Head(ctx context.Context, id string) (service.ProjectHead, error) {
	h, err := scanProjectHead(
		r.ex.QueryRowContext(ctx, "SELECT "+projectHeadColumns+" FROM timelines WHERE id = ?", id),
	)
	return h, mapNoRows(err)
}

func (r projectRepo) Get(ctx context.Context, id string) (service.ProjectRow, error) {
	var doc, sha string
	h, err := scanProjectHead(r.ex.QueryRowContext(ctx,
		"SELECT "+projectHeadColumns+", document, doc_sha256 FROM timelines WHERE id = ?", id), &doc, &sha)
	if err != nil {
		return service.ProjectRow{}, mapNoRows(err)
	}
	return service.ProjectRow{ProjectHead: h, Document: []byte(doc), DocSHA256: sha}, nil
}

func (r projectRepo) Create(ctx context.Context, row service.ProjectRow) error {
	s := row.Summary
	_, err := r.ex.ExecContext(
		ctx,
		`INSERT INTO timelines
 (id, name, state, revision, schema_version, document, doc_sha256, width, height, fps_num, fps_den,
  duration_frames, track_count, item_count, asset_count, resync_floor, created_ms, updated_ms,
  created_by, owner_user_id, trashed_ms, trashed_by)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID,
		row.Name,
		stateOrActive(row.State),
		row.Revision,
		row.SchemaVersion,
		string(row.Document),
		row.DocSHA256,
		s.Width,
		s.Height,
		s.FPSNum,
		s.FPSDen,
		s.DurationFrames,
		s.Tracks,
		s.Items,
		s.Assets,
		row.ResyncFloor,
		row.CreatedMs,
		row.UpdatedMs,
		row.CreatedBy,
		row.OwnerUserID,
		row.TrashedMs,
		row.TrashedBy,
	)
	if isUnique(r.ex, err) {
		return service.ErrExists
	}
	return err
}

func stateOrActive(s service.ProjectState) string {
	if s == "" {
		return string(service.ProjectActive)
	}
	return string(s)
}

func (r projectRepo) Commit(ctx context.Context, id string, expected int64, u service.ProjectUpdate) error {
	s := u.Summary
	n, err := rowsAffected(r.ex.ExecContext(ctx, `UPDATE timelines SET
 name = ?, schema_version = ?, document = ?, doc_sha256 = ?,
 width = ?, height = ?, fps_num = ?, fps_den = ?, duration_frames = ?,
 track_count = ?, item_count = ?, asset_count = ?, updated_ms = ?,
 resync_floor = COALESCE(?, resync_floor), revision = revision + 1
 WHERE id = ? AND revision = ?`,
		u.Name, u.SchemaVersion, string(u.Document), u.DocSHA256,
		s.Width, s.Height, s.FPSNum, s.FPSDen, s.DurationFrames,
		s.Tracks, s.Items, s.Assets, u.UpdatedMs, u.ResyncFloor, id, expected))
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrStale
	}
	// The project that holds this timeline rises in the list. Its name is its own, changed by ProjectUpdate and
	// never by a commit of a timeline (SDD-005 section 5.12.2). A timeline without a project, as the store tests
	// make, changes nothing here.
	_, err = r.ex.ExecContext(ctx, `UPDATE projects SET updated_ms = ?
 WHERE id = (SELECT project_id FROM project_items WHERE kind = 'timeline' AND ref = ?)`,
		u.UpdatedMs, id)
	return err
}

func (r projectRepo) SetState(ctx context.Context, id string, st service.ProjectState, by string, atMs int64) error {
	var (
		n   int64
		err error
	)
	if st == service.ProjectTrashed {
		n, err = rowsAffected(r.ex.ExecContext(
			ctx,
			"UPDATE timelines SET state = 'trashed', trashed_ms = ?, trashed_by = ?, updated_ms = ? WHERE id = ?",
			atMs,
			by,
			atMs,
			id,
		))
	} else {
		n, err = rowsAffected(r.ex.ExecContext(
			ctx,
			"UPDATE timelines SET state = 'active', trashed_ms = NULL, trashed_by = NULL, updated_ms = ? WHERE id = ?",
			atMs,
			id,
		))
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrNotFound
	}
	return nil
}

func (r projectRepo) Delete(ctx context.Context, id string) error {
	if _, err := r.ex.ExecContext(
		ctx,
		"DELETE FROM document_backups WHERE owner_kind = 'project' AND owner_id = ?",
		id,
	); err != nil {
		return err
	}
	n, err := rowsAffected(r.ex.ExecContext(ctx, "DELETE FROM timelines WHERE id = ?", id))
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrNotFound
	}
	return nil
}
