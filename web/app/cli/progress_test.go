package cli

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/service"
)

// inlineFake behaves like the in-process Service: JobStart and MediaImport return
// only when the work ends, and only a cancel (or the context) ends it early.
type inlineFake struct {
	service.Service

	mu        sync.Mutex
	started   bool
	created   time.Time
	cancelled []string
	stop      chan struct{}
}

func newInlineFake() *inlineFake { return &inlineFake{stop: make(chan struct{})} }

func (f *inlineFake) JobStart(_ context.Context, _ service.JobStartRequest) (*service.Job, error) {
	f.mu.Lock()
	f.started, f.created = true, time.Now()
	f.mu.Unlock()

	<-f.stop

	return &service.Job{
		JobID: "job_1", Kind: "export", State: service.JobCancelled, Text: "job_1 export cancelled 3%",
	}, nil
}

func (f *inlineFake) JobList(_ context.Context, _ service.JobListRequest) (*service.JobListResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.started {
		return &service.JobListResult{}, nil
	}

	return &service.JobListResult{Jobs: []service.Job{{
		JobID: "job_1", Kind: "export", State: service.JobRunning, Progress: 0.4, CreatedAt: f.created,
	}}}, nil
}

func (f *inlineFake) JobCancel(_ context.Context, r service.JobCancelRequest) (*service.Job, error) {
	f.mu.Lock()
	f.cancelled = append(f.cancelled, r.JobID)
	f.mu.Unlock()

	close(f.stop)

	return &service.Job{JobID: r.JobID, State: service.JobCancelled}, nil
}

func (f *inlineFake) MediaImport(
	ctx context.Context,
	_ service.MediaImportRequest,
) (*service.MediaImportResult, error) {
	<-ctx.Done()

	return nil, ctx.Err()
}

func (f *inlineFake) MediaList(_ context.Context, r service.MediaListRequest) (*service.MediaListResult, error) {
	return &service.MediaListResult{ProjectID: r.ProjectID}, nil
}

func inlineHarness(t *testing.T, f service.Service) *harness {
	t.Helper()

	h := newHarness(t)
	h.res.svc = f
	h.res.inProcess = true

	return h
}

func TestInProcessTimeoutStopsTheJobAndExitsTwo(t *testing.T) {
	t.Parallel()

	f := newInlineFake()
	h := inlineHarness(t, f)

	if code := h.run("export", projectFlag, "--timeout", "100ms"); code != ExitTransient {
		t.Fatalf("exit %d, want 2: %s", code, h.stdout.String())
	}

	if len(f.cancelled) != 1 || f.cancelled[0] != "job_1" {
		t.Errorf("cancelled = %v, want job_1", f.cancelled)
	}

	if out := h.stdout.String(); !strings.Contains(out, "timed out after 100ms") || !strings.Contains(out, "serve") {
		t.Errorf("stdout = %q", out)
	}
}

func TestInProcessTimeoutKeepsTheJSONShapeOfAJob(t *testing.T) {
	t.Parallel()

	h := inlineHarness(t, newInlineFake())

	if code := h.run("export", projectFlag, "--timeout", "100ms", "--json"); code != ExitTransient {
		t.Fatalf("exit %d, want 2", code)
	}

	doc := oneJSONDocument(t, h.stdout.String())
	if doc["job_id"] != "job_1" || doc["state"] != service.JobCancelled {
		t.Errorf("doc = %v", doc)
	}

	if h.stderr.Len() != 0 {
		t.Errorf("--json wrote to stderr: %q", h.stderr.String())
	}
}

func TestInProcessNoWaitIsRefusedAndTellsToServe(t *testing.T) {
	t.Parallel()

	for _, argv := range [][]string{
		{"export", projectFlag, "--no-wait"},
		{"transcribe", projectFlag, "--asset", "ast_1", "--no-wait"},
		{"media:import", "a.mov", projectFlag, "--no-wait"},
		{"bundle:export", projectFlag, "--no-wait"},
	} {
		f := newInlineFake()
		h := inlineHarness(t, f)

		if code := h.run(argv...); code != ExitFailure {
			t.Errorf("%v: exit %d, want 1", argv, code)
		}

		if !strings.Contains(h.stderr.String(), "vukem-video serve") || f.started {
			t.Errorf("%v: stderr %q, started %v", argv, h.stderr.String(), f.started)
		}
	}
}

func TestInProcessImportTimeoutExitsTwo(t *testing.T) {
	t.Parallel()

	h := inlineHarness(t, newInlineFake())

	if code := h.run("media:import", "a.mov", projectFlag, "--timeout", "100ms"); code != ExitTransient {
		t.Fatalf("exit %d, want 2: %s", code, h.stdout.String())
	}

	if !strings.Contains(h.stdout.String(), "timed out after 100ms") {
		t.Errorf("stdout = %q", h.stdout.String())
	}
}

func TestInProcessProgressLineGoesToStderrOnATerminal(t *testing.T) {
	t.Parallel()

	f := newInlineFake()
	h := inlineHarness(t, f)
	h.rt.StderrTTY = true

	h.run("export", projectFlag, "--timeout", "1200ms")

	if !strings.Contains(h.stderr.String(), "export running 40%") {
		t.Errorf("stderr = %q, want a progress line", h.stderr.String())
	}
}

func TestProgressLinesOfARemoteWaitAreOnlyOnATerminal(t *testing.T) {
	t.Parallel()

	newFake := func() *mediaFake {
		return &mediaFake{
			startAns: job("job_3", "running", "job_3 export running 10%"),
			getAns: []*service.Job{
				{JobID: "job_3", Kind: "export", State: "running", Progress: 0.6, Text: "x"},
				{JobID: "job_3", Kind: "export", State: "succeeded", Progress: 1, Text: "job_3 export succeeded 100%"},
			},
		}
	}

	h := mediaHarness(t, newFake())
	h.rt.StderrTTY = true

	if code := h.run("export", projectFlag); code != ExitOK {
		t.Fatalf("exit %d", code)
	}

	if !strings.Contains(h.stderr.String(), "export running 60%") {
		t.Errorf("stderr = %q, want a progress line", h.stderr.String())
	}

	for _, extra := range [][]string{{}, {"--json"}} {
		h = mediaHarness(t, newFake())
		h.rt.StderrTTY = len(extra) > 0

		if code := h.run(append([]string{"export", projectFlag}, extra...)...); code != ExitOK {
			t.Fatalf("exit %d", code)
		}

		if h.stderr.Len() != 0 {
			t.Errorf("%v: stderr = %q, want none", extra, h.stderr.String())
		}
	}
}
