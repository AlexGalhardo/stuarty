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

// EnvLauncherSecret is the variable through which the desktop shell hands the
// server its launcher secret (SDD-007 section 5.3). The server reads it once at
// start and removes it from its environment, so no child process inherits it.
const EnvLauncherSecret = "VUKEM_VIDEO_LAUNCHER_SECRET" //nolint:gosec // the name of the variable, not a secret

// LauncherHeader carries the launcher secret on POST /api/v1/system/launch.
const LauncherHeader = "Vukem-Video-Launcher"

// IsLauncher is true when got is the launcher secret of this server process.
// An empty secret (no launcher started the server) matches nothing, an empty
// header included. Both sides are hashed so the comparison is constant time and
// the length of the secret does not leak.
func IsLauncher(secret, got string) bool {
	if secret == "" {
		return false
	}

	a, b := sha256.Sum256([]byte(secret)), sha256.Sum256([]byte(got))

	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

type launchCode struct {
	digest     [32]byte
	created    time.Time
	privileged bool
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
// eight stay outstanding; the oldest is dropped for a ninth. A privileged code
// turns into a session with the browser-only privileges (SDD-007 section 5.3);
// the caller decides that from the launcher secret.
func (l *Launch) Issue(privileged bool) (string, error) {
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

	l.codes = append(l.codes, launchCode{digest: sha256.Sum256([]byte(code)), created: l.now(), privileged: privileged})

	return code, nil
}

// Redeem is ok once for a code that is within its 120 seconds, and says
// whether that code was issued as privileged. The entry is deleted whatever
// the outcome, and five failures within 60 seconds lock the route (Locked) for
// the next 60 seconds.
func (l *Launch) Redeem(code string) (privileged, ok bool) {
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
		return false, false
	}

	privileged = l.codes[found].privileged
	l.codes = slices.Delete(l.codes, found, found+1)

	return privileged, true
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
