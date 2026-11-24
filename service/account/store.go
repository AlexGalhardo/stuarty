package account

import (
	"context"
	"errors"
)

// ErrNoAccount is a workspace that has no account yet.
var ErrNoAccount = errors.New("account: no account exists in this workspace")

// ErrExists is a second account: a workspace has one.
var ErrExists = errors.New("account: an account exists already")

// Record is the one row of the table accounts (SDD-005 section 5.17). Every secret in it is wrapped or derived:
// there is no password, no recovery key and no clear key.
type Record struct {
	Username        string
	Email           string
	KDF             KDF
	Verifier        []byte
	KeyByPassword   []byte
	KeyByRecovery   []byte
	VaultKey        []byte
	IdleLockMinutes int
	CreatedMs       int64
	UpdatedMs       int64
	// SealVersion is how much of the workspace the account has sealed (SDD-005 section 5.17.4): 0 for an account
	// made before ADR-0053, whose documents, edit log and asset notes are still clear text until the first unlock
	// seals them, 1 once everything is sealed. An account is made at 1. Update never changes it.
	SealVersion int
}

// SealedAll is the SealVersion of a workspace whose every sealed column is sealed.
const SealedAll = 1

// Store keeps the record. The implementation is in web/app/store.
type Store interface {
	// Get returns the record, or ErrNoAccount.
	Get(ctx context.Context) (Record, error)
	// Create inserts the record, or returns ErrExists.
	Create(ctx context.Context, rec Record) error
	// Update replaces the record, or returns ErrNoAccount.
	Update(ctx context.Context, rec Record) error
}
