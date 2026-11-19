package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/database"
)

type templateRepo struct{ ex database.Executor }

const templateListColumns = `id, name, description, schema_version, doc_sha256, slots, width, height, fps_num, fps_den,
 duration_frames, source_project_id, created_ms, created_by, owner_user_id`

func scanTemplate(sc scanner, doc *string) (service.TemplateRow, error) {
	var (
		t      service.TemplateRow
		slots  string
		source sql.NullString
		owner  sql.NullString
	)
	dest := []any{
		&t.ID, &t.Name, &t.Description, &t.SchemaVersion, &t.DocSHA256, &slots,
		&t.Summary.Width, &t.Summary.Height, &t.Summary.FPSNum, &t.Summary.FPSDen,
		&t.Summary.DurationFrames, &source, &t.CreatedMs, &t.CreatedBy, &owner,
	}
	if doc != nil {
		dest = append(dest, doc)
	}
	if err := sc.Scan(dest...); err != nil {
		return service.TemplateRow{}, err
	}
	t.Slots, t.SourceProjectID, t.OwnerUserID = json.RawMessage(slots), ptrString(source), ptrString(owner)
	return t, nil
}

func (r templateRepo) Create(ctx context.Context, t service.TemplateRow) error {
	s := t.Summary
	_, err := r.ex.ExecContext(
		ctx,
		`INSERT INTO templates
 (id, name, description, schema_version, document, doc_sha256, slots, width, height, fps_num, fps_den,
  duration_frames, source_project_id, created_ms, created_by, owner_user_id)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID,
		t.Name,
		t.Description,
		t.SchemaVersion,
		string(t.Document),
		t.DocSHA256,
		string(t.Slots),
		s.Width,
		s.Height,
		s.FPSNum,
		s.FPSDen,
		s.DurationFrames,
		t.SourceProjectID,
		t.CreatedMs,
		t.CreatedBy,
		t.OwnerUserID,
	)
	if isUnique(r.ex, err) {
		return service.ErrExists
	}
	return err
}

func (r templateRepo) Get(ctx context.Context, id string) (service.TemplateRow, error) {
	var doc string
	t, err := scanTemplate(r.ex.QueryRowContext(ctx,
		"SELECT "+templateListColumns+", document FROM templates WHERE id = ?", id), &doc)
	if err != nil {
		return service.TemplateRow{}, mapNoRows(err)
	}
	t.Document = []byte(doc)
	return t, nil
}

const defaultTemplateLimit = 50

// List pages by created_ms descending with the id as tie breaker. The cursor
// is the id of the last row returned; the document is left empty.
func (r templateRepo) List(
	ctx context.Context,
	limit int,
	after, ownerUserID string,
) ([]service.TemplateRow, string, error) {
	if limit <= 0 {
		limit = defaultTemplateLimit
	}
	query := "SELECT " + templateListColumns + " FROM templates"
	var (
		args  []any
		where []string
	)
	if ownerUserID != "" {
		// The starter templates have no owner and belong to everyone.
		where = append(where, "(owner_user_id = ? OR (owner_user_id IS NULL AND created_by = ?))")
		args = append(args, ownerUserID, service.StarterActor)
	}
	if after != "" {
		// The cursor row is looked up inside the same owner filter, so another
		// owner's id is a cursor that matches nothing.
		sub := "SELECT created_ms, id FROM templates WHERE id = ?"
		args = append(args, after)
		if ownerUserID != "" {
			sub += " AND (owner_user_id = ? OR (owner_user_id IS NULL AND created_by = ?))"
			args = append(args, ownerUserID, service.StarterActor)
		}
		where = append(where, "(created_ms, id) < ("+sub+")")
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_ms DESC, id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := r.ex.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var out []service.TemplateRow
	for rows.Next() {
		t, err := scanTemplate(rows, nil)
		if err != nil {
			return nil, "", err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[len(out)-1].ID
	}
	return out, next, nil
}

func (r templateRepo) UpdateDocument(
	ctx context.Context,
	id string,
	schemaVersion int,
	doc []byte,
	sha string,
	slots json.RawMessage,
	sum service.DocSummary,
) error {
	return notFoundIfNone(rowsAffected(r.ex.ExecContext(
		ctx,
		`UPDATE templates SET
 schema_version = ?, document = ?, doc_sha256 = ?, slots = ?, width = ?, height = ?, fps_num = ?, fps_den = ?,
 duration_frames = ? WHERE id = ?`,
		schemaVersion,
		string(doc),
		sha,
		string(slots),
		sum.Width,
		sum.Height,
		sum.FPSNum,
		sum.FPSDen,
		sum.DurationFrames,
		id,
	)))
}

type backupRepo struct{ ex database.Executor }

func (r backupRepo) Put(ctx context.Context, b service.BackupRow) error {
	_, err := r.ex.ExecContext(ctx, `INSERT INTO document_backups
 (owner_kind, owner_id, revision, schema_version, document, reason, created_ms)
 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.OwnerKind, b.OwnerID, b.Revision, b.SchemaVersion, string(b.Document), b.Reason, b.CreatedMs)
	return err
}

