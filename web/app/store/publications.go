package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	pub "github.com/AlexGalhardo/stuarty/service/publish"

	"github.com/AlexGalhardo/vukem/database"
)

// publicationRepo is publish.Store over the tables of SDD-005 section 5.15.
type publicationRepo struct{ ex database.Executor }

const publicationColumns = `id, project_id, title, text, export_job, thumbnail_job, created_ms, updated_ms, created_by, owner_user_id`

const targetColumns = `id, publication_id, position, connection_id, network, fields, state, scheduled_ms, zone, late_minutes,
 send_now, approval_id, snapshot, payload_sha256, attempts, next_attempt_ms, lease_owner, lease_until_ms, remote_id, url,
 remote_state, error, published_ms, created_ms, updated_ms`

func msTime(n sql.NullInt64) *time.Time {
	if !n.Valid {
		return nil
	}

	t := time.UnixMilli(n.Int64).UTC()

	return &t
}

func timeMs(t *time.Time) any {
	if t == nil {
		return nil
	}

	return t.UnixMilli()
}

func scanPublication(sc scanner) (pub.Publication, error) {
	var (
		p              pub.Publication
		owner          sql.NullString
		created, upd   int64
		thumb, project string
	)

	if err := sc.Scan(
		&p.ID,
		&project,
		&p.Title,
		&p.Text,
		&p.Export,
		&thumb,
		&created,
		&upd,
		&p.CreatedBy,
		&owner,
	); err != nil {
		return p, err
	}

	p.ProjectID, p.Thumbnail, p.OwnerUserID = project, thumb, owner.String
	p.CreatedAt, p.UpdatedAt = time.UnixMilli(created).UTC(), time.UnixMilli(upd).UTC()
	p.Targets = []pub.Target{}

	return p, nil
}

func scanTarget(sc scanner) (pub.Target, error) {
	var (
		t                                                                      pub.Target
		fields, state                                                          string
		sched, next, leaseUntil, published                                     sql.NullInt64
		approval, snapshot, sha, leaseOwner, remoteID, url, remoteState, errJS sql.NullString
		created, updated                                                       int64
		sendNow                                                                int
	)

	if err := sc.Scan(
		&t.ID, &t.PublicationID, &t.Position, &t.ConnectionID, &t.Network, &fields, &state, &sched, &t.Zone,
		&t.LateWindowMinutes, &sendNow, &approval, &snapshot, &sha, &t.Attempts, &next, &leaseOwner, &leaseUntil,
		&remoteID, &url, &remoteState, &errJS, &published, &created, &updated,
	); err != nil {
		return t, err
	}

	if err := json.Unmarshal([]byte(fields), &t.Fields); err != nil {
		return t, err
	}

	t.State = pub.TargetState(state)
	t.ScheduledAt, t.NextAttemptAt, t.LeaseUntil, t.PublishedAt = msTime(
		sched,
	), msTime(
		next,
	), msTime(
		leaseUntil,
	), msTime(
		published,
	)
	t.SendNow, t.ApprovalID, t.PayloadSHA256, t.LeaseOwner = sendNow != 0, approval.String, sha.String, leaseOwner.String
	t.RemoteID, t.URL, t.RemoteState = remoteID.String, url.String, remoteState.String
	t.CreatedAt, t.UpdatedAt = time.UnixMilli(created).UTC(), time.UnixMilli(updated).UTC()

	if snapshot.Valid {
		t.Snapshot = json.RawMessage(snapshot.String)
	}

	if errJS.Valid {
		t.Error = new(pub.TargetError)
		if err := json.Unmarshal([]byte(errJS.String), t.Error); err != nil {
			return t, err
		}
	}

	return t, nil
}

