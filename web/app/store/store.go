// Package store implements service.Store over vukem's database package
// (SDD-005). It contains SQL and nothing else: it never decodes a document,
// never calls core/ and never publishes an event.
package store

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"

	"github.com/AlexGalhardo/vukem/database"
)

// Store is the service.Store over one workspace database.
type Store struct {
	repos
	db    *database.DB
	locks *projectLocks
}

var _ service.Store = (*Store)(nil)

func newStore(db *database.DB) *Store {
	return &Store{ex: db, db: db, locks: newProjectLocks()}
}

// DB returns the underlying database, for the migrator and for vukem's queue.
func (s *Store) DB() *database.DB { return s.db }

// Close checkpoints the WAL (so db.sqlite is self-contained when copied) and
// closes the pools.
func (s *Store) Close() error {
	_, cpErr := s.db.ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)")
	return errors.Join(cpErr, s.db.Close())
}

// Write runs fn in one write transaction.
func (s *Store) Write(ctx context.Context, fn func(ctx context.Context, tx service.Tx) error) error {
	return s.db.Tx(ctx, func(dtx *database.Tx) error {
		return fn(ctx, newTx(dtx))
	})
}

// WriteProject takes the in-process lock of the project, loads its row inside
// the transaction and runs fn. The lock is held until the transaction and its
// after-commit hooks are done, so commits of one project and the events they
// publish stay in revision order (SDD-005 section 9.1).
func (s *Store) WriteProject(
	ctx context.Context,
	projectID string,
	fn func(ctx context.Context, tx service.ProjectTx) error,
) error {
	release, err := s.locks.acquire(ctx, projectID)
	if err != nil {
		return err
	}
	defer release()

	return s.db.Tx(ctx, func(dtx *database.Tx) error {
		t := newTx(dtx)
		row, err := t.Projects().Get(ctx, projectID)
		if err != nil {
			return err
		}
		return fn(ctx, &projectTx{tx: t, row: row})
	})
}

type tx struct {
	repos
	dtx *database.Tx
}

func newTx(dtx *database.Tx) *tx { return &tx{ex: dtx, dtx: dtx} }

// Executor is the transaction as vukem's executor, for the queue to insert its
// message in the same transaction as the job row (SDD-005 section 14.1).
func (t *tx) Executor() database.Executor { return t.dtx }

func (t *tx) AfterCommit(fn func()) {
	t.dtx.AfterCommit(func(context.Context) { fn() })
}

type projectTx struct {
	tx  *tx
	row service.ProjectRow
}

func (p *projectTx) Projects() service.ProjectRepo        { return p.tx.Projects() }
func (p *projectTx) Operations() service.OperationRepo    { return p.tx.Operations() }
func (p *projectTx) Idempotency() service.IdempotencyRepo { return p.tx.Idempotency() }
func (p *projectTx) Assets() service.AssetRepo            { return p.tx.Assets() }
func (p *projectTx) Jobs() service.JobRepo                { return p.tx.Jobs() }
func (p *projectTx) Templates() service.TemplateRepo      { return p.tx.Templates() }
func (p *projectTx) Backups() service.BackupRepo          { return p.tx.Backups() }
func (p *projectTx) Audit() service.AuditRepo             { return p.tx.Audit() }
func (p *projectTx) Fonts() service.FontRepo              { return p.tx.Fonts() }
func (p *projectTx) BrandKit() service.BrandKitRepo       { return p.tx.BrandKit() }
func (p *projectTx) Connections() connections.Store       { return p.tx.Connections() }
func (p *projectTx) AfterCommit(fn func())                { p.tx.AfterCommit(fn) }
func (p *projectTx) Project() service.ProjectRow          { return p.row }
func (p *projectTx) Executor() database.Executor          { return p.tx.Executor() }

// projectLocks is a map projectID -> channel of capacity 1, acquired with a
// select on the channel and ctx.Done(). An entry lives only while someone
// holds or waits for it, so the map is bounded by the number of projects in
// use at once, and an id that does not exist leaves nothing behind.
type projectLocks struct {
	mu sync.Mutex
	m  map[string]*lockEntry
}

type lockEntry struct {
	ch   chan struct{}
	refs int
}

func newProjectLocks() *projectLocks { return &projectLocks{m: map[string]*lockEntry{}} }

func (l *projectLocks) acquire(ctx context.Context, id string) (release func(), err error) {
	l.mu.Lock()
	e := l.m[id]
	if e == nil {
		e = &lockEntry{ch: make(chan struct{}, 1)}
		l.m[id] = e
	}
	e.refs++
	l.mu.Unlock()

	drop := func() {
		l.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(l.m, id)
		}
		l.mu.Unlock()
	}

	select {
	case e.ch <- struct{}{}:
		return func() {
			<-e.ch
			drop()
		}, nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}

// uniqueChecker is implemented by *database.DB and *database.Tx.
type uniqueChecker interface{ UniqueViolation(error) bool }

func isUnique(ex database.Executor, err error) bool {
	if err == nil {
		return false
	}
	if u, ok := ex.(uniqueChecker); ok {
		return u.UniqueViolation(err)
	}
	return false
}

// mapNoRows turns sql.ErrNoRows into service.ErrNotFound.
func mapNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrNotFound
	}
	return err
}
