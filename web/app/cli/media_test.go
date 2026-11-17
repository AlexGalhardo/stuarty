package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

// mediaFake answers the media and job methods from scripts.
type mediaFake struct {
	service.Service

	imports   []service.MediaImportRequest
	importAns []*service.MediaImportResult
	inspect   service.MediaInspectRequest
	listReq   service.MediaListRequest
	jobStart  service.JobStartRequest
	startAns  *service.Job
	gets      []service.JobGetRequest
	getAns    []*service.Job
	jobsReq   service.JobListRequest
	cancelled []service.JobCancelRequest
}

func (f *mediaFake) MediaImport(_ context.Context, r service.MediaImportRequest) (*service.MediaImportResult, error) {
	f.imports = append(f.imports, r)

	i := min(len(f.imports)-1, len(f.importAns)-1)
	ans := *f.importAns[i]
	ans.Assets = append([]service.ImportedAsset(nil), ans.Assets...)

	return &ans, nil
}

func (f *mediaFake) MediaInspect(
	_ context.Context,
	r service.MediaInspectRequest,
) (*service.MediaInspectResult, error) {
	f.inspect = r

	return &service.MediaInspectResult{AssetID: r.AssetID, Text: "inspected"}, nil
}

func (f *mediaFake) MediaList(_ context.Context, r service.MediaListRequest) (*service.MediaListResult, error) {
	f.listReq = r

	return &service.MediaListResult{ProjectID: r.ProjectID, Text: "listed"}, nil
}

func (f *mediaFake) JobStart(_ context.Context, r service.JobStartRequest) (*service.Job, error) {
	f.jobStart = r

	return f.startAns, nil
}

func (f *mediaFake) JobGet(_ context.Context, r service.JobGetRequest) (*service.Job, error) {
	f.gets = append(f.gets, r)

	i := min(len(f.gets)-1, len(f.getAns)-1)

	return f.getAns[i], nil
}

func (f *mediaFake) JobCancel(_ context.Context, r service.JobCancelRequest) (*service.Job, error) {
	f.cancelled = append(f.cancelled, r)

	return job(r.JobID, service.JobCancelled, r.JobID+" export cancelled 62%"), nil
}

func (f *mediaFake) JobList(_ context.Context, r service.JobListRequest) (*service.JobListResult, error) {
	f.jobsReq = r

	return &service.JobListResult{ProjectID: r.ProjectID, Text: "jobs"}, nil
}

func mediaHarness(t *testing.T, f *mediaFake) *harness {
	t.Helper()

	h := newHarness(t)
	h.res.svc = f

	return h
}

func job(id, state, text string) *service.Job {
	return &service.Job{JobID: id, Kind: "export", State: state, Text: text}
}

const projectFlag = "--project=prj_7k2m9x4q1b"

func TestMediaImportMakesPathsAbsoluteAndWaitsForTheAssets(t *testing.T) {
	t.Parallel()

	f := &mediaFake{importAns: []*service.MediaImportResult{
		{Assets: []service.ImportedAsset{{AssetID: "ast_1", Name: "a.mov", State: "ingesting"}}, Text: "ingesting"},
		{Assets: []service.ImportedAsset{{AssetID: "ast_1", Name: "a.mov", State: "ready", Deduplicated: true}}},
	}}
	h := mediaHarness(t, f)

	if code := h.run("media:import", "clip.mov", "--asset-id", "ast_1", projectFlag, "--json"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stdout.String())
	}

	if len(f.imports) != 2 {
		t.Fatalf("the wait loop made %d calls, want 2", len(f.imports))
	}

	got := f.imports[0]
	if !filepath.IsAbs(got.Files[0].Path) || filepath.Base(got.Files[0].Path) != "clip.mov" ||
		got.Files[0].AssetID != "ast_1" {
		t.Errorf("files = %+v", got.Files)
	}

	if got.WaitMS <= 0 || got.WaitMS > 25000 {
		t.Errorf("wait_ms = %d, want 1..25000", got.WaitMS)
	}

	doc := oneJSONDocument(t, h.stdout.String())

	assets, _ := doc["assets"].([]any)
	a, _ := assets[0].(map[string]any)

	if a["state"] != "ready" || a["deduplicated"] != false {
		t.Errorf("the final answer must be ready and read as the first call: %v", a)
	}
}

