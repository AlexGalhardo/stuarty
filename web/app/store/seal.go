package store

import (
	"strconv"
	"sync"

	"github.com/AlexGalhardo/stuarty/service/account"

	"github.com/AlexGalhardo/vukem/database"
)

// sealRef holds the sealer of a store. The store is opened before the account manager exists (the manager reads
// the account from the store), so the sealer is set afterwards; until then the fields are clear text, as they
// are in a workspace with no account (SDD-005 section 5.16.1).
type sealRef struct {
	mu sync.RWMutex
	s  account.Sealer
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

// UniqueViolation lets isUnique see through the wrapper to the driver of the database or transaction.
func (e sealedExec) UniqueViolation(err error) bool { return isUnique(e.Executor, err) }

// sealerOf is the sealer behind an executor; the clear-text one for an executor that carries none.
func sealerOf(ex database.Executor) account.Sealer {
	if se, ok := ex.(sealedExec); ok {
		return se.ref.get()
	}

	return account.Passthrough{}
}

// sealField seals a value of a sealed column (SDD-005 section 5.16.2); the row key binds it to its row.
func sealField(sl account.Sealer, table, column, rowKey, plain string) (string, error) {
	return sl.Seal(table, column, rowKey, plain)
}

// openField opens a value of a sealed column.
func openField(sl account.Sealer, table, column, rowKey, stored string) (string, error) {
	return sl.Open(table, column, rowKey, stored)
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
