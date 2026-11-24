package account

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	rules "github.com/AlexGalhardo/stuarty/core/account"
)

// State is where the account stands.
type State string

// The three states of SDD-007 section 3.9.
const (
	// StateNone is a workspace with no account: it works as before, and the interface asks for one.
	StateNone State = "none"
	// StateLocked is an account with nobody signed in: the data key is not in memory.
	StateLocked State = "locked"
	// StateUnlocked is a person signed in: the data key is in memory.
	StateUnlocked State = "unlocked"
)

// Sign-in limits (SDD-007 section 3.9 item 3).
const (
	maxFailures   = 5
	failureWindow = time.Minute
)

// ValidationError is an input the rules of core/account refused. It holds fields and reasons, never values.
type ValidationError struct{ Problems []rules.Problem }

func (e *ValidationError) Error() string { return "account: the input is not valid" }

// RateLimitedError is a sign-in refused because of too many recent failures.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "account: too many failed attempts" }

// Apply creates the account in a workspace that may hold data (SDD-005 section 5.17.3). It receives the
// record to insert, a sealer over the new data key and the new vault master key; it makes the copy, seals the
// existing fields, re-seals the vault, verifies, inserts the record and removes the copy, or restores the copy
// and returns an error. Nothing it is given may outlive the call.
type Apply func(ctx context.Context, rec Record, fields Sealer, vaultKey []byte) error

// Options builds a Manager.
type Options struct {
	Store  Store
	Now    func() time.Time
	Params Params // zero means DefaultParams
}

// Manager holds the state of the account and, while a person is signed in, the keys. Every method is safe for
// concurrent use.
type Manager struct {
	store  Store
	now    func() time.Time
	params Params

	// derive lets one Argon2id run at a time, since each uses 256 MiB. It also serializes what rewrites the record.
	derive sync.Mutex

	mu    sync.Mutex
	state State
	// creating closes the studio while the account is being made: the state is still none, but a write now
	// would land in clear text in a column that is being sealed.
	creating   bool
	username   string
	email      string
	idleLock   int
	keys       Keys
	sealer     *KeySealer
	lastActive time.Time
	failures   []time.Time
	onLock     []func()
	beforeLock []func()
	onUnlock   []func()
	// opening closes the gate of the Service while the OnUnlock functions run: the keys are in place and the store is
	// being sealed or its index built, and a request now would race that work.
	opening bool
}

// NewManager returns a Manager that has not read the record; call Load.
func NewManager(o Options) *Manager {
	if o.Now == nil {
		o.Now = time.Now
	}

	if o.Params == (Params{}) {
		o.Params = DefaultParams
	}

	return &Manager{store: o.Store, now: o.Now, params: o.Params, state: StateNone}
}

// Load reads the record and sets the state: none, or locked. It never unlocks.
func (m *Manager) Load(ctx context.Context) error {
	rec, err := m.store.Get(ctx)

	m.mu.Lock()
	defer m.mu.Unlock()

	switch {
	case errors.Is(err, ErrNoAccount):
		m.lockLocked()
		m.state, m.username, m.email, m.idleLock = StateNone, "", "", 0

		return nil
	case err != nil:
		return err
	}

	if m.state == StateUnlocked {
		m.username, m.email, m.idleLock = rec.Username, rec.Email, rec.IdleLockMinutes

		return nil
	}

	m.state, m.username, m.email, m.idleLock = StateLocked, rec.Username, rec.Email, rec.IdleLockMinutes

	return nil
}

// State is the state of the account.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.state
}

// Username is the username of the account, or "" when there is none.
func (m *Manager) Username() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.username
}

// Email is the e-mail address of the account, or "" when there is none.
func (m *Manager) Email() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.email
}

// IdleLockMinutes is the idle lock, 0 meaning off.
func (m *Manager) IdleLockMinutes() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.idleLock
}

// Check is the gate of the Service: ErrLocked when an account exists and nobody is signed in.
func (m *Manager) Check() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state == StateLocked || m.creating || m.opening {
		return ErrLocked
	}

	return nil
}

// Touch records activity, which the idle lock counts from.
func (m *Manager) Touch() {
	m.mu.Lock()
	m.lastActive = m.now()
	m.mu.Unlock()
}

// OnLock registers a function that runs each time the keys are wiped, after they are: the vault forgets its
// key and the sessions end. It must not call back into the Manager.
func (m *Manager) OnLock(fn func()) {
	m.mu.Lock()
	m.onLock = append(m.onLock, fn)
	m.mu.Unlock()
}

