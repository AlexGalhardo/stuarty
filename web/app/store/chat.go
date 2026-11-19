package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/service/agenthost"

	"github.com/AlexGalhardo/vukem/database"
)

// chatRepo is agenthost.Store over the tables agent_profiles, chat_sessions and chat_events
// (SDD-017 sections 3 and 9).
type chatRepo struct{ ex database.Executor }

// chatBody is the part of an event that is stored as JSON; the number, the kind and the time are
// columns.
type chatBody struct {
	Text       string           `json:"text,omitempty"`
	Tool       *chat.Tool       `json:"tool,omitempty"`
	Permission *chat.Permission `json:"permission,omitempty"`
	State      *chat.StateInfo  `json:"state,omitempty"`
}

func (r chatRepo) ProfileList(ctx context.Context) ([]agenthost.Profile, error) {
	rows, err := r.ex.QueryContext(ctx,
		`SELECT name, command, args_json, created_by, created_ms, updated_ms FROM agent_profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []agenthost.Profile

	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, p)
	}

	return out, rows.Err()
}

func scanProfile(sc interface{ Scan(...any) error }) (agenthost.Profile, error) {
	var (
		p       agenthost.Profile
		args    string
		created int64
		updated int64
	)

	if err := sc.Scan(&p.Name, &p.Command, &args, &p.CreatedBy, &created, &updated); err != nil {
		return p, err
	}

	if err := json.Unmarshal([]byte(args), &p.Args); err != nil {
		return p, err
	}

	if p.Args == nil {
		p.Args = []string{}
	}

	p.CreatedAt, p.UpdatedAt = time.UnixMilli(created).UTC(), time.UnixMilli(updated).UTC()

	return p, nil
}

func (r chatRepo) ProfileGet(ctx context.Context, name string) (*agenthost.Profile, error) {
	p, err := scanProfile(r.ex.QueryRowContext(ctx,
		`SELECT name, command, args_json, created_by, created_ms, updated_ms FROM agent_profiles WHERE name = ?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, agenthost.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return &p, nil
}

func (r chatRepo) ProfileSet(ctx context.Context, p agenthost.Profile) error {
	args, err := json.Marshal(append([]string{}, p.Args...))
	if err != nil {
		return err
	}

	_, err = r.ex.ExecContext(ctx,
		`INSERT INTO agent_profiles (name, command, args_json, created_by, created_ms, updated_ms)
 VALUES (?, ?, ?, ?, ?, ?)
 ON CONFLICT (name) DO UPDATE SET command = excluded.command, args_json = excluded.args_json,
   updated_ms = excluded.updated_ms`,
		p.Name, p.Command, string(args), p.CreatedBy, p.CreatedAt.UnixMilli(), p.UpdatedAt.UnixMilli())

	return err
}

func (r chatRepo) ProfileRemove(ctx context.Context, name string) error {
	res, err := r.ex.ExecContext(ctx, `DELETE FROM agent_profiles WHERE name = ?`, name)

	return affected(res, err)
}

// affected maps a statement that touched no row to agenthost.ErrNotFound.
func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return agenthost.ErrNotFound
	}

	return nil
}

const chatSessionColumns = `id, project_id, profile, owner_user_id, started_by, state, end_reason, title,
 created_ms, updated_ms, ended_ms, event_count`

func scanChatSession(sc interface{ Scan(...any) error }) (agenthost.Session, error) {
	var (
		s       agenthost.Session
		owner   sql.NullString
		state   string
		reason  string
		created int64
		updated int64
		ended   sql.NullInt64
	)

	if err := sc.Scan(&s.ID, &s.ProjectID, &s.Profile, &owner, &s.StartedBy, &state, &reason, &s.Title,
		&created, &updated, &ended, &s.EventCount); err != nil {
		return s, err
	}

	s.OwnerUserID = owner.String
	s.State, s.EndReason = chat.State(state), chat.EndReason(reason)
	s.CreatedAt, s.UpdatedAt = time.UnixMilli(created).UTC(), time.UnixMilli(updated).UTC()

	if ended.Valid {
		t := time.UnixMilli(ended.Int64).UTC()
		s.EndedAt = &t
	}

	return s, nil
}

func (r chatRepo) SessionCreate(ctx context.Context, s agenthost.Session) error {
	_, err := r.ex.ExecContext(ctx,
		`INSERT INTO chat_sessions (`+chatSessionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.ProjectID, s.Profile, nullString(s.OwnerUserID), s.StartedBy, string(s.State), string(s.EndReason),
		s.Title, s.CreatedAt.UnixMilli(), s.UpdatedAt.UnixMilli(), endedMs(s.EndedAt), s.EventCount)

	return err
}

