package store

import (
	"context"
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
