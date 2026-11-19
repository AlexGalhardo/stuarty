package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// bundleService adds the bundle export to the fake, as the in-process service does.
type bundleService struct {
	fakeService

	req service.ProjectBundleExportRequest
	imp service.ProjectBundleImportRequest
}

func (b *bundleService) ProjectBundleExport(
	ctx context.Context,
	r service.ProjectBundleExportRequest,
) (*service.Job, error) {
	b.note(ctx)
	b.req = r

	return &service.Job{JobID: "job_9", Kind: service.JobExport, State: service.JobQueued, Text: "job_9 queued"}, b.err
}

func TestBundleExportStartsTheJob(t *testing.T) {
	h := newHarness(t)
	svc := &bundleService{}
	h.res.svc = svc

	if code := h.run("bundle:export", "--project", "prj_7k2m9x4q1b", "--no-log"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if svc.req.ProjectID != "prj_7k2m9x4q1b" || svc.req.IncludeLog == nil || *svc.req.IncludeLog {
		t.Fatalf("request: %+v", svc.req)
	}

	if !strings.Contains(h.stdout.String(), "job_9") {
		t.Fatalf("output: %s", h.stdout.String())
	}
}

func TestBundleExportKeepsTheLogByDefault(t *testing.T) {
	h := newHarness(t)
	svc := &bundleService{}
	h.res.svc = svc

	if code := h.run("bundle:export", "--project", "prj_7k2m9x4q1b"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if svc.req.IncludeLog == nil || !*svc.req.IncludeLog {
		t.Fatalf("request: %+v", svc.req)
	}
}

func (b *bundleService) JobGet(context.Context, service.JobGetRequest) (*service.Job, error) {
	return &service.Job{
		JobID: "job_9", Kind: service.JobExport, State: service.JobSucceeded, Text: "job_9 succeeded",
	}, nil
}

func (b *bundleService) ProjectBundleImport(
	ctx context.Context,
	r service.ProjectBundleImportRequest,
) (*service.ProjectCreateResult, error) {
	b.note(ctx)
	b.imp = r

	return &service.ProjectCreateResult{ProjectID: "prj_9", Text: "prj_9 created"}, b.err
}

func TestBundleImportSendsAnAbsolutePathAndTheFlags(t *testing.T) {
	h := newHarness(t)
	svc := &bundleService{}
	h.res.svc = svc

	if code := h.run("bundle:import", "teaser.vvbundle", "--as-new", "--dry-run"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if !filepath.IsAbs(svc.imp.Path) || filepath.Base(svc.imp.Path) != "teaser.vvbundle" ||
		!svc.imp.AsNew || !svc.imp.DryRun {
		t.Fatalf("request: %+v", svc.imp)
	}

	if !strings.Contains(h.stdout.String(), "prj_9") {
		t.Fatalf("output: %s", h.stdout.String())
	}
}

func TestBundleImportKeepsTheIdByDefault(t *testing.T) {
	h := newHarness(t)
	svc := &bundleService{}
	h.res.svc = svc

	if code := h.run("bundle:import", "teaser.vvbundle"); code != 0 || svc.imp.AsNew || svc.imp.DryRun {
		t.Fatalf("exit %d: %+v", code, svc.imp)
	}

	for _, args := range [][]string{{"bundle:import"}, {"bundle:import", "a", "b"}} {
		if code := h.run(args...); code != ExitFailure {
			t.Errorf("%v: exit %d, want 1", args, code)
		}
	}
}
