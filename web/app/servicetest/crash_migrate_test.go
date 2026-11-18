//go:build crashtest

package servicetest

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/crashpoint"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
)

// TestCrashMigrationChild is the body of the child of TestCrashMigration.
func TestCrashMigrationChild(t *testing.T) {
	if os.Getenv(roleEnv) != "migrate-child" {
		t.Skip("only runs as the child of a crash test")
	}

	st, err := store.Open(context.Background(), workspace.Dir(os.Getenv(workspace.EnvHome)),
		store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.MigrateStoredDocuments(
		context.Background(),
		st,
		service.MigrateOptions{NowMs: 1, Migrate: migrateFrom0},
	)

	t.Fatalf("the child finished (%v) without reaching %s", err, os.Getenv(crashpoint.CrashEnv))
}

// SDD-005 section 18 check 6 for the start-up migration: a kill before the transaction commits leaves
// the old document at the old revision and no backup; a kill after leaves the migrated document, its
// backup and the migrate log row.
func TestCrashMigration(t *testing.T) {
	t.Parallel()

	for _, point := range []string{"migrate.before_commit", "migrate.after_commit"} {
		t.Run(point, func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)

			raw, err := os.ReadFile(
				filepath.Join(
					"..",
					"..",
					"..",
					"core",
					"timeline",
					"testdata",
					"versions",
					"v1",
					"all-item-types.json",
				),
			)
			if err != nil {
				t.Fatal(err)
			}

			old := downgraded(raw)
			e.insertProject("prj_7k2m9x4q1b", old)

			if err := e.store.Close(); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(os.Args[0], "-test.run=^TestCrashMigrationChild$", "-test.v")
			cmd.Env = append(os.Environ(), roleEnv+"=migrate-child", workspace.EnvHome+"="+string(e.dir),
				crashpoint.CrashEnv+"="+point)

			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out

			var exit *exec.ExitError
			if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 137 {
				t.Fatalf("child = %v, want exit 137 at %s\n%s", err, point, out.String())
			}

			st, err := store.Open(
				context.Background(),
				e.dir,
				store.Options{SchemaVersion: timeline.CurrentSchemaVersion},
			)
			if err != nil {
				t.Fatalf("reopen after the crash: %v", err)
			}

			defer st.Close()

			var integrity string
			if err := st.DB().
				QueryRowContext(context.Background(), "PRAGMA integrity_check").
				Scan(&integrity); err != nil ||
				integrity != "ok" {
				t.Errorf("integrity_check = %q, %v", integrity, err)
			}

			row, err := st.Projects().Get(context.Background(), "prj_7k2m9x4q1b")
			if err != nil {
				t.Fatal(err)
			}

			backups, err := st.Backups().List(context.Background(), "project", "prj_7k2m9x4q1b")
			if err != nil {
				t.Fatal(err)
			}

			if point == "migrate.before_commit" {
				if row.Revision != 4 || row.SchemaVersion != stored || !bytes.Equal(row.Document, old) ||
					len(backups) != 0 {
					t.Errorf("after a crash before the commit: r%d v%d, %d backups; want the untouched r4 v%d",
						row.Revision, row.SchemaVersion, len(backups), stored)
				}

				return
			}

			var kept []byte

			if len(backups) == 1 {
				full, err := st.Backups().Get(context.Background(), backups[0].ID)
				if err != nil {
					t.Fatal(err)
				}

				kept = full.Document
			}

			if row.Revision != 5 || row.SchemaVersion != timeline.CurrentSchemaVersion || !bytes.Equal(kept, old) {
				t.Errorf("after a crash after the commit: r%d v%d, %d backups; want r5 migrated with its backup",
					row.Revision, row.SchemaVersion, len(backups))
			}
		})
	}
}