// The wait loop throws away the text of the first answer and draws the last one.
// A file that fails while the command is waiting must keep its hint in that text.
func TestMediaImportWaitKeepsTheErrorHint(t *testing.T) {
	t.Parallel()

	f := &mediaFake{importAns: []*service.MediaImportResult{
		{Assets: []service.ImportedAsset{{AssetID: "ast_1", Name: "a.mov", State: "ingesting"}}, Text: "ingesting"},
		{
			Assets: []service.ImportedAsset{{
				AssetID: "ast_1", Name: "a.mov", State: "failed",
				Error: &service.Error{
					Code:    "E_UNSUPPORTED_MEDIA",
					Message: "The file is not a media file FFmpeg can read.",
					Hint:    "Import a video, audio or image file.",
				},
			}},
			Failed: 1,
		},
	}}
	h := mediaHarness(t, f)

	if code := h.run("media:import", "a.mov", projectFlag); code != ExitFailure {
		t.Fatalf("exit %d: %s", code, h.stdout.String())
	}

	out := h.stdout.String()
	if !strings.Contains(out, "E_UNSUPPORTED_MEDIA") ||
		!strings.Contains(out, "hint: Import a video, audio or image file.") {
		t.Fatalf("the redrawn text dropped the hint:\n%s", out)
	}
}

func TestMediaImportNoWaitCallsOnceAndExitCodes(t *testing.T) {
	t.Parallel()

	f := &mediaFake{importAns: []*service.MediaImportResult{
		{Assets: []service.ImportedAsset{{State: "ingesting"}}, Text: "ingesting"},
	}}
	h := mediaHarness(t, f)

	if code := h.run("media:import", "a.mov", projectFlag, "--no-wait"); code != ExitOK {
		t.Errorf("--no-wait exit %d", code)
	}

	if len(f.imports) != 1 || f.imports[0].WaitMS != 0 {
		t.Errorf("calls = %+v", f.imports)
	}

	f = &mediaFake{importAns: []*service.MediaImportResult{
		{
			Assets: []service.ImportedAsset{{State: "ready"}, {Error: &service.Error{Code: "E_FILE_NOT_FOUND"}}},
			Failed: 1,
		},
	}}
	h = mediaHarness(t, f)

	if code := h.run("media:import", "a.mov", "b.mov", projectFlag); code != ExitFailure {
		t.Errorf("a failed file: exit %d, want 1", code)
	}

	f = &mediaFake{importAns: []*service.MediaImportResult{
		{Assets: []service.ImportedAsset{{State: "ingesting"}}, Text: "ingesting"},
	}}
	h = mediaHarness(t, f)

	if code := h.run("media:import", "a.mov", projectFlag, "--timeout", "1ms"); code != ExitTransient {
		t.Errorf("a timeout: exit %d, want 2", code)
	}

	if !strings.Contains(h.stdout.String(), "still running") {
		t.Errorf("stdout = %q", h.stdout.String())
	}

	if code := h.run("media:import", projectFlag); code != ExitFailure {
		t.Errorf("no path: exit %d", code)
	}

	if code := h.run("media:import", "a.mov", "b.mov", "--asset-id", "ast_1", projectFlag); code != ExitFailure {
		t.Errorf("one id for two paths: exit %d", code)
	}
}

func TestMediaInspectAndListPassTheirFlags(t *testing.T) {
	t.Parallel()

	f := &mediaFake{}
	h := mediaHarness(t, f)

	if code := h.run(
		"media:inspect",
		"ast_1",
		projectFlag,
		"--sections",
		"probe, silences",
		"--limit",
		"5",
	); code != ExitOK {
		t.Fatalf("exit %d", code)
	}

	if f.inspect.AssetID != "ast_1" || strings.Join(f.inspect.Sections, ",") != "probe,silences" ||
		f.inspect.Limit != 5 {
		t.Errorf("request = %+v", f.inspect)
	}

	if code := h.run("media:inspect", projectFlag); code != ExitFailure {
		t.Errorf("no asset id: exit %d", code)
	}

	if code := h.run(
		"media:list",
		projectFlag,
		"--cursor",
		"c1",
	); code != ExitOK || f.listReq.Cursor != "c1" ||
		f.listReq.Limit != 50 {
		t.Errorf("media:list exit %d, request %+v", code, f.listReq)
	}
}

