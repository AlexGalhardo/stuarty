package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

// otioService adds the OpenTimelineIO operations to the fake.
type otioService struct {
	fakeService

	imp service.ProjectOtioImportRequest
}

func (o *otioService) ProjectOtioExport(
	ctx context.Context,
	r service.ProjectOtioExportRequest,
) (*service.ProjectOtioExportResult, error) {
	o.note(ctx)

	return &service.ProjectOtioExportResult{
		ProjectID: r.ProjectID, Revision: 4, FileName: "cut.otio", Content: `{"OTIO_SCHEMA":"Timeline.1"}`,
		Text: "nothing lost",
	}, o.err
}

func (o *otioService) ProjectOtioImport(
	ctx context.Context,
	r service.ProjectOtioImportRequest,
) (*service.ProjectOtioImportResult, error) {
	o.note(ctx)
	o.imp = r

	return &service.ProjectOtioImportResult{ProjectID: r.ProjectID, Revision: 5, Applied: !r.DryRun, Text: "imported"}, o.err
}

func TestOtioExportWritesTheFileAndRefusesToReplaceIt(t *testing.T) {
	h := newHarness(t)
	h.res.svc = &otioService{}
	out := filepath.Join(t.TempDir(), "sub", "cut.otio")

	if code := h.run("otio:export", "--project", "prj_7k2m9x4q1b", "--out", out); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	got, err := os.ReadFile(out)
	if err != nil || string(got) != `{"OTIO_SCHEMA":"Timeline.1"}` {
		t.Fatalf("file %q, err %v", got, err)
	}

	if strings.Contains(h.stdout.String(), "OTIO_SCHEMA") {
		t.Errorf("the output carries the file: %s", h.stdout.String())
	}

	if code := h.run("otio:export", "--project", "prj_7k2m9x4q1b", "--out", out); code != ExitFailure {
		t.Errorf("second export without --force: exit %d, want 1", code)
	}

	if code := h.run("otio:export", "--project", "prj_7k2m9x4q1b", "--out", out, "--force"); code != ExitOK {
		t.Errorf("with --force: exit %d: %s", code, h.stderr.String())
	}
}

func TestOtioExportNamesTheFileAfterTheProjectByDefault(t *testing.T) {
	h := newHarness(t)
	h.res.svc = &otioService{}
	dir := t.TempDir()
	h.rt.Getwd = func() (string, error) { return dir, nil }

	if code := h.run("otio:export", "--project", "prj_7k2m9x4q1b"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if _, err := os.Stat(filepath.Join(dir, "cut.otio")); err != nil {
		t.Fatal(err)
	}
}

func TestOtioImportSendsAnAbsolutePathAndTheBaseRevision(t *testing.T) {
	h := newHarness(t)
	svc := &otioService{}
	h.res.svc = svc

	if code := h.run(
		"otio:import", "cut.otio", "--project", "prj_7k2m9x4q1b", "--base-revision", "3", "--dry-run",
	); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if !filepath.IsAbs(svc.imp.Path) || filepath.Base(svc.imp.Path) != "cut.otio" ||
		svc.imp.BaseRevision != 3 || !svc.imp.DryRun || svc.imp.ProjectID != "prj_7k2m9x4q1b" {
		t.Fatalf("request: %+v", svc.imp)
	}
}

func TestOtioImportDefaultsToTheCurrentRevision(t *testing.T) {
	h := newHarness(t)
	svc := &otioService{fakeService: fakeService{revision: 7}}
	h.res.svc = svc

	if code := h.run("otio:import", "cut.otio", "--project", "prj_7k2m9x4q1b"); code != ExitOK || svc.imp.BaseRevision != 7 {
		t.Fatalf("exit %d: %+v", code, svc.imp)
	}

	for _, args := range [][]string{{"otio:import", "--project", "prj_7k2m9x4q1b"}, {"otio:import", "a", "b", "--project", "prj_7k2m9x4q1b"}} {
		if code := h.run(args...); code != ExitFailure {
			t.Errorf("%v: exit %d, want 1", args, code)
		}
	}
}
