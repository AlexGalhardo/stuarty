package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	core "github.com/AlexGalhardo/stuarty/core/webhook"
	"github.com/AlexGalhardo/stuarty/service/webhooks"

	"github.com/AlexGalhardo/vukem/database"
)

// webhookRepo is webhooks.Store over the tables webhooks, webhook_deliveries and webhook_attempts (SDD-005 section
// 5.16).
type webhookRepo struct{ ex database.Executor }

const (
	webhookColumns  = `id, name, url, events, enabled, created_by, created_ms, updated_ms`
	deliveryColumns = `id, webhook_id, event, event_id, kind, state, attempts, max_attempts, last_status, last_error,
 next_ms, created_ms, finished_ms`
)

func scanWebhook(sc interface{ Scan(...any) error }) (webhooks.Row, error) {
	var (
		r       webhooks.Row
		events  string
		enabled int
	)

	if err := sc.Scan(&r.ID, &r.Name, &r.URL, &events, &enabled, &r.CreatedBy, &r.CreatedMs, &r.UpdatedMs); err != nil {
		return r, err
	}

	r.Enabled = enabled != 0

	return r, json.Unmarshal([]byte(events), &r.Events)
}

func (r webhookRepo) Count(ctx context.Context) (int, error) {
	var n int

	err := r.ex.QueryRowContext(ctx, `SELECT count(*) FROM webhooks`).Scan(&n)

	return n, err
}

func (r webhookRepo) List(ctx context.Context) ([]webhooks.Row, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT `+webhookColumns+` FROM webhooks ORDER BY created_ms, id`)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []webhooks.Row

	for rows.Next() {
		row, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, row)
	}

	return out, rows.Err()
}

func (r webhookRepo) Get(ctx context.Context, id string) (webhooks.Row, error) {
	row, err := scanWebhook(r.ex.QueryRowContext(ctx, `SELECT `+webhookColumns+` FROM webhooks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return row, webhooks.ErrNotFound
	}

	return row, err
}

func (r webhookRepo) Insert(ctx context.Context, w webhooks.Row) error {
	events, err := json.Marshal(w.Events)
	if err != nil {
		return err
	}

	_, err = r.ex.ExecContext(ctx,
		`INSERT INTO webhooks (`+webhookColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.URL, string(events), boolInt(w.Enabled), w.CreatedBy, w.CreatedMs, w.UpdatedMs)

	return err
}

func (r webhookRepo) Update(ctx context.Context, w webhooks.Row) error {
	events, err := json.Marshal(w.Events)
	if err != nil {
		return err
	}

	res, err := r.ex.ExecContext(ctx,
		`UPDATE webhooks SET name = ?, url = ?, events = ?, enabled = ?, updated_ms = ? WHERE id = ?`,
		w.Name, w.URL, string(events), boolInt(w.Enabled), w.UpdatedMs, w.ID)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return webhooks.ErrNotFound
	}

	return nil
}

func (r webhookRepo) Delete(ctx context.Context, id string) error {
	res, err := r.ex.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return webhooks.ErrNotFound
	}

	return nil
}

func (r webhookRepo) Enqueue(ctx context.Context, d webhooks.Delivery) (bool, error) {
	res, err := r.ex.ExecContext(ctx,
		`INSERT INTO webhook_deliveries (id, webhook_id, event, event_id, kind, body, state, max_attempts, attempts,
 next_ms, created_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?) ON CONFLICT DO NOTHING`,
		d.ID, d.WebhookID, d.Event, d.EventID, d.Kind, string(d.Body), core.StatePending, d.MaxAttempts, d.NextMs,
		d.CreatedMs)
	if err != nil {
		return false, err
	}

	n, _ := res.RowsAffected()

	return n > 0, nil
}

func scanDelivery(sc interface{ Scan(...any) error }) (webhooks.Delivery, error) {
	var d webhooks.Delivery

	err := sc.Scan(&d.ID, &d.WebhookID, &d.Event, &d.EventID, &d.Kind, &d.State, &d.Attempts, &d.MaxAttempts,
		&d.LastStatus, &d.LastError, &d.NextMs, &d.CreatedMs, &d.FinishedMs)

	return d, err
}

func (r webhookRepo) Due(ctx context.Context, nowMs int64, limit int) ([]webhooks.Delivery, error) {
	rows, err := r.ex.QueryContext(ctx,
		`SELECT `+deliveryColumns+`, body FROM webhook_deliveries d
 WHERE state = 'pending' AND next_ms <= ? AND EXISTS (SELECT 1 FROM webhooks w WHERE w.id = d.webhook_id AND w.enabled = 1)
 ORDER BY next_ms, created_ms LIMIT ?`, nowMs, limit)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []webhooks.Delivery

	for rows.Next() {
		var (
			d    webhooks.Delivery
			body string
		)

		if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &d.EventID, &d.Kind, &d.State, &d.Attempts, &d.MaxAttempts,
			&d.LastStatus, &d.LastError, &d.NextMs, &d.CreatedMs, &d.FinishedMs, &body); err != nil {
			return nil, err
		}

		d.Body = []byte(body)
		out = append(out, d)
	}

	return out, rows.Err()
}

