package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/AlexGalhardo/stuarty/service/account"

	"github.com/AlexGalhardo/vukem/database"
)

// accountRepo is account.Store over the table accounts (SDD-005 section 5.17). The row holds only wrapped keys
// and a verifier: nothing here is sealed by the field sealer, since the keys that would seal it live in it.
type accountRepo struct{ ex database.Executor }

const accountColumns = `username, email, kdf, verifier, key_by_password, key_by_recovery, vault_key,
 idle_lock_minutes, created_ms, updated_ms`

// Get implements account.Store.
func (r accountRepo) Get(ctx context.Context) (account.Record, error) {
	var (
		rec account.Record
		kdf string
	)

	err := r.ex.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = 1`).Scan(
		&rec.Username, &rec.Email, &kdf, &rec.Verifier, &rec.KeyByPassword, &rec.KeyByRecovery, &rec.VaultKey,
		&rec.IdleLockMinutes, &rec.CreatedMs, &rec.UpdatedMs)
	if errors.Is(err, sql.ErrNoRows) {
		return account.Record{}, account.ErrNoAccount
	}

	if err != nil {
		return account.Record{}, err
	}

	if err := json.Unmarshal([]byte(kdf), &rec.KDF); err != nil {
		return account.Record{}, account.ErrCorrupt
	}

	return rec, nil
}

// Create implements account.Store.
func (r accountRepo) Create(ctx context.Context, rec account.Record) error {
	kdf, err := json.Marshal(rec.KDF)
	if err != nil {
		return err
	}

	_, err = r.ex.ExecContext(ctx, `INSERT INTO accounts (id, `+accountColumns+`)
 VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.Username, rec.Email, string(kdf), rec.Verifier, rec.KeyByPassword, rec.KeyByRecovery, rec.VaultKey,
		rec.IdleLockMinutes, rec.CreatedMs, rec.UpdatedMs)
	if err != nil {
		if existing, gerr := r.Get(ctx); gerr == nil && existing.Username != "" {
			return account.ErrExists
		}

		return err
	}

	return nil
}

// Update implements account.Store.
func (r accountRepo) Update(ctx context.Context, rec account.Record) error {
	kdf, err := json.Marshal(rec.KDF)
	if err != nil {
		return err
	}

	n, err := rowsAffected(r.ex.ExecContext(ctx, `UPDATE accounts SET username = ?, email = ?, kdf = ?, verifier = ?,
 key_by_password = ?, key_by_recovery = ?, vault_key = ?, idle_lock_minutes = ?, updated_ms = ? WHERE id = 1`,
		rec.Username, rec.Email, string(kdf), rec.Verifier, rec.KeyByPassword, rec.KeyByRecovery, rec.VaultKey,
		rec.IdleLockMinutes, rec.UpdatedMs))
	if err != nil {
		return err
	}

	if n == 0 {
		return account.ErrNoAccount
	}

	return nil
}