func (r publicationRepo) insertTarget(ctx context.Context, t *pub.Target) error {
	fields, err := json.Marshal(t.Fields)
	if err != nil {
		return err
	}

	var errJS any

	if t.Error != nil {
		raw, merr := json.Marshal(t.Error)
		if merr != nil {
			return merr
		}

		errJS = string(raw)
	}

	_, err = r.ex.ExecContext(
		ctx,
		`INSERT INTO publication_targets (`+targetColumns+`)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT (id) DO UPDATE SET position = excluded.position, connection_id = excluded.connection_id,
  network = excluded.network, fields = excluded.fields, state = excluded.state, scheduled_ms = excluded.scheduled_ms,
  zone = excluded.zone, late_minutes = excluded.late_minutes, send_now = excluded.send_now,
  approval_id = excluded.approval_id, snapshot = excluded.snapshot, payload_sha256 = excluded.payload_sha256,
  attempts = excluded.attempts, next_attempt_ms = excluded.next_attempt_ms, lease_owner = excluded.lease_owner,
  lease_until_ms = excluded.lease_until_ms, remote_id = excluded.remote_id, url = excluded.url,
  remote_state = excluded.remote_state, error = excluded.error, published_ms = excluded.published_ms,
  updated_ms = excluded.updated_ms`,
		t.ID,
		t.PublicationID,
		t.Position,
		t.ConnectionID,
		t.Network,
		string(fields),
		string(t.State),
		timeMs(t.ScheduledAt),
		t.Zone,
		t.LateWindowMinutes,
		boolInt(t.SendNow),
		nullString(t.ApprovalID),
		nullJSON(
			t.Snapshot,
		),
		nullString(t.PayloadSHA256),
		t.Attempts,
		timeMs(t.NextAttemptAt),
		nullString(t.LeaseOwner),
		timeMs(t.LeaseUntil),
		nullString(t.RemoteID),
		nullString(t.URL),
		nullString(t.RemoteState),
		errJS,
		timeMs(t.PublishedAt),
		t.CreatedAt.UnixMilli(),
		t.UpdatedAt.UnixMilli(),
	)

	return err
}

