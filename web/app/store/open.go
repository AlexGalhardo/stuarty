package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/database/migrations"

	"github.com/AlexGalhardo/vukem/database"
	"github.com/AlexGalhardo/vukem/queue"
	"github.com/AlexGalhardo/vukem/schema"
	"github.com/AlexGalhardo/vukem/session"
)

// QueueOptions are the vukem queue tables of SDD-005 section 14.1. The editor
// keeps its own jobs table (ADR-0018), so the defaults of vukem are not used.
var QueueOptions = queue.DatabaseOptions{Table: "queue_jobs", FailedTable: "queue_failed_jobs"}

// BackupsKept is how many database backups survive a migration (SDD-005 section 7 step 3).
const BackupsKept = 5

// Migrations is every migration of the product: the vukem queue tables first,
// then the editor tables.
func Migrations() []schema.Migration {
	return slices.Concat(queue.Migrations(QueueOptions), migrations.All(), session.Migrations(""))
}

// Options configures Open. The zero value is valid.
type Options struct {
	// SchemaVersion is the highest document schema_version this binary knows
	// (timeline.CurrentSchemaVersion). A stored project or template above it
	// refuses the start. Zero skips the check.
	SchemaVersion int
	// Migrations replaces Migrations(); tests use it.
	Migrations []schema.Migration
	// Now is the clock for backup names; default time.Now.
	Now func() time.Time
}

// NewerWorkspaceError is returned when the workspace was written by a newer
// version of the product. Nothing was written (SDD-005 section 7 step 2).
type NewerWorkspaceError struct {
	Detail string
}

func (e *NewerWorkspaceError) Error() string {
	return "this workspace was written by a newer version (" + e.Detail + "); install a version that is at least as new"
}

// Open runs steps 1 to 4 of SDD-005 section 7: open db.sqlite, refuse a
// workspace from a newer version, back the database up when migrations are
// pending, and run them. The caller holds the workspace lock.
func Open(ctx context.Context, dir workspace.Dir, o Options) (*Store, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	migs := o.Migrations
	if migs == nil {
		migs = Migrations()
	}

	path := filepath.ToSlash(dir.Path("db.sqlite"))
	if i := strings.IndexAny(path, "?#%"); i >= 0 {
		return nil, fmt.Errorf("%w: it holds %q", workspace.ErrUnusablePath, path[i])
	}
	if err := touchPrivate(dir.Path("db.sqlite")); err != nil {
		return nil, err
	}

	db, err := database.Open(ctx, "file:"+path)
	if err != nil {
		return nil, fmt.Errorf("store: open database: %w", err)
	}
	s := newStore(db)
	if err := s.start(ctx, dir, migs, o); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// touchPrivate creates the database file with mode 0600 before it is first
// opened; the -wal and -shm files inherit it on Unix (SDD-007 section 3.2).
func touchPrivate(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("store: create database file: %w", err)
	}
	return f.Close()
}

func (s *Store) start(ctx context.Context, dir workspace.Dir, migs []schema.Migration, o Options) error {
	if err := s.assertPragmas(ctx); err != nil {
		return err
	}
	m, err := schema.NewMigrator(s.db, migs)
	if err != nil {
		return err
	}

	status, err := m.Status(ctx)
	if err != nil {
		return fmt.Errorf("store: read migrations: %w", err)
	}
	var anyRan bool
	for _, st := range status {
		if st.Missing {
			return &NewerWorkspaceError{Detail: "migration " + st.ID + " is unknown"}
		}
		anyRan = anyRan || st.Ran
	}
	if err := s.checkSchemaVersions(ctx, o.SchemaVersion); err != nil {
		return err
	}

	pending, err := m.Pending(ctx)
	if err != nil {
		return err
	}
	if len(pending) > 0 && anyRan {
		if err := s.backup(ctx, dir, pending[0].ID, o.Now()); err != nil {
			return err
		}
	}

	if _, err := m.Up(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) assertPragmas(ctx context.Context) error {
	var mode string
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("store: read journal_mode: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("store: journal_mode is %q, want wal; is the workspace on a local file system?", mode)
	}
	var fk int
	if err := s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return fmt.Errorf("store: read foreign_keys: %w", err)
	}
	if fk != 1 {
		return errors.New("store: foreign_keys is off")
	}
	return nil
}

// checkSchemaVersions refuses a stored document newer than the binary knows.
func (s *Store) checkSchemaVersions(ctx context.Context, known int) error {
	if known <= 0 {
		return nil
	}
	// timelines holds the documents since ADR-0039; projects held them before, and is then a production table
	// that has no schema_version, so a table counts only when it has the column.
	for _, table := range []string{"timelines", "projects", "templates"} {
		var exists int
		err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info(?) WHERE name = 'schema_version'", table).
			Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			continue
		}
		var highest sql.NullInt64
		if err := s.db.QueryRowContext(ctx, "SELECT max(schema_version) FROM "+table).Scan(&highest); err != nil {
			return err
		}
		if highest.Valid && highest.Int64 > int64(known) {
			return &NewerWorkspaceError{
				Detail: fmt.Sprintf("%s hold schema version %d and this binary knows %d", table, highest.Int64, known),
			}
		}
	}
	return nil
}

// backup copies the database with VACUUM INTO before a migration runs and
// keeps the newest BackupsKept copies. A failure stops the start: no
// migration runs when its backup cannot be written.
func (s *Store) backup(ctx context.Context, dir workspace.Dir, firstPending string, now time.Time) error {
	if err := os.MkdirAll(dir.Path("backups"), 0o700); err != nil {
		return fmt.Errorf("store: backups: %w", err)
	}
	name := fmt.Sprintf("db-%s-before-%s.sqlite", now.UTC().Format("20060102T150405"), firstPending)
	target := dir.Path("backups", name)
	if _, err := os.Stat(target); err == nil {
		return nil // one backup per second and pending set is enough
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", target); err != nil {
		return fmt.Errorf("store: back up the database before migrating: %w", err)
	}
	return pruneBackups(dir.Path("backups"))
}

func pruneBackups(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if n := e.Name(); strings.HasPrefix(n, "db-") && strings.HasSuffix(n, ".sqlite") {
			names = append(names, n)
		}
	}
	slices.Sort(names) // the UTC timestamp sorts oldest first
	for len(names) > BackupsKept {
		if err := os.Remove(filepath.Join(dir, names[0])); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		names = names[1:]
	}
	return nil
}
