package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// Session lifetimes (SDD-007 section 3.4 item 6).
const (
	sessionIdle = 24 * time.Hour
	sessionMax  = 7 * 24 * time.Hour
)

// CookieName is the cookie of local mode (SDD-007 section 3.5).
const CookieName = "vukem_video_session"

// SessionMaxAge is the Max-Age of the cookie in seconds.
const SessionMaxAge = int(sessionMax / time.Second)

type session struct{ created, seen time.Time }

// Sessions are the browser sessions of local mode: in memory, keyed by the
// SHA-256 of the identifier, so a memory dump alone does not hold a usable
// cookie value.
type Sessions struct {
	mu  sync.Mutex
	now func() time.Time
	m   map[[32]byte]*session
}

// NewSessions makes an empty store. now may be nil.
func NewSessions(now func() time.Time) *Sessions {
	if now == nil {
		now = time.Now
	}

	return &Sessions{now: now, m: map[[32]byte]*session{}}
}

// Create starts a session and returns the cookie value.
func (s *Sessions) Create() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}

	id := hex.EncodeToString(raw[:])
	t := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.m[sha256.Sum256([]byte(id))] = &session{created: t, seen: t}

	return id, nil
}

// Valid reports whether the cookie value is a live session, and renews its
// idle time.
func (s *Sessions) Valid(id string) bool {
	key := sha256.Sum256([]byte(id))
	t := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.m[key]
	if !ok {
		return false
	}

	if t.Sub(sess.seen) > sessionIdle || t.Sub(sess.created) > sessionMax {
		delete(s.m, key)
		return false
	}

	sess.seen = t

	return true
}

// Clear ends every session (token rotation).
func (s *Sessions) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.m)
}
