package connections

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/vault"
)

// DefaultTimeout is the time a request through a connection may take.
const DefaultTimeout = 15 * time.Second

// maxTestBody is how much of the answer of a destination a test reads.
const maxTestBody = 4 << 10

// testMessageBudget is the budget of the delimited text a test returns.
const testMessageBudget = 500

// Doer is the part of an HTTP client a connector needs; the client is always the
// gated one of internal/outbound.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Credential is what a connector gets for the length of one call: the
// connection, its secret and a client that can reach that connection only.
// The secret is not stored by the manager after the closure returns.
type Credential struct {
	Connection Connection
	Secret     []byte
	Client     Doer
	// SignedIn is true when the connection was made by a sign-in in the browser (SDD-014 section 13):
	// Secret is then the access token, or the key the provider issued, and nothing else.
	SignedIn bool
}

// Errors of a sign-in credential (SDD-014 section 13).
var (
	// ErrNeedsSignIn is a connection whose sign-in is no longer good: the person must sign in again.
	ErrNeedsSignIn = errors.New("connections: the sign-in of the connection must be done again")
	// ErrSignInUnavailable is a sign-in that could not be renewed now, for a reason that says nothing
	// about the grant (no network, a server error): it is kept as it is and may work later.
	ErrSignInUnavailable = errors.New("connections: the sign-in could not be renewed now")
)

// Prepared is what a Refresher decided about a stored secret.
type Prepared struct {
	// Store is the new stored secret, written to the vault before use; nil leaves it as it is.
	Store []byte
	// Use is the secret the closure receives; nil means the stored secret is used as it is.
	Use []byte
	// SignedIn says that the secret is the credential of a sign-in.
	SignedIn bool
}

// Refresher turns the stored secret of a sign-in into what a closure needs, renewing it when it is
// about to expire (SDD-014 section 13). It runs under the lock of the connection, with the gated
// client of that connection. A secret that is not a sign-in is none of its business: it returns the
// zero Prepared and no error. When it returns an error, a non-nil Store is still written (a refused
// grant is remembered), and the error ends the call.
type Refresher interface {
	Prepare(ctx context.Context, c Credential) (Prepared, error)
}

// TestResult is the outcome of a test request.
type TestResult struct {
	OK     bool
	Status int
	// Message is the start of the answer of the destination, as untrusted
	// text (SDD-007 section 8): a block delimited and truncated.
	Message string
}

// Connector is the code of a kind of connection. The kinds of the real
// providers are added by later epics; this epic has the test kind.
type Connector interface {
	// Test makes one harmless request to the destination. The Message of its result is built
	// with SafeMessage from the raw body, so an echoed credential is found before any cut.
	Test(ctx context.Context, c Credential) (TestResult, error)
}

// Options configure NewManager.
type Options struct {
	Store Store
	Vault *vault.Vault
	// Now is the clock; default time.Now.
	Now func() time.Time
	// Timeout of a request through a connection; default DefaultTimeout.
	Timeout time.Duration
	// Trust is for tests with a local TLS server only.
	Trust *outbound.TestOptions
}

// Manager is the registry of connections and the only reader of the vault.
// It is the outbound.Registry of the gate.
type Manager struct {
	store   Store
	vault   *vault.Vault
	now     func() time.Time
	timeout time.Duration
	trust   *outbound.TestOptions

	mu         sync.RWMutex
	connectors map[string]Connector
	refresher  Refresher

	// locks holds one lock per connection for the renewal of a sign-in.
	lockMu sync.Mutex
	locks  map[string]*sync.Mutex
}

// NewManager returns a manager with the test connector registered.
func NewManager(o Options) *Manager {
	m := &Manager{
		store: o.Store, vault: o.Vault, now: o.Now, timeout: o.Timeout, trust: o.Trust,
		connectors: map[string]Connector{KindTest: testConnector{}},
	}

	if m.now == nil {
		m.now = time.Now
	}

	if m.timeout <= 0 {
		m.timeout = DefaultTimeout
	}

	return m
}

// Register adds the connector of a kind.
func (m *Manager) Register(kind string, c Connector) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.connectors[kind] = c
}

// SetRefresher installs the refresher of the sign-in credentials; a manager without one hands every
// secret to a closure as it is stored.
func (m *Manager) SetRefresher(r Refresher) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.refresher = r
}

func (m *Manager) lockOf(id string) *sync.Mutex {
	m.lockMu.Lock()
	defer m.lockMu.Unlock()

	if m.locks == nil {
		m.locks = map[string]*sync.Mutex{}
	}

	l, ok := m.locks[id]
	if !ok {
		l = &sync.Mutex{}
		m.locks[id] = l
	}

	return l
}

