package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// Limits of API keys (SDD-007 section 4.2).
const (
	keyNameMax         = 60
	keyDefaultDays     = 90
	keyMaxDays         = 365
	keyAdminMaxDays    = 30
	maxActiveKeys      = 50
	maxKeyCreations    = 10
	keyCreationsWindow = time.Hour
)

// Key is an API key as a client sees it: never the secret and never its hash.
type Key struct {
	KeyID      string     `json:"key_id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	ProjectID  *string    `json:"project_id"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	UserID     string     `json:"-"`
}

// NewKey is what the key store records for a key that passed validation.
type NewKey struct {
	KeyID     string
	UserID    string
	Name      string
	Scopes    []string
	ProjectID string
	TTL       time.Duration
	// The limits, checked by the store in the same step as the insert so that
	// concurrent creations cannot all pass the check.
	Now       time.Time
	MaxActive int
	MaxRecent int
	Window    time.Duration
}

// KeyLimitError is what CreateKey returns when a limit stops the creation.
type KeyLimitError struct {
	Rate   bool // the creations of the last hour; otherwise the active keys
	Active int
	Recent int
	Oldest time.Time // the earliest creation of the window
}

func (e *KeyLimitError) Error() string { return "service: a key limit was reached" }

// KeyStore is where the keys of network mode live (SDD-007 section 4.2). The
// service validates and decides; the store holds the rows and issues the
// secret, which only the web layer can do because the generator is vukem's.
type KeyStore interface {
	// CreateKey checks the limits of k and, in the same step, issues the secret
	// and records the key; it returns *KeyLimitError when a limit is reached.
	CreateKey(ctx context.Context, k NewKey) (secret string, key Key, err error)
	// ListKeys returns the keys of a user, newest first.
	ListKeys(ctx context.Context, userID string) ([]Key, error)
	// RevokeKey revokes a key at once and keeps its row. userID "" matches a
	// key of any owner. A key that does not exist for that owner is ErrNotFound.
	RevokeKey(ctx context.Context, keyID, userID string) (Key, error)
}

// KeyCreateRequest is SDD-007 section 4.2 "Creation".
type KeyCreateRequest struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	ProjectID     string   `json:"project_id,omitempty"`
	ExpiresInDays int      `json:"expires_in_days,omitempty"`
}

// KeyCreateResult carries the secret, the one time it is shown.
type KeyCreateResult struct {
	Key    Key    `json:"key"`
	Secret string `json:"secret"`
}

// KeyListRequest has no member: a user lists their own keys.
type KeyListRequest struct{}

// KeyListResult is the keys of the caller.
type KeyListResult struct {
	Keys []Key `json:"keys"`
}

// KeyRevokeRequest names the key.
type KeyRevokeRequest struct {
	KeyID string `json:"key_id"`
}

// keyCaller is the check that the three key operations share: a person who
// logged in, in network mode. A key can never manage keys (T-33), and local
// mode has no users, so there is nothing to find there.
func (s *svc) keyCaller(ctx context.Context, method string) (Caller, error) {
	c, err := authorize(ctx, method, "")
	if err != nil {
		return Caller{}, err
	}

	if c.OwnerUserID == "" || s.keys == nil {
		return Caller{}, newError(command.CodeNotFound, "API keys exist in network mode only.")
	}

	if !c.Browser {
		return Caller{}, newError(CodeForbidden, "only a person who logged in can manage API keys.").
			withHint("log in in the browser; a key cannot create, list or revoke keys")
	}

	return c, nil
}

func (s *svc) KeyCreate(ctx context.Context, req KeyCreateRequest) (*KeyCreateResult, error) {
	c, err := s.keyCaller(ctx, "KeyCreate")
	if err != nil {
		return nil, err
	}

	days, verr := validateKey(c, &req)
	if verr != nil {
		return nil, verr
	}

	if req.ProjectID != "" {
		// A key is limited to a project, never to the id of one of its timelines: Productions().Get answers
		// for projects only, and a timeline id gets the answer of an id that names nothing.
		head, perr := s.store.Productions().Get(ctx, req.ProjectID)
		if perr != nil || !visible(c, head.OwnerUserID) {
			return nil, projectNotFound(req.ProjectID, perr)
		}
	}

	now := s.now()

	secret, key, kerr := s.keys.CreateKey(ctx, NewKey{
		KeyID: s.newIDs("key"), UserID: c.OwnerUserID, Name: req.Name, Scopes: req.Scopes,
		ProjectID: req.ProjectID, TTL: time.Duration(days) * 24 * time.Hour,
		Now: now, MaxActive: maxActiveKeys, MaxRecent: maxKeyCreations, Window: keyCreationsWindow,
	})
	if limit, ok := errors.AsType[*KeyLimitError](kerr); ok {
		return nil, keyLimitError(limit, now)
	}

	if kerr != nil {
		return nil, internalError()
	}

	return &KeyCreateResult{Key: key, Secret: secret}, nil
}

