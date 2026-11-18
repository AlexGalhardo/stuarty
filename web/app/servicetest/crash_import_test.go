//go:build crashtest

package servicetest

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	enginestore "github.com/AlexGalhardo/vukem-video/engine/store"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/crashpoint"
	"github.com/AlexGalhardo/vukem-video/service/pathguard"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
)

const bundleEnv = "VUKEM_VIDEO_CRASH_BUNDLE"

// TestCrashBundleImportChild is the body of the child of TestCrashBundleImport.
func TestCrashBundleImportChild(t *testing.T) {
	if os.Getenv(roleEnv) != "import-child" {
		t.Skip("only runs as the child of a crash test")
	}

	dir := workspace.Dir(os.Getenv(workspace.EnvHome))

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	guard, err := pathguard.New(pathguard.Config{Workspace: string(dir)})
	if err != nil {
		t.Fatal(err)
	}

	svc, err := service.New(service.Options{
		Store: st, Workspace: string(dir),
		Media: service.Media{
			Content: enginestore.New(string(dir)), Guard: guard,
			FreeBytes: func(string) (uint64, error) { return 1 << 40, nil },
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := service.WithCaller(context.Background(), service.Caller{Actor: ui, Scopes: service.AllScopes})
	_, err = svc.(bundleImporter).ProjectBundleImport(
		ctx,
		service.ProjectBundleImportRequest{Path: os.Getenv(bundleEnv)},
	)

	t.Fatalf("the child finished (%v) without reaching %s", err, os.Getenv(crashpoint.CrashEnv))
}

// SDD-005 section 18 check 6 for the bundle import: a kill after the copy and before the commit leaves
// no project (the copied files are orphans for the sweep); a kill after the commit leaves the whole
// project; the database is consistent either way.
func TestCrashBundleImport(t *testing.T) {
	t.Parallel()

	src := newMediaEnv(t, mediaCfg{})
	project, _ := src.bundleProject()
	bundle := src.exportBundle(project, nil).Result.Export.Path
	revision := src.head(project).Revision

	for _, point := range []string{"bundle_import.after_copy", "bundle_import.before_commit", "bundle_import.after_commit"} {
		t.Run(point, func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)
			if err := e.store.Close(); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(os.Args[0], "-test.run=^TestCrashBundleImportChild$", "-test.v")
			cmd.Env = append(os.Environ(), roleEnv+"=import-child", workspace.EnvHome+"="+string(e.dir),
				bundleEnv+"="+bundle, crashpoint.CrashEnv+"="+point)

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

			head, err := st.Projects().Head(context.Background(), project)
			if point != "bundle_import.after_commit" {
				if !errors.Is(err, service.ErrNotFound) {
					t.Errorf("crash at %s: the project exists (%v)", point, err)
				}

				if n, _ := st.Operations().Count(context.Background(), project, ""); n != 0 {
					t.Errorf("crash at %s left %d log rows", point, n)
				}

				return
			}

			if err != nil || head.Revision != revision {
				t.Fatalf("after the commit: %+v, %v; want revision %d", head, err, revision)
			}

			n, err := st.Operations().Count(context.Background(), project, "")
			if err != nil || n != revision {
				t.Errorf("log rows = %d, %v; want %d", n, err, revision)
			}

			active, err := st.Assets().ListProjectAssets(context.Background(), project, service.ProjectAssetActive)
			if err != nil || len(active) != 2 {
				t.Errorf("active project assets = %d, %v; want 2", len(active), err)
			}
		})
	}
}
