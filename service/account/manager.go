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

// Apply creates the account in a workspace that may hold data (SDD-005 section 5.16.3). It receives the
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

	// derive lets one Argon2id run at a time: each uses 64 MiB.
	derive sync.Mutex

	mu         sync.Mutex
	state      State
	username   string
	email      string
	idleLock   int
	keys       Keys
	sealer     *KeySealer
	lastActive time.Time
	failures   []time.Time
	onLock     []func()
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

	if m.state == StateLocked {
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

// Sealer is the Sealer of the store: clear text with no account, ErrLocked while locked, the data key while
// unlocked.
func (m *Manager) Sealer() Sealer { return managerSealer{m} }

type managerSealer struct{ m *Manager }

func (s managerSealer) with(fn func(Sealer) (string, error)) (string, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()

	switch s.m.state {
	case StateNone:
		return fn(Passthrough{})
	case StateLocked:
		return "", ErrLocked
	case StateUnlocked:
	}

	return fn(s.m.sealer)
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

	if m.State() != StateNone {
		return "", ErrExists
	}

	m.derive.Lock()
	defer m.derive.Unlock()

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

	if apply == nil {
		err = m.store.Create(ctx, rec)
	} else {
		err = apply(ctx, rec, fields, keys.Vault)
	}

	if err != nil {
		return "", err
	}

	m.mu.Lock()
	m.state, m.username, m.email, m.idleLock = StateLocked, rec.Username, rec.Email, rec.IdleLockMinutes
	m.mu.Unlock()

	return recovery, nil
}

// Login opens the account with a username or an e-mail address and the password. An unknown name and a wrong
// password cost the same and are the same error (ErrWrongPassword).
func (m *Manager) Login(ctx context.Context, identity, password string) error {
	if err := m.throttle(); err != nil {
		return err
	}

	m.derive.Lock()
	defer m.derive.Unlock()

	rec, err := m.store.Get(ctx)
	if errors.Is(err, ErrNoAccount) {
		return ErrWrongPassword
	}

	if err != nil {
		return err
	}

	keys, err := OpenWithPassword(rec, password)
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

	if err := m.throttle(); err != nil {
		return err
	}

	m.derive.Lock()
	defer m.derive.Unlock()

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

	if err := m.throttle(); err != nil {
		return err
	}

	m.derive.Lock()
	defer m.derive.Unlock()

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
	defer m.mu.Unlock()

	m.lockLocked()
	m.keys, m.sealer = keys, sealer
	m.state, m.username, m.email, m.idleLock = StateUnlocked, rec.Username, rec.Email, rec.IdleLockMinutes
	m.lastActive = m.now()
	m.failures = nil

	return nil
}

// Lock wipes the keys and runs the OnLock functions. It is a no-op when nothing is unlocked.
func (m *Manager) Lock() {
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