// validateKey checks the body of KeyCreate against the creator and returns the
// lifetime in days. The scopes are sorted and de-duplicated in place.
func validateKey(c Caller, req *KeyCreateRequest) (int, *Error) {
	if n := len([]rune(req.Name)); n < 1 || n > keyNameMax {
		return 0, newError(command.CodeValidation, "the name must have 1 to %d characters.", keyNameMax).
			withPath("name")
	}

	if len(req.Scopes) == 0 {
		return 0, newError(command.CodeValidation, "the key needs at least one scope.").withPath("scopes").
			withHint("choose from %v", c.Scopes)
	}

	for _, sc := range req.Scopes {
		if !slices.Contains(AllScopes, sc) {
			return 0, newError(command.CodeValidation, "the scope %q is not known.", sc).withPath("scopes")
		}

		if !slices.Contains(c.Scopes, sc) {
			return 0, newError(CodeForbidden, "the key cannot have the %q scope: you do not hold it.", sc).
				withPath("scopes").withHint("a key holds a subset of the scopes of its creator")
		}
	}

	slices.Sort(req.Scopes)
	req.Scopes = slices.Compact(req.Scopes)

	admin := slices.Contains(req.Scopes, ScopeAdmin)
	if admin && req.ProjectID != "" {
		return 0, newError(command.CodeValidation, "a key with the admin scope cannot be limited to a project.").
			withPath("project_id")
	}

	days := req.ExpiresInDays
	if days == 0 {
		days = min(keyDefaultDays, keyLifetimeCap(admin))
	}

	if days < 1 || days > keyLifetimeCap(admin) {
		return 0, newError(command.CodeValidation, "expires_in_days must be 1 to %d.", keyLifetimeCap(admin)).
			withPath("expires_in_days").withHint("a key with the admin scope lasts 30 days at most")
	}

	return days, nil
}

func keyLifetimeCap(admin bool) int {
	if admin {
		return keyAdminMaxDays
	}

	return keyMaxDays
}

func (s *svc) KeyList(ctx context.Context, _ KeyListRequest) (*KeyListResult, error) {
	c, err := s.keyCaller(ctx, "KeyList")
	if err != nil {
		return nil, err
	}

	keys, lerr := s.keys.ListKeys(ctx, c.OwnerUserID)
	if lerr != nil {
		return nil, internalError()
	}

	if keys == nil {
		keys = []Key{}
	}

	return &KeyListResult{Keys: keys}, nil
}

func (s *svc) KeyRevoke(ctx context.Context, req KeyRevokeRequest) (*Key, error) {
	c, err := s.keyCaller(ctx, "KeyRevoke")
	if err != nil {
		return nil, err
	}

	owner := c.OwnerUserID
	if slices.Contains(c.Scopes, ScopeAdmin) {
		owner = ""
	}

	key, rerr := s.keys.RevokeKey(ctx, req.KeyID, owner)
	if errors.Is(rerr, ErrNotFound) {
		return nil, newError(command.CodeNotFound, "there is no key %q.", req.KeyID).withPath("key_id")
	}

	if rerr != nil {
		return nil, internalError()
	}

	return &key, nil
}

func keyLimitError(l *KeyLimitError, now time.Time) *Error {
	if l.Rate {
		wait := max(l.Oldest.Add(keyCreationsWindow).Sub(now), time.Second)
		e := newError(CodeRateLimited, "%d keys were created in the last hour; the limit is %d.",
			l.Recent, maxKeyCreations).withHint("wait before creating another key")
		e.Details = map[string]any{"retry_after": int(wait.Seconds()) + 1}

		return e.withLimit("keys_created_per_hour", l.Recent, maxKeyCreations)
	}

	e := newError("E_LIMIT_EXCEEDED", "You have %d active keys; the limit is %d.", l.Active, maxActiveKeys).
		withHint("revoke a key you no longer use")
	e.Details = map[string]any{"limit": "keys", "value": l.Active, "ceiling": maxActiveKeys}

	return e
}
