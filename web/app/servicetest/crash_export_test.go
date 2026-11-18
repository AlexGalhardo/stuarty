//go:build crashtest

package servicetest

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	enginestore "github.com/AlexGalhardo/vukem-video/engine/store"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/crashpoint"
	"github.com/AlexGalhardo/vukem-video/service/pathguard"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/store"
)

const exportJobEnv = "VUKEM_VIDEO_CRASH_EXPORT_JOB"

// TestCrashBundleExportChild is the body of the child of TestCrashBundleExport: it runs the queued job.
func TestCrashBundleExportChild(t *testing.T) {
	if os.Getenv(roleEnv) != "export-child" {
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

	runner, ok := service.RunnerOf(svc)
	if !ok {
		t.Fatal("the service has no job runner")
	}

	err = runner.RunJob(context.Background(), os.Getenv(exportJobEnv))

	t.Fatalf("the child finished (%v) without reaching %s", err, os.Getenv(crashpoint.CrashEnv))
}

// SDD-005 section 18 check 6 for the bundle export: a kill after the archive is written and before the
// rename leaves no bundle at its final name (a half-written one is never offered); a kill after the
// rename leaves a whole, readable bundle; the project and the database are untouched either way
// (the export only reads).
func TestCrashBundleExport(t *testing.T) {
	t.Parallel()

	for _, point := range []string{"bundle_export.after_write", "bundle_export.after_rename"} {
		t.Run(point, func(t *testing.T) {
			t.Parallel()

			m := newMediaEnv(t, mediaCfg{})
			project, _ := m.bundleProject()
			revision := m.head(project).Revision

			m.hold.Store(true) // the job stays queued: the child runs it

			job, err := m.svc.(bundleExporter).ProjectBundleExport(
				m.as(ui),
				service.ProjectBundleExportRequest{ProjectID: project},
			)
			if err != nil {
				t.Fatal(err)
			}

			if err := m.store.Close(); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(os.Args[0], "-test.run=^TestCrashBundleExportChild$", "-test.v")
			cmd.Env = append(os.Environ(), roleEnv+"=export-child", workspace.EnvHome+"="+m.ws,
				exportJobEnv+"="+job.JobID, crashpoint.CrashEnv+"="+point)

			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out

			var exit *exec.ExitError
			if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 137 {
				t.Fatalf("child = %v, want exit 137 at %s\n%s", err, point, out.String())
			}

			st, err := store.Open(
				context.Background(),
				m.dir,
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
			if err != nil || head.Revision != revision {
				t.Errorf("project head = %+v, %v; want revision %d untouched", head, err, revision)
			}

			bundles, _ := filepath.Glob(filepath.Join(m.ws, "exports", project, "*.vvbundle"))

			if point == "bundle_export.after_write" {
				if len(bundles) != 0 {
					t.Errorf("crash before the rename left %v at a final name", bundles)
				}

				temps, _ := filepath.Glob(filepath.Join(m.ws, "exports", project, "bundle-r*.vvbundle.tmp"))
				if len(temps) != 1 {
					t.Fatalf("crash before the rename left %v, want one temp file", temps)
				}

				// A service made after the crash is the next start: its sweep removes the leftover.
				next, err := service.New(service.Options{Store: st, Workspace: m.ws})
				if err != nil {
					t.Fatal(err)
				}

				sweeper, _ := service.SweeperOf(next)
				if _, err := sweeper.Sweep(context.Background()); err != nil {
					t.Fatal(err)
				}

				if _, err := os.Stat(temps[0]); err == nil {
					t.Error("the sweep of the next start left the temp file")
				}

				return
			}

			if len(bundles) != 1 {
				t.Fatalf("crash after the rename left %v, want one bundle", bundles)
			}

			zr, err := zip.OpenReader(bundles[0])
			if err != nil {
				t.Fatalf("the bundle is not a whole archive: %v", err)
			}

			_ = zr.Close()
		})
	}
}
