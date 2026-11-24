package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexGalhardo/vukem/database"
)

// SnapshotDatabase writes a consistent copy of the database at dbPath to dest with VACUUM INTO, the
// SQLite online backup: it reads one transaction of a database that another process (the server) may
// be writing, and never copies the live file (SDD-009 section 21.4). dest must not exist. The
// copy is not migrated and no lock of the workspace is needed.
func SnapshotDatabase(ctx context.Context, dbPath, dest string) error {
	path := filepath.ToSlash(dbPath)
	if i := strings.IndexAny(path, "?#%"); i >= 0 {
		return fmt.Errorf("store: snapshot: the path holds %q", path[i])
	}

	if info, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("store: snapshot: there is no database at %s: this is not a workspace", dbPath)
		}

		return fmt.Errorf("store: snapshot: %w", err)
	} else if info.Size() == 0 {
		return fmt.Errorf("store: snapshot: the database at %s is empty: the workspace was never started", dbPath)
	}

	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("store: snapshot: %s already exists", dest)
	}

	db, err := database.Open(ctx, "file:"+path)
	if err != nil {
		return fmt.Errorf("store: snapshot: open the database: %w", err)
	}

	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		_ = os.Remove(dest)

		return fmt.Errorf("store: snapshot: %w", err)
	}

	return os.Chmod(dest, 0o600)
}

// DatabaseHasAccount reports whether the database file at dbPath holds a local account (ADR-0052). It only reads:
// the file may be in a backup folder, whose manifest lists every file, so it leaves no log or shared memory beside
// it. A missing file or a database from before the accounts table has none.
func DatabaseHasAccount(ctx context.Context, dbPath string) (bool, error) {
	path := filepath.ToSlash(dbPath)
	if i := strings.IndexAny(path, "?#%"); i >= 0 {
		return false, fmt.Errorf("store: account check: the path holds %q", path[i])
	}

	if info, err := os.Stat(dbPath); err != nil || info.Size() == 0 {
		if err == nil || errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("store: account check: %w", err)
	}

	// Immutable leaves nothing beside the file, but it ignores a write-ahead log, which a live workspace may hold.
	dsn := "file:" + path + "?mode=ro&immutable=1"
	if info, err := os.Stat(dbPath + "-wal"); err == nil && info.Size() > 0 {
		dsn = "file:" + path + "?mode=ro"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return false, fmt.Errorf("store: account check: %w", err)
	}

	defer func() { _ = db.Close() }()

	var tables int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'accounts'`).Scan(&tables); err != nil {
		return false, fmt.Errorf("store: account check: %w", err)
	}

	if tables == 0 {
		return false, nil
	}

	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM accounts`).Scan(&rows); err != nil {
		return false, fmt.Errorf("store: account check: %w", err)
	}

	return rows > 0, nil
}