// BeforeLock registers a function that runs each time the studio is about to lock, while the keys are still in
// memory: what must be saved with them (the transcript of a hosted agent session is sealed) is saved here. It runs
// on every Lock of an unlocked account, outside the lock of the Manager, so it may use the store; it must not call
// Lock.
func (m *Manager) BeforeLock(fn func()) {
	m.mu.Lock()
	m.beforeLock = append(m.beforeLock, fn)
	m.mu.Unlock()
}

// OnUnlock registers a function that runs once per successful sign-in, after the keys are in place and outside the
// lock of the Manager, in the order registered. The Service stays closed (Check answers ErrLocked) until every
// function has returned, so nothing races them; the Sealer and Files work in them. A function must not call Lock,
// Login or Recover. It does not run for a refused sign-in.
func (m *Manager) OnUnlock(fn func()) {
	m.mu.Lock()
	m.onUnlock = append(m.onUnlock, fn)
	m.mu.Unlock()
}

// Sealer is the Sealer of the store: clear text with no account, ErrLocked while locked, the data key while
// unlocked.
func (m *Manager) Sealer() Sealer { return managerSealer{m} }

type managerSealer struct{ m *Manager }

func (s managerSealer) with(fn func(Sealer) (string, error)) (string, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()

	if s.m.creating {
		return "", ErrLocked
	}

	switch s.m.state {
	case StateNone:
		return fn(Passthrough{})
	case StateLocked:
		return "", ErrLocked
	case StateUnlocked:
	}

	return fn(s.m.sealer)
}

// Sealing implements the optional method of Sealing: only a workspace with no account leaves clear text.
func (s managerSealer) Sealing() bool {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()

	return s.m.state != StateNone || s.m.creating
}

func (s managerSealer) Seal(table, column, rowKey, plain string) (string, error) {
	return s.with(func(k Sealer) (string, error) { return k.Seal(table, column, rowKey, plain) })
}

func (s managerSealer) Open(table, column, rowKey, stored string) (string, error) {
	return s.with(func(k Sealer) (string, error) { return k.Open(table, column, rowKey, stored) })
}

// VaultKey is a copy of the vault master key while unlocked, ErrLocked while locked and ErrNoAccount when there
// is no account. The caller wipes the copy.
func (m *Manager) VaultKey() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.creating {
		return nil, ErrLocked
	}

	switch m.state {
	case StateNone:
		return nil, ErrNoAccount
	case StateLocked:
		return nil, ErrLocked
	case StateUnlocked:
	}

	return slices.Clone(m.keys.Vault), nil
}

// Create makes the account. It checks the input with the rules of core/account, makes the keys, runs apply
// (the migration of what the workspace holds) and leaves the account locked: the person signs in next. It
// returns the recovery key, to be shown once.
func (m *Manager) Create(ctx context.Context, in rules.Input, apply Apply) (string, error) {
	if problems := rules.Validate(in); len(problems) > 0 {
		return "", &ValidationError{Problems: problems}
	}

	// The check is under the derive lock: a second request that waited for the first must see its account.
	m.derive.Lock()
	defer m.derive.Unlock()

	if err := m.beginCreate(); err != nil {
		return "", err
	}

	defer m.endCreate()

	rec, keys, recovery, err := seal(in.Username, strings.TrimSpace(in.Email), in.Password, m.params)
	if err != nil {
		return "", err
	}

	defer keys.Wipe()

	rec.CreatedMs = m.now().UnixMilli()
	rec.UpdatedMs = rec.CreatedMs

	fields, err := NewKeySealer(keys.Data)
	if err != nil {
		return "", err
	}

	defer fields.Close()

	// Once started the creation runs to its end: a browser that closes must not leave it half done.
	done := context.WithoutCancel(ctx)

	if apply == nil {
		err = m.store.Create(done, rec)
	} else {
		err = apply(done, rec, fields, keys.Vault)
	}

	if err != nil {
		// The state must follow the table: with the row there, a state of none would write clear text.
		if lerr := m.Load(done); lerr != nil {
			return "", errors.Join(err, lerr)
		}

		return "", err
	}

	m.mu.Lock()
	m.state, m.username, m.email, m.idleLock = StateLocked, rec.Username, rec.Email, rec.IdleLockMinutes
	m.mu.Unlock()

	return recovery, nil
}

// beginCreate closes the studio for the creation of the account.
func (m *Manager) beginCreate() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state != StateNone || m.creating {
		return ErrExists
	}

	m.creating = true

	return nil
}

