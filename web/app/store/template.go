package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/AlexGalhardo/stuarty/service/workspace"
)

var (
	templateOnce  sync.Once
	templateBytes []byte
	errTemplate   error
)

func loadTemplateDB() ([]byte, error) {
	templateOnce.Do(func() {
		tmp, err := os.MkdirTemp("", "stuarty-store-template-*")
		if err != nil {
			errTemplate = fmt.Errorf("store: create template dir: %w", err)
			return
		}
		defer func() { _ = os.RemoveAll(tmp) }()

		dir, err := workspace.Resolve(func(string) string { return filepath.Join(tmp, "ws") })
		if err != nil {
			errTemplate = err
			return
		}
		if err := dir.EnsureLayout(); err != nil {
			errTemplate = err
			return
		}

		st, err := Open(context.Background(), dir, Options{})
		if err != nil {
			errTemplate = err
			return
		}
		// Flush all WAL frames into the main db.sqlite file so copying a single file
		// reproduces the fully migrated schema cleanly.
		if _, err := st.DB().ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			_ = st.Close()
			errTemplate = fmt.Errorf("store: checkpoint template db: %w", err)
			return
		}
		if err := st.Close(); err != nil {
			errTemplate = err
			return
		}

		data, err := os.ReadFile(dir.Path("db.sqlite"))
		if err != nil {
			errTemplate = fmt.Errorf("store: read template db: %w", err)
			return
		}
		templateBytes = data
	})

	return templateBytes, errTemplate
}

// OpenTemplate seeds an empty workspace with a process-cached, pre-migrated SQLite database before
// calling Open. Intended for test harnesses (servicetest, contract/testenv) so thousands of tests do
// not replay every migration from scratch while Open still verifies pragmas, migration status,
// schema versions, and sealed-column guards.
func OpenTemplate(ctx context.Context, dir workspace.Dir, o Options) (*Store, error) {
	if o.Migrations != nil {
		return Open(ctx, dir, o)
	}
	dbPath := dir.Path("db.sqlite")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		data, err := loadTemplateDB()
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(dbPath, data, 0o600); err != nil {
			return nil, fmt.Errorf("store: write template db: %w", err)
		}
	}
	return Open(ctx, dir, o)
}
