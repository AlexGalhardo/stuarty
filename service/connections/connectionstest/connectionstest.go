// Package connectionstest is a connection with a credential, for the tests of the packages that use
// connections (the provider adapters, the connectors of later epics). It is a test helper: the
// product never imports it. It lives under service/connections so that it may build a vault, which no
// other package may (internal/tools/importcheck).
package connectionstest

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/vault"
)

// ID is the id of the connection a Harness holds.
const ID = "con_testtesttt"

// Harness is one connection, with its secret in a vault of its own, over an in-memory store.
type Harness struct {
	Manager *connections.Manager
	Store   *Store
}

// New makes a connection of the given kind to endpoint (https, or http on a loopback host) whose
// credential is secret.
func New(t *testing.T, kind, endpoint, secret string) *Harness {
	t.Helper()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 7)
	}

	store := &Store{rows: map[string]connections.Row{}}
	m := connections.NewManager(connections.Options{
		Store: store, Vault: vault.Open(filepath.Join(t.TempDir(), "vault.json"), vault.NewMemoryProvider(key)),
	})

	spec, ferr := connections.Normalize(connections.Spec{
		Kind: kind, Label: "test", Endpoint: endpoint, Accepts: []string{connections.AcceptText},
	}, []string{kind})
	if ferr != nil {
		t.Fatal(ferr)
	}

	if err := m.PutSecret(ID, []byte(secret)); err != nil {
		t.Fatal(err)
	}

	err := store.Insert(context.Background(), connections.Row{
		ID: ID, Kind: spec.Kind, Label: spec.Label, Endpoint: spec.Endpoint, Hosts: spec.Hosts,
		Accepts: spec.Accepts, SecretRef: ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	return &Harness{Manager: m, Store: store}
}

// Do runs fn with the credential of the connection, as a connector would: the secret and the gated
// client exist for the length of the call.
func (h *Harness) Do(t *testing.T, fn func(ctx context.Context, c connections.Credential)) {
	t.Helper()

	err := h.Manager.With(context.Background(), ID, func(c connections.Credential) error {
		fn(context.Background(), c)

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Store is an in-memory connections.Store.
type Store struct {
	mu   sync.Mutex
	rows map[string]connections.Row
}

// List implements connections.Store.
func (s *Store) List(context.Context) ([]connections.Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]connections.Row, 0, len(s.rows))
	for _, id := range slices.Sorted(maps.Keys(s.rows)) {
		out = append(out, s.rows[id])
	}

	return out, nil
}

// Get implements connections.Store.
func (s *Store) Get(_ context.Context, id string) (connections.Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.rows[id]
	if !ok {
		return connections.Row{}, connections.ErrNotFound
	}

	return r, nil
}

// Count implements connections.Store.
func (s *Store) Count(context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.rows), nil
}

// Insert implements connections.Store.
func (s *Store) Insert(_ context.Context, r connections.Row) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.rows[r.ID] = r

	return nil
}

// Delete implements connections.Store.
func (s *Store) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.rows[id]; !ok {
		return connections.ErrNotFound
	}

	delete(s.rows, id)

	return nil
}

// Touch implements connections.Store.
func (s *Store) Touch(context.Context, string, int64) error { return nil }