func (m *Manager) endCreate() {
	m.mu.Lock()
	m.creating = false
	m.mu.Unlock()
}

// Login opens the account with a username or an e-mail address and the password. An unknown name and a wrong
// password cost the same and are the same error (ErrWrongPassword).
func (m *Manager) Login(ctx context.Context, identity, password string) error {
	// The limit is read after waiting for the derivation: read before, every request of a burst would pass it.
	m.derive.Lock()
	defer m.derive.Unlock()

	if err := m.throttle(); err != nil {
		return err
	}

	rec, err := m.store.Get(ctx)
	if errors.Is(err, ErrNoAccount) {
		return ErrWrongPassword
	}

	if err != nil {
		return err
	}

	keys, legacy, err := openWithPassword(rec, password)
	if err == nil && !sameIdentity(rec, identity) {
		keys.Wipe()

		err = ErrWrongPassword
	}

	if err != nil {
		if errors.Is(err, ErrWrongPassword) {
			m.fail()
		}

		return err
	}

	// An account made before the normalization (VV-380) opened with the bytes as typed: the verifier is rewritten
	// from the composed form, so the next sign-in is the ordinary one. A failure here changes nothing for this
	// sign-in and is tried again at the next.
	if legacy {
		if next, rerr := rewrap(rec, keys.Data, password, m.params); rerr == nil {
			next.UpdatedMs = m.now().UnixMilli()

			if m.store.Update(ctx, next) == nil {
				rec = next
			}
		}
	}

	return m.unlock(rec, keys)
}

func sameIdentity(rec Record, identity string) bool {
	identity = strings.TrimSpace(identity)

	return identity != "" && (identity == rec.Username || strings.EqualFold(identity, rec.Email))
}

// Recover opens the account with the recovery key, sets a new password and signs in.
func (m *Manager) Recover(ctx context.Context, recoveryKey, newPassword, confirmation string) error {
	problems := append(rules.Password(newPassword), rules.Confirmation(newPassword, confirmation)...)
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}

	m.derive.Lock()
	defer m.derive.Unlock()

	if err := m.throttle(); err != nil {
		return err
	}

	rec, err := m.store.Get(ctx)
	if errors.Is(err, ErrNoAccount) {
		return ErrWrongRecoveryKey
	}

	if err != nil {
		return err
	}

	keys, err := OpenWithRecoveryKey(rec, recoveryKey)
	if err != nil {
		if errors.Is(err, ErrWrongRecoveryKey) {
			m.fail()
		}

		return err
	}

	next, err := rewrap(rec, keys.Data, newPassword, m.params)
	if err != nil {
		keys.Wipe()

		return err
	}

	next.UpdatedMs = m.now().UnixMilli()

	if err := m.store.Update(ctx, next); err != nil {
		keys.Wipe()

		return err
	}

	return m.unlock(next, keys)
}

// ChangePassword sets a new password for a signed-in account. The current password is asked again; the data key
// is re-wrapped and nothing is re-encrypted.
func (m *Manager) ChangePassword(ctx context.Context, current, newPassword, confirmation string) error {
	problems := append(rules.Password(newPassword), rules.Confirmation(newPassword, confirmation)...)
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}

	if err := m.Check(); err != nil {
		return err
	}

	if m.State() != StateUnlocked {
		return ErrNoAccount
	}

	m.derive.Lock()
	defer m.derive.Unlock()

	if err := m.throttle(); err != nil {
		return err
	}

	rec, err := m.store.Get(ctx)
	if err != nil {
		return err
	}

	keys, err := OpenWithPassword(rec, current)
	if err != nil {
		if errors.Is(err, ErrWrongPassword) {
			m.fail()
		}

		return err
	}

	defer keys.Wipe()

	next, err := rewrap(rec, keys.Data, newPassword, m.params)
	if err != nil {
		return err
	}

	next.UpdatedMs = m.now().UnixMilli()

	return m.store.Update(ctx, next)
}

