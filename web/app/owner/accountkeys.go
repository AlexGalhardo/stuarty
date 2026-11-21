package owner

import (
	"errors"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/vault"
)

// accountKeys is the vault key provider of a workspace that has an account (ADR-0049): the master key is the
// random one wrapped by the data key, so it exists in memory only while a person is signed in. With no account
// yet the provider is the one the environment, the file or the keychain named.
type accountKeys struct {
	m        *account.Manager
	fallback vault.KeyProvider
}

// Name implements vault.KeyProvider.
func (k accountKeys) Name() string {
	if k.m.State() == account.StateNone {
		return k.fallback.Name()
	}

	return "the account"
}

// Load implements vault.KeyProvider. A locked studio holds no key.
func (k accountKeys) Load() ([]byte, error) {
	key, err := k.m.VaultKey()

	switch {
	case err == nil:
		return key, nil
	case errors.Is(err, account.ErrNoAccount):
		return k.fallback.Load()
	}

	return nil, vault.ErrKeyNotFound
}

// Save implements vault.KeyProvider: the key of an account is made with the account and never saved here.
func (k accountKeys) Save(key []byte) error {
	if k.m.State() == account.StateNone {
		return k.fallback.Save(key)
	}

	return vault.ErrReadOnly
}
