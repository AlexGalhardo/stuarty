package vault

import (
	"bytes"
	"errors"
	"sync"
)

// The errors of a KeyProvider.
var (
	// ErrKeyNotFound is a provider that holds no master key.
	ErrKeyNotFound = errors.New("vault: the provider holds no master key")
	// ErrReadOnly is a provider that cannot store a generated master key.
	ErrReadOnly = errors.New("vault: the provider cannot store a master key")
)

// KeyProvider finds the master key of a vault and, for a new vault, may keep
// a generated one (SDD-014 section 4).
type KeyProvider interface {
	// Name says where the key lives, for messages and for doctor; it never
	// contains the key.
	Name() string
	// Load returns the key, or ErrKeyNotFound when the provider has none.
	Load() ([]byte, error)
	// Save stores a generated key, or returns ErrReadOnly.
	Save(key []byte) error
}

// MemoryProvider keeps the key in memory only. Tests and the tools that need
// a vault for one run use it; the product never does.
type MemoryProvider struct {
	mu  sync.Mutex
	key []byte
}

// NewMemoryProvider returns a provider that holds key; a nil key starts empty.
func NewMemoryProvider(key []byte) *MemoryProvider {
	return &MemoryProvider{key: bytes.Clone(key)}
}

// Name implements KeyProvider.
func (p *MemoryProvider) Name() string { return "memory" }

// Load implements KeyProvider.
func (p *MemoryProvider) Load() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.key == nil {
		return nil, ErrKeyNotFound
	}

	return bytes.Clone(p.key), nil
}

// Save implements KeyProvider.
func (p *MemoryProvider) Save(key []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.key = bytes.Clone(key)

	return nil
}