func TestExportStartsWaitsAndReportsTheOutcome(t *testing.T) {
	t.Parallel()

	f := &mediaFake{
		startAns: job("job_1", "running", "job_1 export running 10%\nstill running: call job_get again"),
		getAns:   []*service.Job{job("job_1", "succeeded", "job_1 export succeeded 100% file=/x.mp4")},
	}
	h := mediaHarness(t, f)

	if code := h.run("export", projectFlag, "--start", "1s", "--end", "3s"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	var args service.ExportArgs
	if err := json.Unmarshal(f.jobStart.Args, &args); err != nil || args.Start != "1s" || args.End != "3s" {
		t.Errorf("args = %s", f.jobStart.Args)
	}

	if f.jobStart.Kind != "export" || f.jobStart.WaitMS <= 0 {
		t.Errorf("request = %+v", f.jobStart)
	}

	if !strings.Contains(h.stdout.String(), "succeeded") {
		t.Errorf("stdout = %q", h.stdout.String())
	}

	f = &mediaFake{startAns: job("job_2", "failed", "job_2 export failed 0%")}
	h = mediaHarness(t, f)

	if code := h.run("export", projectFlag); code != ExitFailure || f.jobStart.Args != nil {
		t.Errorf("a failed export: exit %d, args %s", code, f.jobStart.Args)
	}

	f = &mediaFake{
		startAns: job("job_3", "running", "job_3 export running 5%"),
		getAns:   []*service.Job{job("job_3", "running", "job_3 export running 5%\nstill running: call job_get again")},
	}
	h = mediaHarness(t, f)

	if code := h.run("export", projectFlag, "--timeout", "1ms"); code != ExitTransient {
		t.Errorf("a timeout: exit %d, want 2", code)
	}

	want := "job_3 export running 5%\nstill running: run vukem-video job:show job_3 --wait\n"
	if h.stdout.String() != want {
		t.Errorf("stdout = %q, want %q", h.stdout.String(), want)
	}

	if code := h.run("export", projectFlag, "--no-wait"); code != ExitOK || f.jobStart.WaitMS != 0 {
		t.Errorf("--no-wait: exit %d, wait_ms %d", code, f.jobStart.WaitMS)
	}
}

func TestJobCancelCallsTheServiceAndExitsZero(t *testing.T) {
	t.Parallel()

	f := &mediaFake{}
	h := mediaHarness(t, f)

	code := h.run("job:cancel", "job_1")
	if code != ExitOK || len(f.cancelled) != 1 || f.cancelled[0].JobID != "job_1" {
		t.Errorf("job:cancel: exit %d, calls %+v", code, f.cancelled)
	}

	if !strings.Contains(h.stdout.String(), "export cancelled 62%") {
		t.Errorf("text = %q", h.stdout.String())
	}

	if code := h.run("job:cancel"); code != ExitFailure {
		t.Errorf("no job id: exit %d", code)
	}
}

func TestJobShowWaitsOnlyWhenAsked(t *testing.T) {
	t.Parallel()

	f := &mediaFake{getAns: []*service.Job{job("job_1", "running", "job_1 export running 40%")}}
	h := mediaHarness(t, f)

	if code := h.run("job:show", "job_1"); code != ExitOK || f.gets[0].WaitMS != 0 || len(f.gets) != 1 {
		t.Errorf("without --wait: exit %d, calls %+v", code, f.gets)
	}

	if code := h.run("job:show", "job_1", "--wait", "--timeout", "1ms"); code != ExitTransient {
		t.Errorf("--wait that ran out: exit %d, want 2", code)
	}

	if code := h.run("job:show"); code != ExitFailure {
		t.Errorf("no job id: exit %d", code)
	}

	if code := h.run("job:list", projectFlag, "--state", "active"); code != ExitOK || f.jobsReq.State != "active" {
		t.Errorf("job:list exit %d, request %+v", code, f.jobsReq)
	}
}

func TestExportSendsTheFormat(t *testing.T) {
	t.Parallel()

	f := &mediaFake{startAns: job("job_1", "queued", "job_1 export queued")}
	h := mediaHarness(t, f)

	if code := h.run("export", projectFlag, "--format", "gif", "--no-wait"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	var args service.ExportArgs
	if err := json.Unmarshal(f.jobStart.Args, &args); err != nil || args.Format != "gif" {
		t.Errorf("args = %s", f.jobStart.Args)
	}
}

func TestExportSendsTheFrameTimeOfAPNG(t *testing.T) {
	t.Parallel()

	f := &mediaFake{startAns: job("job_1", "queued", "job_1 export queued")}
	h := mediaHarness(t, f)

	if code := h.run("export", projectFlag, "--format", "png", "--at", "2.5s", "--no-wait"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	var args service.ExportArgs
	if err := json.Unmarshal(f.jobStart.Args, &args); err != nil || args.Format != "png" || args.At != "2.5s" {
		t.Errorf("args = %s", f.jobStart.Args)
	}
}

// With VUKEM_VIDEO_URL the path belongs to the server and goes as typed (VV-005): a
// POSIX path is not rewritten against the Windows working directory.
func TestAbsKeepsServerPathsWhenRemote(t *testing.T) {
	h := newHarness(t)
	h.rt.Getenv = func(k string) string {
		if k == workspace.EnvURL {
			return "http://server:7878"
		}

		return ""
	}

	got, err := h.rt.abs("/assets/video/a.mp4")
	if err != nil || got != "/assets/video/a.mp4" {
		t.Errorf("abs = %q, %v", got, err)
	}
}