func (r backupRepo) List(ctx context.Context, ownerKind, ownerID string) ([]service.BackupRow, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT id, owner_kind, owner_id, revision, schema_version, reason, created_ms
 FROM document_backups WHERE owner_kind = ? AND owner_id = ? ORDER BY id`, ownerKind, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []service.BackupRow
	for rows.Next() {
		var b service.BackupRow
		if err := rows.Scan(
			&b.ID,
			&b.OwnerKind,
			&b.OwnerID,
			&b.Revision,
			&b.SchemaVersion,
			&b.Reason,
			&b.CreatedMs,
		); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r backupRepo) Get(ctx context.Context, id int64) (service.BackupRow, error) {
	var (
		b   service.BackupRow
		doc string
	)
	err := r.ex.QueryRowContext(ctx, `SELECT id, owner_kind, owner_id, revision, schema_version, document, reason, created_ms
 FROM document_backups WHERE id = ?`, id).
		Scan(&b.ID, &b.OwnerKind, &b.OwnerID, &b.Revision, &b.SchemaVersion, &doc, &b.Reason, &b.CreatedMs)
	if err != nil {
		return service.BackupRow{}, mapNoRows(err)
	}
	b.Document = []byte(doc)
	return b, nil
}

type auditRepo struct{ ex database.Executor }

func (r auditRepo) Append(ctx context.Context, a service.AuditRow) error {
	_, err := r.ex.ExecContext(ctx, `INSERT INTO audit (at_ms, actor, action, project_id, subject_id, details)
 VALUES (?, ?, ?, ?, ?, ?)`, a.AtMs, a.Actor, a.Action, a.ProjectID, a.SubjectID, nullJSON(a.Details))
	return err
}

const defaultAuditLimit = 100

// List returns rows newest first. beforeID 0 starts from the newest.
func (r auditRepo) List(ctx context.Context, projectID string, beforeID int64, limit int) ([]service.AuditRow, error) {
	if limit <= 0 {
		limit = defaultAuditLimit
	}
	rows, err := r.ex.QueryContext(ctx, `SELECT id, at_ms, actor, action, project_id, subject_id, details FROM audit
 WHERE (? = '' OR project_id = ?) AND (? = 0 OR id < ?) ORDER BY id DESC LIMIT ?`,
		projectID, projectID, beforeID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []service.AuditRow
	for rows.Next() {
		var (
			a                service.AuditRow
			project, subject sql.NullString
			details          sql.NullString
		)
		if err := rows.Scan(&a.ID, &a.AtMs, &a.Actor, &a.Action, &project, &subject, &details); err != nil {
			return nil, err
		}
		a.ProjectID, a.SubjectID, a.Details = ptrString(project), ptrString(subject), rawJSON(details)
		out = append(out, a)
	}
	return out, rows.Err()
}
