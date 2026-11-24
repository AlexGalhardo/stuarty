package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/database"
)

type imageRepo struct{ ex database.Executor }

const imageHeadColumns = `id, name, template, revision, schema_version, width, height, layers, created_ms, updated_ms, created_by, owner_user_id`

func scanImageHead(sc interface{ Scan(...any) error }, h *service.ImageHead, extra ...any) error {
	var (
		tpl   int
		owner sql.NullString
	)

	dest := append([]any{
		&h.ID, &h.Name, &tpl, &h.Revision, &h.SchemaVersion, &h.Width, &h.Height, &h.Layers, &h.CreatedMs, &h.UpdatedMs,
		&h.CreatedBy, &owner,
	}, extra...)

	if err := sc.Scan(dest...); err != nil {
		return err
	}

	h.Template = tpl == 1
	h.OwnerUserID = ptrString(owner)

	return nil
}

func (r imageRepo) Create(ctx context.Context, row service.ImageRow) error {
	doc, err := sealDoc(r.ex, "image_documents", "document", row.ID, string(row.Document))
	if err != nil {
		return err
	}

	_, err = r.ex.ExecContext(ctx,
		`INSERT INTO image_documents (`+imageHeadColumns+`, document, doc_sha256)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID, row.Name, boolInt(row.Template), row.Revision, row.SchemaVersion, row.Width, row.Height, row.Layers,
		row.CreatedMs, row.UpdatedMs, row.CreatedBy, row.OwnerUserID, doc, row.DocSHA256,
	)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return service.ErrExists
	}

	return err
}

func (r imageRepo) Get(ctx context.Context, id string) (service.ImageRow, error) {
	var (
		row service.ImageRow
		doc string
	)

	err := scanImageHead(r.ex.QueryRowContext(ctx,
		`SELECT `+imageHeadColumns+`, document, doc_sha256 FROM image_documents WHERE id = ?`, id,
	), &row.ImageHead, &doc, &row.DocSHA256)
	if err != nil {
		return service.ImageRow{}, mapNoRows(err)
	}

	plain, err := openDoc(r.ex, "image_documents", "document", id, doc)
	if err != nil {
		return service.ImageRow{}, err
	}

	row.Document = []byte(plain)

	return row, nil
}

func (r imageRepo) List(ctx context.Context, q service.ImageQuery) ([]service.ImageHead, error) {
	where, args := []string{"1 = 1"}, []any{}

	if q.OwnerUserID != "" {
		// A template that ships with the product has no owner and is every user's while it is still the document
		// that shipped, at revision 1: this is service.SharedImageTemplate in SQL (VV-300).
		where = append(
			where,
			"(owner_user_id = ? OR (template = 1 AND owner_user_id IS NULL AND created_by = ? AND revision = 1))",
		)
		args = append(args, q.OwnerUserID, service.StarterActor)
	}

	switch q.Templates {
	case "only":
		where = append(where, "template = 1")
	case "none":
		where = append(where, "template = 0")
	}

	args = append(args, q.Limit+1, q.Offset)

	rows, err := r.ex.QueryContext(ctx,
		`SELECT `+imageHeadColumns+` FROM image_documents WHERE `+strings.Join(where, " AND ")+
			` ORDER BY updated_ms DESC, id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []service.ImageHead

	for rows.Next() {
		var h service.ImageHead
		if err := scanImageHead(rows, &h); err != nil {
			return nil, err
		}

		out = append(out, h)
	}

	return out, rows.Err()
}

func (r imageRepo) Commit(ctx context.Context, id string, from int64, u service.ImageUpdate) error {
	doc, err := sealDoc(r.ex, "image_documents", "document", id, string(u.Document))
	if err != nil {
		return err
	}

	n, err := rowsAffected(r.ex.ExecContext(ctx,
		`UPDATE image_documents SET name = ?, template = ?, revision = revision + 1, schema_version = ?, document = ?,
 doc_sha256 = ?, width = ?, height = ?, layers = ?, updated_ms = ? WHERE id = ? AND revision = ?`,
		u.Name, boolInt(u.Template), u.SchemaVersion, doc, u.DocSHA256, u.Width, u.Height, u.Layers,
		u.UpdatedMs, id, from,
	))
	if err != nil {
		return err
	}

	if n == 0 {
		return service.ErrStale
	}

	return nil
}

const imageOpColumns = `image_id, revision, kind, actor, base_revision, target_revision, at_ms, ops, inverse, diff, skipped,
 doc_sha256, closed_by, idempotency_key, request_hash, result`

// opKey is the row key of an entry of an edit log: it binds the sealed columns to their document and revision, so
// a row copied to another revision or another document does not open (ADR-0053).
func opKey(id string, revision int64) string { return id + "/" + strconv.FormatInt(revision, 10) }

// sealNullable seals a nullable column: no value stays NULL, so the absence of a value is still readable.
func sealNullable(ex database.Executor, table, column, key string, v json.RawMessage) (any, error) {
	if len(v) == 0 {
		return nil, nil
	}

	return sealDoc(ex, table, column, key, string(v))
}

// openNullable opens a nullable column that was read as a NullString.
func openNullable(ex database.Executor, table, column, key string, v sql.NullString) (json.RawMessage, error) {
	if !v.Valid {
		return nil, nil
	}

	plain, err := openDoc(ex, table, column, key, v.String)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(plain), nil
}

func (r imageRepo) AppendOp(ctx context.Context, op service.ImageOpRow) error {
	const table = "image_operations"

	key := opKey(op.ImageID, op.Revision)
	cols := make([]string, 3)

	for i, v := range []struct{ column, plain string }{
		{"ops", string(op.Ops)}, {"inverse", string(op.Inverse)}, {"diff", string(op.Diff)},
	} {
		var err error
		if cols[i], err = sealDoc(r.ex, table, v.column, key, v.plain); err != nil {
			return err
		}
	}

	skipped, err := sealNullable(r.ex, table, "skipped", key, op.Skipped)
	if err != nil {
		return err
	}

	result, err := sealNullable(r.ex, table, "result", key, op.Result)
	if err != nil {
		return err
	}

	_, err = r.ex.ExecContext(ctx,
		`INSERT INTO image_operations (`+imageOpColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		op.ImageID, op.Revision, string(op.Kind), op.Actor, op.BaseRevision, op.TargetRevision, op.AtMs, cols[0],
		cols[1], cols[2], skipped, op.DocSHA256, op.ClosedBy, op.IdempotencyKey,
		nullString(op.RequestHash), result,
	)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return service.ErrExists
	}

	return err
}

