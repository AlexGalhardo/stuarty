package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/AlexGalhardo/stuarty/service/account"

	"github.com/AlexGalhardo/vukem/database"
)

// sealRef holds the sealer of a store. The store is opened before the account manager exists (the manager reads
// the account from the store), so the sealer is set afterwards; until then the fields are clear text, as they
// are in a workspace with no account (SDD-005 section 5.17.1).
type sealRef struct {
	mu sync.RWMutex
	s  account.Sealer
	// lenient is true while the account was made before ADR-0053 and the second stage of sealed columns (sealedCellsV2)
	// has not been sealed yet: those columns are read as clear text where a value is not sealed (SDD-005 section
	// 5.17.4). It is set when the store opens an account of seal version 0 and cleared when the version becomes 1.
	lenient atomic.Bool
}

func (r *sealRef) get() account.Sealer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.s == nil {
		return account.Passthrough{}
	}

	return r.s
}

func (r *sealRef) set(s account.Sealer) {
	r.mu.Lock()
	r.s = s
	r.mu.Unlock()
}

// sealedExec is the executor of every repository: the database or a transaction, carrying the sealer the
// repositories that hold sealed columns ask for. It adds nothing to the SQL.
type sealedExec struct {
	database.Executor
	ref *sealRef
}

// ExecContext runs the statement and says E_LOCKED when a guard of the account (installSealGuards) refused a clear
// value: the write sealed before the account existed and reached the database after it did.
func (e sealedExec) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	res, err := e.Executor.ExecContext(ctx, query, args...)
	if err != nil && strings.Contains(err.Error(), guardMessage) {
		return nil, account.ErrLocked
	}

	return res, err
}

// UniqueViolation lets isUnique see through the wrapper to the driver of the database or transaction.
func (e sealedExec) UniqueViolation(err error) bool { return isUnique(e.Executor, err) }

// sealerOf is the sealer behind an executor; the clear-text one for an executor that carries none.
func sealerOf(ex database.Executor) account.Sealer {
	if se, ok := ex.(sealedExec); ok {
		return se.ref.get()
	}

	return account.Passthrough{}
}

// sealField seals a value of a sealed column (SDD-005 section 5.17.2); the row key binds it to its row.
func sealField(sl account.Sealer, table, column, rowKey, plain string) (string, error) {
	return sl.Seal(table, column, rowKey, plain)
}

// openField opens a value of a sealed column.
func openField(sl account.Sealer, table, column, rowKey, stored string) (string, error) {
	return sl.Open(table, column, rowKey, stored)
}

// isLenient says whether the second stage of sealed columns may still hold clear text (SDD-005 section 5.17.4).
func isLenient(ex database.Executor) bool {
	if se, ok := ex.(sealedExec); ok {
		return se.ref.lenient.Load()
	}

	return false
}

// sealDoc seals a value of a second-stage column (documents, the edit log, backups, results, asset notes).
func sealDoc(ex database.Executor, table, column, rowKey, plain string) (string, error) {
	return sealField(sealerOf(ex), table, column, rowKey, plain)
}

// openDoc opens a value of a second-stage column. A workspace whose account is of seal version 0 still holds clear
// text there until the first unlock has sealed it, so a value that does not open as sealed is the clear value; with a
// locked studio it is still E_LOCKED, never the clear text.
func openDoc(ex database.Executor, table, column, rowKey, stored string) (string, error) {
	plain, err := openField(sealerOf(ex), table, column, rowKey, stored)
	if errors.Is(err, account.ErrUnsealed) && isLenient(ex) {
		return stored, nil
	}

	return plain, err
}

// revKey is the row key of a text revision.
func revKey(itemID string, revision int) string { return itemID + "/" + strconv.Itoa(revision) }

// eventKey is the row key of an event of a chat session.
func eventKey(sessionID string, seq int64) string {
	return sessionID + "/" + strconv.FormatInt(seq, 10)
}

// SetSealer sets the sealer of the sealed columns: the account manager's, which seals while a person is
// signed in and refuses while the studio is locked.
func (s *Store) SetSealer(sl account.Sealer) { s.ref.set(sl) }
