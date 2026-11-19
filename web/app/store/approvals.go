package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/approval"

	"github.com/AlexGalhardo/vukem/database"
)

// approvalRepo is approval.Store over the tables of SDD-015 section 3.
type approvalRepo struct{ ex database.Executor }

const requestColumns = `id, actor, owner_user_id, kind, summary, connection_id, project_id, job_id, payload_ref,
 estimate_micros, currency, state, decided_by, rule_id, reason, created_ms, decided_ms, expires_ms`

func scanRequest(sc scanner) (approval.Row, error) {
	var (
		r                                        approval.Row
		owner, conn, project, job, decider, rule sql.NullString
		decidedMs                                sql.NullInt64
	)

	err := sc.Scan(
		&r.ID, &r.Actor, &owner, &r.Kind, &r.Summary, &conn, &project, &job, &r.PayloadRef,
		&r.Estimate.Micros, &r.Estimate.Currency, &r.State, &decider, &rule, &r.Reason, &r.CreatedMs, &decidedMs,
		&r.ExpiresMs,
	)
	if err != nil {
		return r, err
	}

	r.OwnerUserID, r.ConnectionID, r.ProjectID = owner.String, conn.String, project.String
	r.JobID, r.DecidedBy, r.RuleID, r.DecidedMs = job.String, decider.String, rule.String, ptrInt64(decidedMs)

	return r, nil
}

func (r approvalRepo) InsertRequest(ctx context.Context, row *approval.Row) error {
	_, err := r.ex.ExecContext(ctx, `INSERT INTO approval_requests (`+requestColumns+`)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID, row.Actor, nullString(row.OwnerUserID), row.Kind, row.Summary, nullString(row.ConnectionID),
		nullString(row.ProjectID), nullString(row.JobID), row.PayloadRef, row.Estimate.Micros, row.Estimate.Currency,
		row.State, nullString(row.DecidedBy), nullString(row.RuleID), row.Reason, row.CreatedMs, row.DecidedMs,
		row.ExpiresMs)

	return err
}

func (r approvalRepo) GetRequest(ctx context.Context, id string) (approval.Row, error) {
	row, err := scanRequest(
		r.ex.QueryRowContext(ctx, `SELECT `+requestColumns+` FROM approval_requests WHERE id = ?`, id),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return row, approval.ErrNotFound
	}

	return row, err
}

func (r approvalRepo) RequestOfJob(ctx context.Context, jobID string) (approval.Row, error) {
	row, err := scanRequest(r.ex.QueryRowContext(
		ctx,
		`SELECT `+requestColumns+` FROM approval_requests WHERE job_id = ? ORDER BY created_ms DESC LIMIT 1`,
		jobID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return row, approval.ErrNotFound
	}

	return row, err
}

func (r approvalRepo) ListRequests(ctx context.Context, f approval.Filter) ([]approval.Row, int, error) {
	var (
		where []string
		args  []any
	)

	if f.State != "" {
		where, args = append(where, "state = ?"), append(args, f.State)
	}

	if f.ProjectID != "" {
		where, args = append(where, "project_id = ?"), append(args, f.ProjectID)
	}

	if f.OwnerUserID != "" {
		where, args = append(where, "owner_user_id = ?"), append(args, f.OwnerUserID)
	}

	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_requests`+clause, args...).
		Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.ex.QueryContext(ctx, `SELECT `+requestColumns+` FROM approval_requests`+clause+
		` ORDER BY created_ms DESC, id DESC LIMIT ? OFFSET ?`, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}

	defer func() { _ = rows.Close() }()

	var out []approval.Row

	for rows.Next() {
		row, err := scanRequest(rows)
		if err != nil {
			return nil, 0, err
		}

		out = append(out, row)
	}

	return out, total, rows.Err()
}