// Kinds lists the kinds a connection may have, sorted.
func (m *Manager) Kinds() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return slices.Sorted(maps.Keys(m.connectors))
}

// Redact replaces every credential of the vault and its master key in data (SDD-014 section 9).
func (m *Manager) Redact(data []byte) []byte { return m.vault.Redact(data) }

// VaultStatus is what doctor and the diagnostic bundle may know of the vault: where the master key
// comes from, whether the file exists, and whether the key opens it. It carries no key and no secret.
type VaultStatus struct {
	Provider string
	Exists   bool
	// Err is nil when the vault opens or does not exist yet; else an error that Locked explains.
	Err error
}

// VaultStatus reports the state of the vault. The manager never hands the vault itself out, so no
// package but this one can call Vault.Get (SDD-014 section 3).
func (m *Manager) VaultStatus() VaultStatus {
	return VaultStatus{Provider: m.vault.Provider(), Exists: m.vault.Exists(), Err: m.vault.Verify()}
}

// List returns every connection, oldest first.
func (m *Manager) List(ctx context.Context) ([]Connection, error) {
	rows, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]Connection, 0, len(rows))

	for i := range rows {
		out = append(out, View(rows[i], m.secretSet(&rows[i])))
	}

	return out, nil
}

// Get returns one connection, or ErrNotFound.
func (m *Manager) Get(ctx context.Context, id string) (Connection, error) {
	r, err := m.store.Get(ctx, id)
	if err != nil {
		return Connection{}, err
	}

	return View(r, m.secretSet(&r)), nil
}

func (m *Manager) secretSet(r *Row) bool {
	ok, err := m.vault.Has(r.SecretRef)

	return err == nil && ok
}

// PutSecret keeps the credential of a connection in the vault.
func (m *Manager) PutSecret(ref string, secret []byte) error { return m.vault.Put(ref, secret) }

// DeleteSecret removes the credential of a connection from the vault.
func (m *Manager) DeleteSecret(ref string) error { return m.vault.Delete(ref) }

// Destination implements outbound.Registry: where a connection may go, asked
// at every request.
func (m *Manager) Destination(ctx context.Context, id string) (outbound.Destination, error) {
	r, err := m.store.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return outbound.Destination{}, outbound.ErrNoConnection
		}

		return outbound.Destination{}, err
	}

	return outbound.Destination{ID: r.ID, Hosts: r.Hosts}, nil
}

// Used implements outbound.Registry. A failure to record the use never stops
// the request.
func (m *Manager) Used(ctx context.Context, id string, at time.Time) {
	_ = m.store.Touch(context.WithoutCancel(ctx), id, at.UnixMilli())
}

// With is the only way to use a credential: it reads the secret of the
// connection, builds the gated client and calls fn. The secret does not
// outlive the call and is never returned.
func (m *Manager) With(ctx context.Context, id string, fn func(Credential) error) error {
	return m.WithTimeout(ctx, id, m.timeout, fn)
}

// WithTimeout is With for a request that may take longer than the default, such as the generation of a
// text (SDD-016 section 6): the client of the closure gives up after timeout.
func (m *Manager) WithTimeout(ctx context.Context, id string, timeout time.Duration, fn func(Credential) error) error {
	return m.use(ctx, id, timeout, true, fn)
}

// WithStored is WithTimeout for the sign-in itself (SDD-014 section 13): it hands the closure the secret
// as it is stored, with no renewal, so that a sign-in that is not finished, or that needs to be done
// again, can still exchange its code through the gated client of its connection. Nothing else uses it.
func (m *Manager) WithStored(ctx context.Context, id string, timeout time.Duration, fn func(Credential) error) error {
	return m.use(ctx, id, timeout, false, fn)
}

// WithPublic is With for a destination that must be on the public internet (the webhook of a blog, SDD-020
// section 46): the client refuses, at dial time, an address that is loopback, private, link-local, shared,
// unspecified or multicast. It is for a secret that is not a sign-in: nothing is renewed.
func (m *Manager) WithPublic(ctx context.Context, id string, fn func(Credential) error) error {
	r, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}

	secret, err := m.readSecret(r)
	if err != nil {
		return err
	}

	defer clear(secret)

	client := outbound.ConnectionPublic(m, r.ID, m.timeout, m.trust)

	return fn(Credential{Connection: View(r, true), Secret: secret, Client: client})
}

func (m *Manager) use(
	ctx context.Context,
	id string,
	timeout time.Duration,
	renew bool,
	fn func(Credential) error,
) error {
	r, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}

	client := outbound.Connection(m, r.ID, timeout, m.trust)

	secret, signedIn, err := m.secretFor(ctx, r, client, renew)
	if err != nil {
		return err
	}

	defer clear(secret)

	return fn(Credential{Connection: View(r, true), Secret: secret, Client: client, SignedIn: signedIn})
}

