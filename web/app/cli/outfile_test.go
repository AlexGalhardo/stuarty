package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// outService answers an export with a job that has already succeeded.
type outService struct {
	fakeService

	state  string
	result *service.ExportResult
	file   *service.FileResult
}

func (o *outService) job() *service.Job {
	j := &service.Job{JobID: "job_9", Kind: service.JobExport, State: o.state, Text: "job_9 " + o.state}
	if o.state == service.JobSucceeded {
		j.Result = &service.JobResult{Export: o.result}
	}

	return j
}

func (o *outService) ProjectBundleExport(context.Context, service.ProjectBundleExportRequest) (*service.Job, error) {
	return o.job(), nil
}

func (o *outService) JobStart(context.Context, service.JobStartRequest) (*service.Job, error) {
	return o.job(), nil
}

func (o *outService) JobGet(context.Context, service.JobGetRequest) (*service.Job, error) {
	return o.job(), nil
}

func (o *outService) TemplateExport(context.Context, service.TemplateRef) (*service.FileResult, error) {
	return o.file, nil
}

func writeSource(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "bundle-1.vvbundle")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(raw)
}

func TestBundleExportOutCopiesTheLocalFile(t *testing.T) {
	h := newHarness(t)
	src := writeSource(t, "bundle bytes")
	h.res.svc = &outService{state: service.JobSucceeded, result: &service.ExportResult{Path: src}}

	dest := filepath.Join(t.TempDir(), "sub", "out.vvbundle")

	if code := h.run("bundle:export", "--project", "prj_7k2m9x4q1b", "--out", dest, "--json"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if got := readFile(t, dest); got != "bundle bytes" {
		t.Fatalf("copy = %q", got)
	}

	if _, err := os.Stat(src); err != nil {
		t.Fatalf("the workspace copy must stay: %v", err)
	}

	var doc struct {
		JobID   string `json:"job_id"`
		SavedTo string `json:"saved_to"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &doc); err != nil || doc.JobID != "job_9" || doc.SavedTo != dest {
		t.Fatalf("json = %s (%v)", h.stdout.String(), err)
	}
}

func TestBundleExportOutNamesTheFinalPathInText(t *testing.T) {
	h := newHarness(t)
	h.res.svc = &outService{state: service.JobSucceeded, result: &service.ExportResult{Path: writeSource(t, "x")}}

	dest := filepath.Join(t.TempDir(), "out.vvbundle")

	if code := h.run("bundle:export", "--project", "prj_7k2m9x4q1b", "--out", dest); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if !strings.Contains(h.stdout.String(), "saved to "+dest) {
		t.Fatalf("output: %s", h.stdout.String())
	}
}

func TestOutRefusesToOverwriteUnlessForced(t *testing.T) {
	h := newHarness(t)
	h.res.svc = &outService{state: service.JobSucceeded, result: &service.ExportResult{Path: writeSource(t, "new")}}

	dest := filepath.Join(t.TempDir(), "out.vvbundle")
	if err := os.WriteFile(dest, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if code := h.run("bundle:export", "--project", "prj_7k2m9x4q1b", "--out", dest); code != ExitFailure {
		t.Fatalf("exit %d, want %d", code, ExitFailure)
	}

	if !strings.Contains(h.stderr.String(), "--force") || !strings.Contains(h.stderr.String(), dest) {
		t.Fatalf("stderr: %s", h.stderr.String())
	}

	if got := readFile(t, dest); got != "old" {
		t.Fatalf("the file was replaced: %q", got)
	}

	if code := h.run("bundle:export", "--project", "prj_7k2m9x4q1b", "--out", dest, "--force"); code != 0 {
		t.Fatalf("forced: exit %d: %s", code, h.stderr.String())
	}

	if got := readFile(t, dest); got != "new" {
		t.Fatalf("forced copy = %q", got)
	}
}

func TestOutWritesNothingWhenTheJobFails(t *testing.T) {
	h := newHarness(t)
	h.res.svc = &outService{state: service.JobFailed}

	dest := filepath.Join(t.TempDir(), "out.vvbundle")

	if code := h.run("export", "--project", "prj_7k2m9x4q1b", "--out", dest); code != ExitFailure {
		t.Fatalf("exit %d, want %d", code, ExitFailure)
	}

	if _, err := os.Stat(dest); err == nil {
		t.Fatal("a failed job must leave no file")
	}
}

func TestOutDownloadsWhenTheServerIsRemote(t *testing.T) {
	h := newHarness(t)
	h.res.svc = &outService{
		state: service.JobSucceeded, result: &service.ExportResult{DownloadURL: "/api/v1/jobs/job_9/file"},
	}

	var asked string

	h.res.download = func(_ context.Context, urlPath string) (io.ReadCloser, error) {
		asked = urlPath

		return io.NopCloser(strings.NewReader("remote bytes")), nil
	}

	dest := filepath.Join(t.TempDir(), "out.mp4")

	if code := h.run("export", "--project", "prj_7k2m9x4q1b", "--out", dest); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if asked != "/api/v1/jobs/job_9/file" || readFile(t, dest) != "remote bytes" {
		t.Fatalf("asked %q, copy %q", asked, readFile(t, dest))
	}
}

func TestTemplateExportOutCopiesTheFile(t *testing.T) {
	h := newHarness(t)
	h.res.svc = &outService{file: &service.FileResult{Path: writeSource(t, "zip"), Text: "template file"}}

	dest := filepath.Join(t.TempDir(), "spring.zip")

	if code := h.run("template:export", "tpl_1", "--out", dest); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if readFile(t, dest) != "zip" || !strings.Contains(h.stdout.String(), "saved to "+dest) {
		t.Fatalf("copy %q, output %s", readFile(t, dest), h.stdout.String())
	}

	if code := h.run("template:export", "tpl_1", "--out", dest); code != ExitFailure {
		t.Fatalf("second export over the same file: exit %d", code)
	}
}

func TestOutExpandsTheHomeDirectory(t *testing.T) {
	h := newHarness(t)
	home := t.TempDir()
	h.rt.Getenv = func(k string) string {
		if k == "HOME" {
			return home
		}

		return ""
	}
	h.res.svc = &outService{file: &service.FileResult{Path: writeSource(t, "zip")}}

	if code := h.run("template:export", "tpl_1", "--out", "~/spring.zip"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if readFile(t, filepath.Join(home, "spring.zip")) != "zip" {
		t.Fatal("the copy is not in the home directory")
	}
}

func TestHTTPDownloadSendsTheTokenOnlyToItsOwnServer(t *testing.T) {
	for _, p := range []string{"@evil.example/x", "//evil.example/x", "https://evil.example/x", `/\evil.example/x`, "jobs/x"} {
		if sameServerPath(p) {
			t.Errorf("sameServerPath(%q) = true, want false", p)
		}
	}

	if !sameServerPath("/api/v1/jobs/job_0123456789/file") {
		t.Error("a plain API path must be accepted")
	}
}
