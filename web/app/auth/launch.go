package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"slices"
	"sync"
	"time"
)

// Launch code rules (SDD-007 section 3.4 items 1 and 4).
const (
	launchTTL      = 120 * time.Second
	launchMax      = 8
	failuresToLock = 5
	failureWindow  = 60 * time.Second
	lockFor        = 60 * time.Second
)

type launchCode struct {
	digest  [32]byte
	created time.Time
}

// Launch issues and redeems the one-time codes that turn into a browser
// session. Only the SHA-256 of a code is kept.
type Launch struct {
	mu       sync.Mutex
	now      func() time.Time
	codes    []launchCode
	failures []time.Time
	lockedTo time.Time
}

// NewLaunch makes an empty issuer. now may be nil.
func NewLaunch(now func() time.Time) *Launch {
	if now == nil {
		now = time.Now
	}

	return &Launch{now: now}
}

// Issue returns a fresh code: 32 random bytes as 64 hex characters. At most
// eight stay outstanding; the oldest is dropped for a ninth.
func (l *Launch) Issue() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}

	code := hex.EncodeToString(raw[:])

	l.mu.Lock()
	defer l.mu.Unlock()

	l.expire()

	if len(l.codes) >= launchMax {
		l.codes = l.codes[1:]
	}

	l.codes = append(l.codes, launchCode{digest: sha256.Sum256([]byte(code)), created: l.now()})

	return code, nil
}

// Redeem is true once for a code that is within its 120 seconds. The entry is
// deleted whatever the outcome, and five failures within 60 seconds lock the
// route (Locked) for the next 60 seconds.
func (l *Launch) Redeem(code string) bool {
	digest := sha256.Sum256([]byte(code))

	l.mu.Lock()
	defer l.mu.Unlock()

	l.expire()

	found := -1

	for i, c := range l.codes {
		if subtle.ConstantTimeCompare(c.digest[:], digest[:]) == 1 {
			found = i
		}
	}

	if found < 0 {
		l.fail()
		return false
	}

	l.codes = slices.Delete(l.codes, found, found+1)

	return true
}

// Locked reports whether the route answers 429 now.
func (l *Launch) Locked() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.now().Before(l.lockedTo)
}

func (l *Launch) fail() {
	t := l.now()
	kept := l.failures[:0]

	for _, f := range l.failures {
		if t.Sub(f) < failureWindow {
			kept = append(kept, f)
		}
	}

	kept = append(kept, t)
	l.failures = kept

	if len(l.failures) >= failuresToLock {
		l.lockedTo, l.failures = t.Add(lockFor), nil
	}
}

func (l *Launch) expire() {
	t, kept := l.now(), l.codes[:0]

	for _, c := range l.codes {
		if t.Sub(c.created) <= launchTTL {
			kept = append(kept, c)
		}
	}

	l.codes = kept
}