func (r webhookRepo) NextDue(ctx context.Context) (int64, error) {
	var next sql.NullInt64

	err := r.ex.QueryRowContext(ctx,
		`SELECT min(next_ms) FROM webhook_deliveries d WHERE state = 'pending'
 AND EXISTS (SELECT 1 FROM webhooks w WHERE w.id = d.webhook_id AND w.enabled = 1)`).Scan(&next)

	return next.Int64, err
}

func (r webhookRepo) GetDelivery(ctx context.Context, webhookID, id string) (webhooks.Delivery, error) {
	var body string

	d, err := func() (webhooks.Delivery, error) {
		var d webhooks.Delivery

		err := r.ex.QueryRowContext(ctx,
			`SELECT `+deliveryColumns+`, body FROM webhook_deliveries WHERE webhook_id = ? AND id = ?`, webhookID, id).
			Scan(&d.ID, &d.WebhookID, &d.Event, &d.EventID, &d.Kind, &d.State, &d.Attempts, &d.MaxAttempts,
				&d.LastStatus, &d.LastError, &d.NextMs, &d.CreatedMs, &d.FinishedMs, &body)

		return d, err
	}()
	if errors.Is(err, sql.ErrNoRows) {
		return d, webhooks.ErrNotFound
	}

	d.Body = []byte(body)

	return d, err
}

func (r webhookRepo) Deliveries(ctx context.Context, webhookID string, limit, offset int) ([]webhooks.Delivery, error) {
	rows, err := r.ex.QueryContext(ctx,
		`SELECT `+deliveryColumns+` FROM webhook_deliveries WHERE webhook_id = ? ORDER BY created_ms DESC, id DESC
 LIMIT ? OFFSET ?`, webhookID, limit, offset)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []webhooks.Delivery

	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, d)
	}

	return out, rows.Err()
}

func (r webhookRepo) Latest(ctx context.Context, webhookID string) (webhooks.Delivery, error) {
	d, err := scanDelivery(r.ex.QueryRowContext(
		ctx,
		`SELECT `+deliveryColumns+` FROM webhook_deliveries WHERE webhook_id = ? ORDER BY created_ms DESC, id DESC LIMIT 1`,
		webhookID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return d, webhooks.ErrNotFound
	}

	return d, err
}

func (r webhookRepo) Attempts(ctx context.Context, deliveryID string) ([]webhooks.Attempt, error) {
	rows, err := r.ex.QueryContext(
		ctx,
		`SELECT n, at_ms, status, duration_ms, error, response_bytes FROM webhook_attempts WHERE delivery_id = ? ORDER BY n`,
		deliveryID,
	)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []webhooks.Attempt

	for rows.Next() {
		var a webhooks.Attempt
		if err := rows.Scan(&a.N, &a.AtMs, &a.Status, &a.DurationMs, &a.Error, &a.ResponseBytes); err != nil {
			return nil, err
		}

		out = append(out, a)
	}

	return out, rows.Err()
}

func (r webhookRepo) Record(ctx context.Context, d webhooks.Delivery, a webhooks.Attempt, oldestMs int64) error {
	if _, err := r.ex.ExecContext(ctx,
		`INSERT INTO webhook_attempts (delivery_id, n, at_ms, status, duration_ms, error, response_bytes)
 VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		d.ID, a.N, a.AtMs, a.Status, a.DurationMs, a.Error, a.ResponseBytes); err != nil {
		return err
	}

	if _, err := r.ex.ExecContext(ctx,
		`UPDATE webhook_deliveries SET state = ?, attempts = ?, next_ms = ?, last_status = ?, last_error = ?,
 finished_ms = ? WHERE id = ?`,
		d.State, d.Attempts, d.NextMs, d.LastStatus, d.LastError, d.FinishedMs, d.ID); err != nil {
		return err
	}

	if _, err := r.ex.ExecContext(ctx,
		`DELETE FROM webhook_attempts WHERE rowid IN (
 SELECT a.rowid FROM webhook_attempts a JOIN webhook_deliveries x ON x.id = a.delivery_id WHERE x.webhook_id = ?
 ORDER BY a.at_ms DESC, a.rowid DESC LIMIT -1 OFFSET ?)`, d.WebhookID, core.KeepAttempts); err != nil {
		return err
	}

	_, err := r.ex.ExecContext(ctx,
		`DELETE FROM webhook_deliveries WHERE state != 'pending' AND finished_ms > 0 AND finished_ms < ?`, oldestMs)

	return err
}

func (r webhookRepo) Reschedule(ctx context.Context, id string, nextMs int64, reason string) error {
	_, err := r.ex.ExecContext(
		ctx,
		`UPDATE webhook_deliveries SET next_ms = ?, last_error = ? WHERE id = ? AND state = 'pending'`,
		nextMs,
		reason,
		id,
	)

	return err
}
