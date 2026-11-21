package account

import (
	"encoding/base64"
	"errors"
	"strings"
)

// Prefix marks a sealed field (SDD-005 section 5.16.1).
const Prefix = "s1."

// The errors of a sealed field.
var (
	// ErrLocked is a read or a write that needs the data key while nobody is signed in.
	ErrLocked = errors.New("account: Stuarty is locked: unlock Stuarty with your password")
	// ErrUnsealed is a value that should be sealed and is not, or that does not open: a copied cell, a
	// damaged row, a value written by something that did not hold the key.
	ErrUnsealed = errors.New("account: a stored field is not sealed with the key of this account")
)

// Sealer seals and opens the user's fields (SDD-005 section 5.16). The store calls it inside its
// repositories; table, column and rowKey say where the value lives and are bound into the ciphertext, so a
// value moved to another cell does not open.
type Sealer interface {
	Seal(table, column, rowKey, plain string) (string, error)
	Open(table, column, rowKey, stored string) (string, error)
}

// Passthrough is the Sealer of a workspace that has no account: the fields are clear text, as before.
type Passthrough struct{}

// Seal implements Sealer.
func (Passthrough) Seal(_, _, _, plain string) (string, error) { return plain, nil }

// Open implements Sealer.
func (Passthrough) Open(_, _, _, stored string) (string, error) { return stored, nil }

// Sealing reports whether a Sealer seals the fields, as opposed to leaving them as clear text: it is false for
// a workspace with no account.
func Sealing(s Sealer) bool {
	if sealing, ok := s.(interface{ Sealing() bool }); ok {
		return sealing.Sealing()
	}

	_, plain := s.(Passthrough)

	return !plain
}

// KeySealer seals with one data key. It is what the migration uses and what the Manager uses while
// unlocked; the key buffer belongs to the caller, who wipes it.
type KeySealer struct {
	key []byte
}

// NewKeySealer returns a sealer over key, which must be KeySize bytes. The key is copied.
func NewKeySealer(key []byte) (*KeySealer, error) {
	if len(key) != KeySize {
		return nil, ErrCorrupt
	}

	return &KeySealer{key: append([]byte(nil), key...)}, nil
}

// Close zeroes the copy of the key.
func (s *KeySealer) Close() { wipe(s.key) }

func fieldAAD(table, column, rowKey string) []byte {
	return []byte("stuarty/field/v1\x00" + table + "." + column + "\x00" + rowKey)
}

// Seal implements Sealer. An empty value is sealed too, so the length of nothing is not readable.
func (s *KeySealer) Seal(table, column, rowKey, plain string) (string, error) {
	out, err := wrap(s.key, fieldAAD(table, column, rowKey), []byte(plain))
	if err != nil {
		return "", err
	}

	return Prefix + base64.RawURLEncoding.EncodeToString(out), nil
}

// Open implements Sealer.
func (s *KeySealer) Open(table, column, rowKey, stored string) (string, error) {
	raw, ok := strings.CutPrefix(stored, Prefix)
	if !ok {
		return "", ErrUnsealed
	}

	sealed, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", ErrUnsealed
	}

	plain, err := unwrap(s.key, fieldAAD(table, column, rowKey), sealed)
	if err != nil {
		return "", ErrUnsealed
	}

	return string(plain), nil
}

// IsSealed reports whether a stored value carries the prefix of a sealed field.
func IsSealed(stored string) bool { return strings.HasPrefix(stored, Prefix) }