func (r approvalRepo) Decide(
	ctx context.Context,
	id, state, decidedBy, ruleID, reason string,
	atMs int64,
) (bool, error) {
	n, err := rowsAffected(r.ex.ExecContext(ctx, `UPDATE approval_requests
 SET state = ?, decided_by = ?, rule_id = ?, reason = ?, decided_ms = ? WHERE id = ? AND state = 'pending'`,
		state, nullString(decidedBy), nullString(ruleID), reason, atMs, id))

	return n == 1, err
}

func (r approvalRepo) Due(ctx context.Context, nowMs int64) ([]approval.Row, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT `+requestColumns+
		` FROM approval_requests WHERE state = 'pending' AND expires_ms <= ? ORDER BY created_ms, id`, nowMs)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []approval.Row

	for rows.Next() {
		row, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, row)
	}

	return out, rows.Err()
}

const ruleColumns = `id, kind, connection_id, project_id, currency, per_item_micros, per_day_micros, enabled, created_by, created_ms`

func scanRule(sc scanner) (approval.Rule, error) {
	var (
		r             approval.Rule
		conn, project sql.NullString
		item, day     sql.NullInt64
		enabled       int
	)

	if err := sc.Scan(
		&r.ID, &r.Kind, &conn, &project, &r.Currency, &item, &day, &enabled, &r.CreatedBy, &r.CreatedMs,
	); err != nil {
		return r, err
	}

	r.Connection, r.Project, r.PerItem, r.PerDay, r.Enabled = conn.String, project.String, ptrInt64(
		item,
	), ptrInt64(
		day,
	), enabled != 0

	return r, nil
}

func (r approvalRepo) ListRules(ctx context.Context) ([]approval.Rule, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT `+ruleColumns+` FROM approval_rules ORDER BY created_ms, id`)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []approval.Rule

	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, rule)
	}

	return out, rows.Err()
}