func (r publicationRepo) Insert(ctx context.Context, p *pub.Publication) error {
	if _, err := r.ex.ExecContext(ctx, `INSERT INTO publications (`+publicationColumns+`)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ProjectID, p.Title, p.Text, p.Export, p.Thumbnail, p.CreatedAt.UnixMilli(), p.UpdatedAt.UnixMilli(),
		p.CreatedBy, nullString(p.OwnerUserID)); err != nil {
		return err
	}

	for i := range p.Targets {
		if err := r.insertTarget(ctx, &p.Targets[i]); err != nil {
			return err
		}
	}

	return nil
}

func (r publicationRepo) Get(ctx context.Context, id string) (pub.Publication, error) {
	p, err := scanPublication(
		r.ex.QueryRowContext(ctx, `SELECT `+publicationColumns+` FROM publications WHERE id = ?`, id),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return p, pub.ErrNotFound
	}

	if err != nil {
		return p, err
	}

	targets, err := r.targetsOf(ctx, []string{id})
	if err != nil {
		return p, err
	}

	p.Targets = append(p.Targets, targets[id]...)

	return p, nil
}

func (r publicationRepo) OfTarget(ctx context.Context, targetID string) (pub.Publication, error) {
	var id string

	err := r.ex.QueryRowContext(ctx, `SELECT publication_id FROM publication_targets WHERE id = ?`, targetID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return pub.Publication{}, pub.ErrNotFound
	}

	if err != nil {
		return pub.Publication{}, err
	}

	return r.Get(ctx, id)
}

// targetsOf reads the targets of publications, in their order, grouped by publication.
func (r publicationRepo) targetsOf(ctx context.Context, ids []string) (map[string][]pub.Target, error) {
	out := map[string][]pub.Target{}
	if len(ids) == 0 {
		return out, nil
	}

	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))

	for i, id := range ids {
		args[i] = id
	}

	rows, err := r.ex.QueryContext(
		ctx,
		`SELECT `+targetColumns+` FROM publication_targets WHERE publication_id IN (`+marks+
			`) ORDER BY publication_id, position, id`,
		args...)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}

		out[t.PublicationID] = append(out[t.PublicationID], t)
	}

	return out, rows.Err()
}

func (r publicationRepo) Save(ctx context.Context, p *pub.Publication) error {
	n, err := rowsAffected(r.ex.ExecContext(ctx, `UPDATE publications SET title = ?, text = ?, export_job = ?,
 thumbnail_job = ?, updated_ms = ? WHERE id = ?`, p.Title, p.Text, p.Export, p.Thumbnail, p.UpdatedAt.UnixMilli(), p.ID))
	if err != nil {
		return err
	}

	if n == 0 {
		return pub.ErrNotFound
	}

	keep := make([]any, 0, len(p.Targets)+1)
	keep = append(keep, p.ID)

	for i := range p.Targets {
		p.Targets[i].Position = i

		if err := r.insertTarget(ctx, &p.Targets[i]); err != nil {
			return err
		}

		keep = append(keep, p.Targets[i].ID)
	}

	marks := strings.TrimSuffix(strings.Repeat("?,", len(p.Targets)), ",")
	if marks == "" {
		_, err = r.ex.ExecContext(ctx, `DELETE FROM publication_targets WHERE publication_id = ?`, p.ID)

		return err
	}

	_, err = r.ex.ExecContext(
		ctx,
		`DELETE FROM publication_targets WHERE publication_id = ? AND id NOT IN (`+marks+`)`,
		keep...)

	return err
}

func (r publicationRepo) List(ctx context.Context, f pub.Filter) ([]pub.Publication, int, error) {
	var (
		where []string
		args  []any
	)

	if f.ProjectID != "" {
		where, args = append(where, "p.project_id = ?"), append(args, f.ProjectID)
	}

	if f.OwnerUserID != "" {
		where, args = append(where, "p.owner_user_id = ?"), append(args, f.OwnerUserID)
	}

	var on []string

	if f.State != "" {
		on, args = append(on, "t.state = ?"), append(args, string(f.State))
	}

	if f.Network != "" {
		on, args = append(on, "t.network = ?"), append(args, f.Network)
	}

	if f.From != nil {
		on, args = append(on, "t.scheduled_ms >= ?"), append(args, f.From.UnixMilli())
	}

	if f.To != nil {
		on, args = append(on, "t.scheduled_ms < ?"), append(args, f.To.UnixMilli())
	}

	if len(on) > 0 {
		where = append(where, "EXISTS (SELECT 1 FROM publication_targets t WHERE t.publication_id = p.id AND "+
			strings.Join(on, " AND ")+")")
	}

	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM publications p`+clause, args...).
		Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.ex.QueryContext(ctx, `SELECT `+prefixed("p.", publicationColumns)+` FROM publications p`+clause+
		` ORDER BY p.updated_ms DESC, p.id DESC LIMIT ? OFFSET ?`, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}

	defer func() { _ = rows.Close() }()

	var (
		out []pub.Publication
		ids []string
	)

	for rows.Next() {
		p, err := scanPublication(rows)
		if err != nil {
			return nil, 0, err
		}

		out, ids = append(out, p), append(ids, p.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	_ = rows.Close()

	targets, err := r.targetsOf(ctx, ids)
	if err != nil {
		return nil, 0, err
	}

	for i := range out {
		out[i].Targets = append(out[i].Targets, targets[out[i].ID]...)
	}

	return out, total, nil
}

func prefixed(prefix, columns string) string {
	parts := strings.Split(columns, ",")
	for i, c := range parts {
		parts[i] = prefix + strings.TrimSpace(c)
	}

	return strings.Join(parts, ", ")
}

func (r publicationRepo) OfProject(ctx context.Context, projectID string) ([]pub.Publication, error) {
	list, _, err := r.List(ctx, pub.Filter{ProjectID: projectID, Limit: 1000})

	return list, err
}

func (r publicationRepo) Open(ctx context.Context) ([]pub.Target, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT `+prefixed("t.", targetColumns)+` FROM publication_targets t
 JOIN publications p ON p.id = t.publication_id JOIN projects pr ON pr.id = p.project_id
 WHERE t.state IN ('approved', 'scheduled', 'publishing') AND pr.state = 'active'
 ORDER BY t.scheduled_ms, t.id`)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []pub.Target

	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, t)
	}

	return out, rows.Err()
}

func (r publicationRepo) Claim(ctx context.Context, id, owner string, now, until time.Time) (bool, error) {
	n, err := rowsAffected(r.ex.ExecContext(ctx, `UPDATE publication_targets
 SET state = 'publishing', lease_owner = ?, lease_until_ms = ?, attempts = attempts + 1, updated_ms = ?
 WHERE id = ? AND (
   (state IN ('approved', 'scheduled') AND COALESCE(next_attempt_ms, scheduled_ms) <= ?)
   OR (state = 'publishing' AND lease_until_ms < ?))
 AND publication_id IN (SELECT p.id FROM publications p JOIN projects pr ON pr.id = p.project_id WHERE pr.state = 'active')`,
		owner, until.UnixMilli(), now.UnixMilli(), id, now.UnixMilli(), now.UnixMilli()))

	return n == 1, err
}

func (r publicationRepo) Renew(ctx context.Context, id, owner string, until time.Time) (bool, error) {
	n, err := rowsAffected(r.ex.ExecContext(ctx, `UPDATE publication_targets SET lease_until_ms = ?
 WHERE id = ? AND state = 'publishing' AND lease_owner = ?`, until.UnixMilli(), id, owner))

	return n == 1, err
}
