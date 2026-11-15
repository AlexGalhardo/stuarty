package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

// bundleService adds the bundle export to the fake, as the in-process service does.
type bundleService struct {
	fakeService

	req service.ProjectBundleExportRequest
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

func TestBundleExportNeedsAConnectionThatOffersIt(t *testing.T) {
	h := newHarness(t)

	if code := h.run("bundle:export", "--project", "prj_7k2m9x4q1b"); code == 0 {
		t.Fatalf("expected a failure: %s", h.stdout.String())
	}

	if !strings.Contains(h.stderr.String()+h.stdout.String(), "cannot export a bundle") {
		t.Fatalf("output: %s %s", h.stdout.String(), h.stderr.String())
	}
}

func (b *bundleService) JobGet(context.Context, service.JobGetRequest) (*service.Job, error) {
	return &service.Job{JobID: "job_9", Kind: service.JobExport, State: service.JobSucceeded, Text: "job_9 succeeded"}, nil
}