func scanImageOp(ex database.Executor, sc interface{ Scan(...any) error }) (service.ImageOpRow, error) {
	var (
		op                            service.ImageOpRow
		kind, ops, inverse, diff      string
		target, closed                sql.NullInt64
		skipped, key, reqHash, result sql.NullString
	)

	err := sc.Scan(&op.ImageID, &op.Revision, &kind, &op.Actor, &op.BaseRevision, &target, &op.AtMs, &ops, &inverse,
		&diff, &skipped, &op.DocSHA256, &closed, &key, &reqHash, &result)
	if err != nil {
		return service.ImageOpRow{}, err
	}

	const table = "image_operations"

	rk := opKey(op.ImageID, op.Revision)
	plain := [3]string{ops, inverse, diff}

	for i, column := range []string{"ops", "inverse", "diff"} {
		var oerr error
		if plain[i], oerr = openDoc(ex, table, column, rk, plain[i]); oerr != nil {
			return service.ImageOpRow{}, oerr
		}
	}

	var oerr error

	if op.Skipped, oerr = openNullable(ex, table, "skipped", rk, skipped); oerr != nil {
		return service.ImageOpRow{}, oerr
	}

	if op.Result, oerr = openNullable(ex, table, "result", rk, result); oerr != nil {
		return service.ImageOpRow{}, oerr
	}

	op.Kind, op.Ops, op.Inverse, op.Diff = service.OperationKind(
		kind,
	), []byte(
		plain[0],
	), []byte(
		plain[1],
	), []byte(
		plain[2],
	)
	op.TargetRevision, op.ClosedBy, op.IdempotencyKey = ptrInt64(target), ptrInt64(closed), ptrString(key)
	op.RequestHash = reqHash.String

	return op, nil
}

func (r imageRepo) OpsSince(ctx context.Context, id string, revision int64) ([]service.ImageOpRow, error) {
	return r.OpsPage(ctx, id, revision, -1) // a negative limit is no limit in SQLite
}

func (r imageRepo) OpsPage(ctx context.Context, id string, revision int64, limit int) ([]service.ImageOpRow, error) {
	rows, err := r.ex.QueryContext(
		ctx,
		`SELECT `+imageOpColumns+
			` FROM image_operations WHERE image_id = ? AND revision > ? ORDER BY revision LIMIT ?`,
		id,
		revision,
		limit,
	)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []service.ImageOpRow

	for rows.Next() {
		op, err := scanImageOp(r.ex, rows)
		if err != nil {
			return nil, err
		}

		out = append(out, op)
	}

	return out, rows.Err()
}

func (r imageRepo) LastOpen(
	ctx context.Context,
	id, actor string,
	kinds []service.OperationKind,
) (service.ImageOpRow, error) {
	marks := make([]string, len(kinds))
	args := []any{id, actor}

	for i, k := range kinds {
		marks[i] = "?"
		args = append(args, string(k))
	}

	op, err := scanImageOp(r.ex, r.ex.QueryRowContext(ctx,
		`SELECT `+imageOpColumns+` FROM image_operations
 WHERE image_id = ? AND actor = ? AND revision > 1 AND closed_by IS NULL AND kind IN (`+strings.Join(marks, ",")+`)
 ORDER BY revision DESC LIMIT 1`, args...))
	if err != nil {
		return service.ImageOpRow{}, mapNoRows(err)
	}

	return op, nil
}

func (r imageRepo) Close(ctx context.Context, id string, revision, by int64) error {
	n, err := rowsAffected(r.ex.ExecContext(ctx,
		`UPDATE image_operations SET closed_by = ? WHERE image_id = ? AND revision = ? AND closed_by IS NULL`,
		by, id, revision))
	if err != nil {
		return err
	}

	if n == 0 {
		return service.ErrStale
	}

	return nil
}

func (r imageRepo) CloseOpenRedo(ctx context.Context, id, actor string, by int64) error {
	_, err := r.ex.ExecContext(
		ctx,
		`UPDATE image_operations SET closed_by = ? WHERE image_id = ? AND actor = ? AND kind = 'undo' AND closed_by IS NULL`,
		by,
		id,
		actor,
	)

	return err
}

func (r imageRepo) ByKey(ctx context.Context, id, actor, key string) (service.ImageOpRow, error) {
	op, err := scanImageOp(r.ex, r.ex.QueryRowContext(ctx,
		`SELECT `+imageOpColumns+` FROM image_operations WHERE image_id = ? AND actor = ? AND idempotency_key = ?`,
		id, actor, key))
	if err != nil {
		return service.ImageOpRow{}, mapNoRows(err)
	}

	return op, nil
}