// NewRecoveryKey makes another recovery key for a signed-in account and returns it, to be shown once (VV-382). The
// answer that carried the first one can be lost, and the key exists nowhere else. The password is asked again, as for
// a change of it; the data key is re-wrapped under the new key, so the old key opens nothing any more, and nothing
// is re-encrypted.
func (m *Manager) NewRecoveryKey(ctx context.Context, password string) (string, error) {
	if err := m.Check(); err != nil {
		return "", err
	}

	if m.State() != StateUnlocked {
		return "", ErrNoAccount
	}

	m.derive.Lock()
	defer m.derive.Unlock()

	if err := m.throttle(); err != nil {
		return "", err
	}

	rec, err := m.store.Get(ctx)
	if err != nil {
		return "", err
	}

	keys, err := OpenWithPassword(rec, password)
	if err != nil {
		if errors.Is(err, ErrWrongPassword) {
			m.fail()
		}

		return "", err
	}

	defer keys.Wipe()

	next, recovery, err := rewrapRecovery(rec, keys.Data)
	if err != nil {
		return "", err
	}

	next.UpdatedMs = m.now().UnixMilli()

	if err := m.store.Update(ctx, next); err != nil {
		return "", err
	}

	return recovery, nil
}

// SetIdleLock stores the idle lock, in minutes (0 turns it off).
func (m *Manager) SetIdleLock(ctx context.Context, minutes int) error {
	if problems := rules.IdleMinutes(minutes); len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}

	switch m.State() {
	case StateNone:
		return ErrNoAccount
	case StateLocked:
		return ErrLocked
	case StateUnlocked:
	}

	// The update writes the whole record: under this lock it cannot put back the keys of a password just changed.
	m.derive.Lock()
	defer m.derive.Unlock()

	rec, err := m.store.Get(ctx)
	if err != nil {
		return err
	}

	rec.IdleLockMinutes = minutes
	rec.UpdatedMs = m.now().UnixMilli()

	if err := m.store.Update(ctx, rec); err != nil {
		return err
	}

	m.mu.Lock()
	m.idleLock = minutes
	m.lastActive = m.now()
	m.mu.Unlock()

	return nil
}

func (m *Manager) unlock(rec Record, keys Keys) error {
	sealer, err := NewKeySealer(keys.Data)
	if err != nil {
		keys.Wipe()

		return err
	}

	m.mu.Lock()

	m.lockLocked()
	m.keys, m.sealer = keys, sealer
	m.state, m.username, m.email, m.idleLock = StateUnlocked, rec.Username, rec.Email, rec.IdleLockMinutes
	m.lastActive = m.now()
	m.failures = nil

	hooks := slices.Clone(m.onUnlock)
	m.opening = len(hooks) > 0
	m.mu.Unlock()

	if len(hooks) == 0 {
		return nil
	}

	defer func() {
		m.mu.Lock()
		m.opening = false
		m.mu.Unlock()
	}()

	for _, fn := range hooks {
		fn()
	}

	return nil
}

// Lock wipes the keys and runs the OnLock functions. It is a no-op when nothing is unlocked.
func (m *Manager) Lock() {
	m.mu.Lock()

	if m.state != StateUnlocked {
		m.mu.Unlock()

		return
	}

	before := slices.Clone(m.beforeLock)
	m.mu.Unlock()

	// The keys are still there: what needs them is saved now, with the studio open.
	for _, fn := range before {
		fn()
	}

	m.mu.Lock()

	if m.state != StateUnlocked {
		m.mu.Unlock()

		return
	}

	m.lockLocked()
	m.state = StateLocked
	hooks := slices.Clone(m.onLock)
	m.mu.Unlock()

	for _, fn := range hooks {
		fn()
	}
}

// lockLocked zeroes the keys. The caller holds m.mu.
func (m *Manager) lockLocked() {
	m.keys.Wipe()

	if m.sealer != nil {
		m.sealer.Close()
		m.sealer = nil
	}
}

// Tick locks the account when the idle lock is on and the last activity is older than it. The owner calls it
// on a short timer; a test calls it with its own clock.
func (m *Manager) Tick() {
	m.mu.Lock()
	idle := m.state == StateUnlocked && m.idleLock > 0 &&
		m.now().Sub(m.lastActive) >= time.Duration(m.idleLock)*time.Minute
	m.mu.Unlock()

	if idle {
		m.Lock()
	}
}

// Close wipes the keys, for the exit of the process.
func (m *Manager) Close() { m.Lock() }

func (m *Manager) throttle() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	m.failures = slices.DeleteFunc(m.failures, func(t time.Time) bool { return now.Sub(t) >= failureWindow })

	if len(m.failures) >= maxFailures {
		return &RateLimitedError{RetryAfter: failureWindow - now.Sub(m.failures[0])}
	}

	return nil
}

func (m *Manager) fail() {
	m.mu.Lock()
	m.failures = append(m.failures, m.now())
	m.mu.Unlock()
}
