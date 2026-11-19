package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/auth"
)

// KeyPrefix starts the secret of every API key, so secret scanners recognise a
// leak (SDD-007 section 4.2).
const KeyPrefix = "vvk_"

// ErrBadKey is a credential that is not a live key of a live user.
var ErrBadKey = errors.New("the key is not valid")

// Keys is the service.KeyStore of network mode: the secret and its SHA-256 are
// vukem's auth.Tokens, and api_keys adds the key id, the owner and the optional
// project limit (SDD-005 section 5.10).
func (n *Network) Keys() service.KeyStore { return keyStore{n} }

type keyStore struct{ n *Network }

// keyMu makes the count of the limits and the insert one step. One process owns
// the workspace (its lock), so a mutex serialises every creation.
var keyMu sync.Mutex

const keySelect = `SELECT k.id, k.name, k.user_id, k.project_id, k.created_ms, t.abilities,
  t.last_used_at, t.expires_at, t.revoked_at, k.token_id
FROM api_keys k JOIN access_tokens t ON t.id = k.token_id `

// CreateKey issues the secret and records the key; a failure to record revokes
// the token again, so no secret exists without its row.
func (s keyStore) CreateKey(ctx context.Context, k service.NewKey) (string, service.Key, error) {
	keyMu.Lock()
	defer keyMu.Unlock()

	counts, err := s.counts(ctx, k)
	if err != nil {
		return "", service.Key{}, err
	}

	if counts.Recent >= k.MaxRecent {
		counts.Rate = true
		return "", service.Key{}, counts
	}

	if counts.Active >= k.MaxActive {
		return "", service.Key{}, counts
	}

	secret, tok, err := s.n.tokens.Create(ctx, k.UserID, k.Name, k.Scopes, k.TTL)
	if err != nil {
		return "", service.Key{}, err
	}

	var project any
	if k.ProjectID != "" {
		project = k.ProjectID
	}

	_, err = s.n.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, token_id, user_id, project_id, name, created_ms) VALUES (?, ?, ?, ?, ?, ?)`,
		k.KeyID, tok.ID, k.UserID, project, k.Name, s.n.now().UnixMilli())
	if err != nil {
		_ = s.n.tokens.Revoke(ctx, tok.ID)

		return "", service.Key{}, err
	}

	key, _, err := s.one(ctx, `WHERE k.id = ?`, k.KeyID)

	return secret, key, err
}

func (s keyStore) ListKeys(ctx context.Context, userID string) ([]service.Key, error) {
	keys, _, err := s.many(ctx, `WHERE k.user_id = ? ORDER BY k.created_ms DESC, k.id`, userID)

	return keys, err
}

func (s keyStore) RevokeKey(ctx context.Context, keyID, userID string) (service.Key, error) {
	where, args := `WHERE k.id = ?`, []any{keyID}
	if userID != "" {
		where, args = where+` AND k.user_id = ?`, append(args, userID)
	}

	key, tokenID, err := s.one(ctx, where, args...)
	if err != nil {
		return service.Key{}, err
	}

	if key.RevokedAt == nil {
		if err := s.n.tokens.Revoke(ctx, tokenID); err != nil {
			return service.Key{}, err
		}

		key, _, err = s.one(ctx, where, args...)
	}

	return key, err
}

// counts are the numbers of the limits for the owner of k.
func (s keyStore) counts(ctx context.Context, k service.NewKey) (*service.KeyLimitError, error) {
	keys, _, err := s.many(ctx, `WHERE k.user_id = ?`, k.UserID)
	if err != nil {
		return nil, err
	}

	c := &service.KeyLimitError{}
	since := k.Now.Add(-k.Window)

	for i := range keys {
		key := &keys[i]
		if key.RevokedAt == nil && (key.ExpiresAt == nil || key.ExpiresAt.After(k.Now)) {
			c.Active++
		}

		if !key.CreatedAt.Before(since) {
			c.Recent++

			if c.Oldest.IsZero() || key.CreatedAt.Before(c.Oldest) {
				c.Oldest = key.CreatedAt
			}
		}
	}

	return c, nil
}

func (s keyStore) one(ctx context.Context, where string, args ...any) (service.Key, string, error) {
	keys, tokens, err := s.many(ctx, where, args...)
	if err != nil {
		return service.Key{}, "", err
	}

	if len(keys) == 0 {
		return service.Key{}, "", service.ErrNotFound
	}

	return keys[0], tokens[0], nil
}

// many returns the keys and, alongside, the id of the token of each.
func (s keyStore) many(ctx context.Context, where string, args ...any) ([]service.Key, []string, error) {
	rows, err := s.n.db.QueryContext(ctx, keySelect+where, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var (
		keys   []service.Key
		tokens []string
	)

	for rows.Next() {
		var (
			k         service.Key
			project   sql.NullString
			abilities string
			created   int64
			token     string
		)

		if err := rows.Scan(&k.KeyID, &k.Name, &k.UserID, &project, &created, &abilities,
			&k.LastUsedAt, &k.ExpiresAt, &k.RevokedAt, &token); err != nil {
			return nil, nil, err
		}

		if project.Valid {
			p := project.String
			k.ProjectID = &p
		}

		k.CreatedAt = time.UnixMilli(created).UTC()
		k.Scopes = parseAbilities(abilities)
		keys, tokens = append(keys, k), append(tokens, token)
	}

	return keys, tokens, rows.Err()
}

func parseAbilities(raw string) []string {
	var out []string
	if json.Unmarshal([]byte(raw), &out) != nil || out == nil {
		return []string{}
	}

	return out
}

// KeyCaller authenticates a bearer key: the secret is checked in constant time
// against its stored SHA-256, and the effective scopes are the key's scopes
// intersected with the present role of its owner, so demoting a user narrows
// the keys at once (SDD-007 section 4.2). ErrBadKey is every reason a key is
// refused: unknown, malformed, revoked, expired, or its owner is gone.
func (n *Network) KeyCaller(ctx context.Context, secret string) (service.Caller, error) {
	if !strings.HasPrefix(secret, KeyPrefix) {
		return service.Caller{}, ErrBadKey
	}

	tok, err := n.tokens.Find(ctx, secret)
	if errors.Is(err, auth.ErrInvalidToken) {
		return service.Caller{}, ErrBadKey
	}

	if err != nil {
		return service.Caller{}, err
	}

	var keyID string

	var project sql.NullString

	err = n.db.QueryRowContext(ctx, `SELECT id, project_id FROM api_keys WHERE token_id = ?`, tok.ID).
		Scan(&keyID, &project)
	if errors.Is(err, sql.ErrNoRows) {
		return service.Caller{}, ErrBadKey
	}

	if err != nil {
		return service.Caller{}, err
	}

	user, err := n.Users.ByID(ctx, tok.UserID)
	if errors.Is(err, auth.ErrUserNotFound) {
		return service.Caller{}, ErrBadKey
	}

	if err != nil {
		return service.Caller{}, err
	}

	var scopes []string

	for _, sc := range user.Scopes() {
		if tok.Can(sc) {
			scopes = append(scopes, sc)
		}
	}

	slices.Sort(scopes)

	return service.Caller{
		Actor: "key:" + keyID, Scopes: scopes, OwnerUserID: user.ID, ProjectID: project.String,
	}, nil
}

// RevokeKeysOf revokes every key of a user at once.
func (n *Network) RevokeKeysOf(ctx context.Context, userID string) error {
	return n.tokens.RevokeAll(ctx, userID)
}