func (r approvalRepo) GetRule(ctx context.Context, id string) (approval.Rule, error) {
	rule, err := scanRule(r.ex.QueryRowContext(ctx, `SELECT `+ruleColumns+` FROM approval_rules WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return rule, approval.ErrNotFound
	}

	return rule, err
}

func (r approvalRepo) PutRule(ctx context.Context, rule *approval.Rule) error {
	_, err := r.ex.ExecContext(ctx, `INSERT INTO approval_rules (`+ruleColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT (id) DO UPDATE SET kind = excluded.kind, connection_id = excluded.connection_id,
 project_id = excluded.project_id, currency = excluded.currency, per_item_micros = excluded.per_item_micros,
 per_day_micros = excluded.per_day_micros, enabled = excluded.enabled`,
		rule.ID, rule.Kind, nullString(rule.Connection), nullString(rule.Project), rule.Currency, rule.PerItem,
		rule.PerDay, boolInt(rule.Enabled), rule.CreatedBy, rule.CreatedMs)

	return err
}

func (r approvalRepo) DeleteRule(ctx context.Context, id string) error {
	n, err := rowsAffected(r.ex.ExecContext(ctx, `DELETE FROM approval_rules WHERE id = ?`, id))
	if err == nil && n == 0 {
		return approval.ErrNotFound
	}

	return err
}

func (r approvalRepo) CountRules(ctx context.Context) (int, error) {
	var n int

	err := r.ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_rules`).Scan(&n)

	return n, err
}

func (r approvalRepo) ListCeilings(ctx context.Context) ([]approval.Ceiling, error) {
	rows, err := r.ex.QueryContext(
		ctx,
		`SELECT project_id, currency, per_item_micros, per_day_micros, updated_by, updated_ms
 FROM cost_ceilings ORDER BY project_id, currency`,
	)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []approval.Ceiling

	for rows.Next() {
		var (
			c         approval.Ceiling
			item, day sql.NullInt64
		)

		if err := rows.Scan(&c.Project, &c.Currency, &item, &day, &c.UpdatedBy, &c.UpdatedMs); err != nil {
			return nil, err
		}

		c.PerItem, c.PerDay = ptrInt64(item), ptrInt64(day)
		out = append(out, c)
	}

	return out, rows.Err()
}

func (r approvalRepo) PutCeiling(ctx context.Context, c *approval.Ceiling) error {
	_, err := r.ex.ExecContext(
		ctx,
		`INSERT INTO cost_ceilings (project_id, currency, per_item_micros, per_day_micros, updated_by, updated_ms)
 VALUES (?, ?, ?, ?, ?, ?)
 ON CONFLICT (project_id, currency) DO UPDATE SET per_item_micros = excluded.per_item_micros,
 per_day_micros = excluded.per_day_micros, updated_by = excluded.updated_by, updated_ms = excluded.updated_ms`,
		c.Project,
		c.Currency,
		c.PerItem,
		c.PerDay,
		c.UpdatedBy,
		c.UpdatedMs,
	)

	return err
}

func (r approvalRepo) DeleteCeiling(ctx context.Context, project, currency string) error {
	_, err := r.ex.ExecContext(
		ctx,
		`DELETE FROM cost_ceilings WHERE project_id = ? AND currency = ?`,
		project,
		currency,
	)

	return err
}

func (r approvalRepo) Reserve(ctx context.Context, e *approval.Entry) error {
	_, err := r.ex.ExecContext(
		ctx,
		`INSERT INTO cost_ledger
 (id, request_id, job_id, connection_id, project_id, kind, currency, estimate_micros, state, day, created_ms)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'reserved', ?, ?)`,
		e.ID,
		e.RequestID,
		e.JobID,
		e.ConnectionID,
		e.ProjectID,
		e.Kind,
		e.Currency,
		e.EstimateMicros,
		e.Day,
		e.CreatedMs,
	)

	return err
}

func (r approvalRepo) Settle(ctx context.Context, jobID string, actualMicros, atMs int64) (bool, error) {
	n, err := rowsAffected(
		r.ex.ExecContext(ctx, `UPDATE cost_ledger SET state = 'settled', actual_micros = ?, settled_ms = ?
 WHERE job_id = ? AND state = 'reserved'`, actualMicros, atMs, jobID),
	)

	return n > 0, err
}

func (r approvalRepo) Release(ctx context.Context, jobID string, atMs int64) (bool, error) {
	n, err := rowsAffected(r.ex.ExecContext(ctx, `UPDATE cost_ledger SET state = 'released', settled_ms = ?
 WHERE job_id = ? AND state = 'reserved'`, atMs, jobID))

	return n > 0, err
}

func (r approvalRepo) Spent(ctx context.Context, q approval.SpentQuery) (int64, error) {
	query := `SELECT COALESCE(SUM(CASE WHEN state = 'settled' THEN COALESCE(actual_micros, 0) ELSE estimate_micros END), 0)
 FROM cost_ledger WHERE day = ? AND currency = ? AND state IN ('reserved','settled')`
	args := []any{q.Day, q.Currency}

	var querySb311 strings.Builder
	for _, f := range []struct{ col, v string }{{"project_id", q.Project}, {"kind", q.Kind}, {"connection_id", q.Connection}} {
		if f.v != "" {
			querySb311.WriteString(" AND " + f.col + " = ?")

			args = append(args, f.v)
		}
	}
	query += querySb311.String()

	var n int64

	err := r.ex.QueryRowContext(ctx, query, args...).Scan(&n)

	return n, err
}

func (r approvalRepo) Totals(ctx context.Context, fromDay string) ([]approval.Total, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT day, connection_id, project_id, currency,
 SUM(CASE WHEN state = 'settled' THEN COALESCE(actual_micros, 0) ELSE estimate_micros END)
 FROM cost_ledger WHERE day >= ? AND state IN ('reserved','settled')
 GROUP BY day, connection_id, project_id, currency ORDER BY day DESC, connection_id, project_id, currency`, fromDay)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []approval.Total

	for rows.Next() {
		var t approval.Total
		if err := rows.Scan(&t.Day, &t.ConnectionID, &t.ProjectID, &t.Currency, &t.Micros); err != nil {
			return nil, err
		}

		out = append(out, t)
	}

	return out, rows.Err()
}

var _ approval.Store = approvalRepo{}