func endedMs(t *time.Time) any {
	if t == nil {
		return nil
	}

	return t.UnixMilli()
}

func (r chatRepo) SessionUpdate(ctx context.Context, s agenthost.Session) error {
	res, err := r.ex.ExecContext(ctx,
		`UPDATE chat_sessions SET state = ?, end_reason = ?, title = ?, updated_ms = ?, ended_ms = ?, event_count = ?
 WHERE id = ?`,
		string(s.State), string(s.EndReason), s.Title, s.UpdatedAt.UnixMilli(), endedMs(s.EndedAt), s.EventCount, s.ID)

	return affected(res, err)
}

func (r chatRepo) SessionGet(ctx context.Context, id string) (*agenthost.Session, error) {
	s, err := scanChatSession(r.ex.QueryRowContext(ctx,
		`SELECT `+chatSessionColumns+` FROM chat_sessions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, agenthost.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return &s, nil
}

func (r chatRepo) SessionList(ctx context.Context, projectID string) ([]agenthost.Session, error) {
	rows, err := r.ex.QueryContext(ctx,
		`SELECT `+chatSessionColumns+` FROM chat_sessions WHERE project_id = ? ORDER BY created_ms DESC, id DESC`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []agenthost.Session

	for rows.Next() {
		s, err := scanChatSession(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, s)
	}

	return out, rows.Err()
}

func (r chatRepo) SessionDelete(ctx context.Context, id string) error {
	if _, err := r.ex.ExecContext(ctx, `DELETE FROM chat_events WHERE session_id = ?`, id); err != nil {
		return err
	}

	res, err := r.ex.ExecContext(ctx, `DELETE FROM chat_sessions WHERE id = ?`, id)

	return affected(res, err)
}

func (r chatRepo) SessionsEndLive(ctx context.Context, at time.Time) (int, error) {
	res, err := r.ex.ExecContext(ctx,
		`UPDATE chat_sessions SET state = 'ended', end_reason = ?, ended_ms = ?, updated_ms = ? WHERE state <> 'ended'`,
		string(chat.EndShutdown), at.UnixMilli(), at.UnixMilli())
	if err != nil {
		return 0, err
	}

	n, err := res.RowsAffected()

	return int(n), err
}

// endedBeyond lists the ended sessions of a project past the newest keep.
func (r chatRepo) endedBeyond(ctx context.Context, projectID string, keep int) ([]string, error) {
	rows, err := r.ex.QueryContext(ctx,
		`SELECT id FROM chat_sessions WHERE project_id = ? AND state = 'ended'
 ORDER BY created_ms DESC, id DESC LIMIT -1 OFFSET ?`, projectID, keep)
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

func (r chatRepo) SessionsPrune(ctx context.Context, projectID string, keep int) error {
	// The sessions past the newest keep, of which only the ended ones are deleted: a live one is
	// never taken from under its process.
	ids, err := r.endedBeyond(ctx, projectID, keep)
	if err != nil {
		return err
	}

	for _, id := range ids {
		if err := r.SessionDelete(ctx, id); err != nil && !errors.Is(err, agenthost.ErrNotFound) {
			return err
		}
	}

	return nil
}

func (r chatRepo) EventUpsert(ctx context.Context, sessionID string, e chat.Event) error {
	body, err := json.Marshal(chatBody{Text: e.Text, Tool: e.Tool, Permission: e.Permission, State: e.State})
	if err != nil {
		return err
	}

	_, err = r.ex.ExecContext(ctx,
		`INSERT INTO chat_events (session_id, seq, kind, at_ms, body_json) VALUES (?, ?, ?, ?, ?)
 ON CONFLICT (session_id, seq) DO UPDATE SET kind = excluded.kind, at_ms = excluded.at_ms,
   body_json = excluded.body_json`,
		sessionID, e.Seq, string(e.Kind), e.At.UnixMilli(), string(body))

	return err
}

func (r chatRepo) EventList(ctx context.Context, sessionID string, after int64, limit int) ([]chat.Event, error) {
	rows, err := r.ex.QueryContext(ctx,
		`SELECT seq, kind, at_ms, body_json FROM chat_events WHERE session_id = ? AND seq > ? ORDER BY seq LIMIT ?`,
		sessionID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []chat.Event

	for rows.Next() {
		var (
			e    chat.Event
			kind string
			at   int64
			body string
			b    chatBody
		)

		if err := rows.Scan(&e.Seq, &kind, &at, &body); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(body), &b); err != nil {
			return nil, err
		}

		e.Kind, e.At = chat.Kind(kind), time.UnixMilli(at).UTC()
		e.Text, e.Tool, e.Permission, e.State = b.Text, b.Tool, b.Permission, b.State
		out = append(out, e)
	}

	return out, rows.Err()
}