func (m *Manager) readSecret(r Row) ([]byte, error) {
	secret, err := m.vault.Get(r.SecretRef)
	if errors.Is(err, vault.ErrNotFound) {
		return nil, ErrNoSecret
	}

	return secret, err
}

// secretFor reads the secret of a connection and, for a sign-in, renews it and takes out the token. The
// lock of the connection is held from before the read to after the write, so a concurrent use sees the
// renewed tokens instead of spending a rotating refresh token twice (SDD-014 section 13).
func (m *Manager) secretFor(ctx context.Context, r Row, client Doer, renew bool) ([]byte, bool, error) {
	m.mu.RLock()
	ref := m.refresher
	m.mu.RUnlock()

	if !renew || ref == nil {
		secret, err := m.readSecret(r)

		return secret, false, err
	}

	l := m.lockOf(r.ID)
	l.Lock()
	defer l.Unlock()

	stored, err := m.readSecret(r)
	if err != nil {
		return nil, false, err
	}

	prep, perr := ref.Prepare(ctx, Credential{Connection: View(r, true), Secret: stored, Client: client})

	if prep.Store != nil {
		if werr := m.vault.Put(r.SecretRef, prep.Store); werr != nil && perr == nil {
			perr = werr
		}

		clear(prep.Store)
	}

	if perr != nil {
		clear(stored)
		clear(prep.Use)

		return nil, false, perr
	}

	if prep.Use == nil {
		return stored, false, nil
	}

	clear(stored)

	return prep.Use, prep.SignedIn, nil
}

// Test makes the test request of the kind of the connection.
func (m *Manager) Test(ctx context.Context, id string) (TestResult, error) {
	r, err := m.store.Get(ctx, id)
	if err != nil {
		return TestResult{}, err
	}

	m.mu.RLock()
	conn, ok := m.connectors[r.Kind]
	m.mu.RUnlock()

	if !ok {
		return TestResult{}, fmt.Errorf("connections: no connector for the kind %q", r.Kind)
	}

	var res TestResult

	err = m.With(ctx, id, func(c Credential) error {
		var terr error

		res, terr = conn.Test(ctx, c)

		return terr
	})

	// A destination can echo the credential it was sent; what the caller reads never holds it.
	res.Message = string(m.Redact([]byte(res.Message)))

	return res, err
}

// testConnector is the connector of the test kind: a GET of the endpoint with
// the secret as a bearer token.
type testConnector struct{}

func (testConnector) Test(ctx context.Context, c Credential) (TestResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Connection.Endpoint, http.NoBody)
	if err != nil {
		return TestResult{}, err
	}

	req.Header.Set("Authorization", "Bearer "+string(c.Secret))

	res, err := c.Client.Do(req)
	if err != nil {
		if errors.Is(err, outbound.ErrNoConnection) || errors.Is(err, outbound.ErrDestination) {
			return TestResult{}, err
		}

		// The transport error can name the address and never the header, but its text is not
		// ours to forward: a generic message says what happened.
		return TestResult{Message: "the destination could not be reached."}, nil
	}

	defer func() { _ = res.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(res.Body, maxTestBody))

	return TestResult{
		OK:      res.StatusCode >= 200 && res.StatusCode < 300,
		Status:  res.StatusCode,
		Message: SafeMessage(c, body),
	}, nil
}

// NewDefault is a manager over the store with a vault in the workspace that
// has no master key source: connections can be listed, and none can be
// created until the owner supplies a key provider (SDD-014 section 4).
func NewDefault(store Store, workspace string, now func() time.Time) *Manager {
	return NewManager(Options{
		Store: store, Now: now,
		Vault: vault.Open(filepath.Join(workspace, VaultFile), vault.NoProvider()),
	})
}

// VaultFile is the name of the vault file in the workspace.
const VaultFile = "vault.json"

// Locked says whether err is a failure to open the vault, with the reason that
// travels in details.reason and the text that says how to recover. The text is
// written by the product and holds no secret and no key.
func Locked(err error) (reason, hint string, ok bool) {
	switch {
	case errors.Is(err, ErrNoSecret):
		return "secret_missing", "the credential of this connection is not in the vault: connect the account again", true
	case errors.Is(err, vault.ErrKeyMissing):
		return "key_missing", err.Error(), true
	case errors.Is(err, vault.ErrWrongKey):
		return "wrong_key", err.Error(), true
	case errors.Is(err, vault.ErrNoKeyProvider):
		return "no_key_provider", err.Error(), true
	case errors.Is(err, vault.ErrCorrupt):
		return "corrupt", err.Error(), true
	case errors.Is(err, vault.ErrBadKey):
		return "bad_key", err.Error(), true
	}

	return "", "", false
}
