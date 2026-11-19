package agenthost

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
)

// TokenPrefix starts the secret of every session token, so a secret scanner and the
// authentication can tell one from the local token and from an API key (SDD-017 section 6.1).
const TokenPrefix = "stc_"

// tokenBytes is the entropy of a session token: 256 bits.
const tokenBytes = 32

type grantEntry struct {
	grant   Grant
	expires time.Time
}

// Tokens is the registry of session tokens: in memory only, so a token never outlives the
// process, never reaches a disk, a log or a result. It stores the SHA-256 of a secret, not the
// secret.
type Tokens struct {
	now func() time.Time

	mu      sync.Mutex
	entries map[[sha256.Size]byte]grantEntry
}

// NewTokens returns an empty registry; now is the clock, default time.Now.
func NewTokens(now func() time.Time) *Tokens {
	if now == nil {
		now = time.Now
	}

	return &Tokens{now: now, entries: map[[sha256.Size]byte]grantEntry{}}
}

var _ TokenIssuer = (*Tokens)(nil)

// Issue makes a secret bound to the grant. It refuses a grant for no project, or with the admin
// scope: a session token is for one project and never administers.
func (t *Tokens) Issue(g Grant, expires time.Time) (string, func(), error) {
	if g.SessionID == "" || g.ProjectID == "" {
		return "", nil, errors.New("agenthost: a session token is bound to a session and a project")
	}

	if slices.Contains(g.Scopes, "admin") {
		return "", nil, errors.New("agenthost: a session token never holds the admin scope")
	}

	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}

	secret := TokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	key := sha256.Sum256([]byte(secret))

	t.mu.Lock()
	t.entries[key] = grantEntry{grant: g, expires: expires}
	t.mu.Unlock()

	return secret, func() { t.revoke(key) }, nil
}

func (t *Tokens) revoke(key [sha256.Size]byte) {
	t.mu.Lock()
	delete(t.entries, key)
	t.mu.Unlock()
}

// Lookup returns the grant of a live secret. It answers false for anything else, an expired
// token included, and for a string that is not shaped like a session token without looking at
// the registry.
func (t *Tokens) Lookup(secret string) (Grant, bool) {
	if !strings.HasPrefix(secret, TokenPrefix) ||
		len(secret) != len(TokenPrefix)+base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return Grant{}, false
	}

	key := sha256.Sum256([]byte(secret))

	t.mu.Lock()
	e, ok := t.entries[key]
	t.mu.Unlock()

	// The lookup is by the hash of a 256-bit secret, so how long it takes says nothing about the
	// secret.
	if !ok {
		return Grant{}, false
	}

	if !e.expires.IsZero() && !t.now().Before(e.expires) {
		t.revoke(key)

		return Grant{}, false
	}

	g := e.grant
	g.Scopes = append([]string(nil), g.Scopes...)

	return g, true
}

// Count is how many tokens are live, for the tests.
func (t *Tokens) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return len(t.entries)
}
