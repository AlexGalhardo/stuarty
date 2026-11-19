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
	"github.com/AlexGalhardo/stuarty/service/untrusted"
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
	// Test makes one harmless request to the destination.
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

// Kinds lists the kinds a connection may have, sorted.
func (m *Manager) Kinds() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return slices.Sorted(maps.Keys(m.connectors))
}

// Vault is the vault of the manager, for doctor and the tests of the wiring.
func (m *Manager) Vault() *vault.Vault { return m.vault }

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
	r, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}

	secret, err := m.vault.Get(r.SecretRef)
	if errors.Is(err, vault.ErrNotFound) {
		return ErrNoSecret
	}

	if err != nil {
		return err
	}

	defer clear(secret)

	return fn(Credential{
		Connection: View(r, true), Secret: secret, Client: outbound.Connection(m, r.ID, m.timeout, m.trust),
	})
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
		OK:     res.StatusCode >= 200 && res.StatusCode < 300,
		Status: res.StatusCode,
		Message: untrusted.Block(
			untrusted.SourceNetworkText,
			untrusted.Attrs{"connection": c.Connection.ID},
			string(body),
			testMessageBudget,
		),
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
