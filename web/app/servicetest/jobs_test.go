package servicetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/queue"
)

type exportFn func(ctx context.Context, in service.ExportInput) (*service.ExportOutput, error)

func (f exportFn) Export(ctx context.Context, in service.ExportInput) (*service.ExportOutput, error) {
	return f(ctx, in)
}

var okChecks = []service.VerifyCheck{
	{Name: "streams", OK: true, Detail: "video and audio"},
	{Name: "duration", OK: true, Detail: "within one frame"},
	{Name: "dimensions", OK: true, Detail: "1080x1920"},
	{Name: "pixel_format", OK: true, Detail: "yuv420p"},
	{Name: "faststart", OK: true, Detail: "moov before mdat"},
}

// writeOutput is what a good exporter does: the file under OutPath, then its facts.
func writeOutput(in service.ExportInput, checks []service.VerifyCheck) (*service.ExportOutput, error) {
	body := []byte("an mp4, more or less")
	if err := os.WriteFile(in.OutPath, body, 0o600); err != nil {
		return nil, err
	}

	sum := sha256.Sum256(body)

	return &service.ExportOutput{
		Bytes: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), Frames: int(in.End - in.Start),
		VideoCodec: "h264", AudioCodec: "aac", Checks: checks,
	}, nil
}

func goodExporter() exportFn {
	return func(_ context.Context, in service.ExportInput) (*service.ExportOutput, error) {
		return writeOutput(in, okChecks)
	}
}

// exportable is a project the lint lets through: one visual track with a text.
func (m *mediaEnv) exportable() string {
	m.t.Helper()

	project, track := m.withTrack()
	if _, err := m.addText(ui, project, track, 1, "0s", "hello"); err != nil {
		m.t.Fatal(err)
	}

	return project
}

func (m *mediaEnv) startExport(project string) (*service.Job, error) {
	return m.svc.JobStart(m.as(ui), service.JobStartRequest{ProjectID: project, Kind: "export"})
}

func (m *mediaEnv) job(id string) *service.Job {
	m.t.Helper()

	j, err := m.svc.JobGet(m.as(ui), service.JobGetRequest{JobID: id})
	if err != nil {
		m.t.Fatal(err)
	}

	return j
}

// An export reaches succeeded only after the output passed verification, and
// the in-process queue runs it inside the call (P1-SRV-004 asserts 33 and 35).
func TestExportSucceedsInProcessAndTheFileIsServed(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	project := m.exportable()

	job, err := m.startExport(project)
	if err != nil {
		t.Fatal(err)
	}

	// queue.Sync runs the job before the call returns: the row is final.
	if job.State != service.JobSucceeded || job.Result == nil || job.Result.Export == nil {
		t.Fatalf("job = %+v", job)
	}

	e := job.Result.Export
	if e.Revision != 2 || e.Bytes == 0 || e.SHA256 == "" || e.Width != 1080 || e.Height != 1920 ||
		e.VideoCodec != "h264" ||
		e.DownloadURL != "/api/v1/jobs/"+job.JobID+"/file" ||
		len(e.Checks) != 5 ||
		!strings.HasSuffix(e.Path, job.JobID+".mp4") {
		t.Errorf("result = %+v", e)
	}

	if _, err := os.Stat(e.Path); err != nil {
		t.Errorf("the output is not at the path: %v", err)
	}

	if !strings.Contains(job.Text, "succeeded 100%") || !strings.Contains(job.Text, "file=") {
		t.Errorf("text = %s", job.Text)
	}

	// A network caller never sees the path.
	remote := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:u1", Scopes: service.AllScopes, OwnerUserID: "u1",
	})
	if got, gerr := m.svc.JobGet(remote, service.JobGetRequest{JobID: job.JobID}); gerr == nil && got.Result != nil &&
		got.Result.Export.Path != "" {
		t.Errorf("a network caller sees the path %q", got.Result.Export.Path)
	}

	list, err := m.svc.JobList(m.as(ui), service.JobListRequest{ProjectID: project})
	if err != nil || len(list.Jobs) != 1 || list.Jobs[0].JobID != job.JobID ||
		!strings.HasPrefix(list.Text, "jobs "+project) {
		t.Errorf("list = %+v, %v", list, err)
	}

	var states []string
	for _, ev := range m.eventsOf("job.updated") {
		states = append(states, ev.State)
	}

	if strings.Join(states, ",") != "queued,running,succeeded" {
		t.Errorf("job.updated = %v", states)
	}
}

func TestExportThatFailsVerificationNeverSucceeds(t *testing.T) {
	cases := map[string]exportFn{
		"a failed check": func(_ context.Context, in service.ExportInput) (*service.ExportOutput, error) {
			checks := append([]service.VerifyCheck{}, okChecks...)
			checks[4] = service.VerifyCheck{Name: "faststart", OK: false, Detail: "moov after mdat"}

			return writeOutput(in, checks)
		},
		"the exporter says so": func(_ context.Context, in service.ExportInput) (*service.ExportOutput, error) {
			_, _ = writeOutput(in, okChecks)

			return nil, &ffmpeg.Error{
				Code: "E_VERIFY_FAILED", Reason: "verify_failed", Message: "The exported file failed 1 check(s).",
				Details: map[string]any{"checks": []string{"duration"}},
			}
		},
		"no file": func(_ context.Context, in service.ExportInput) (*service.ExportOutput, error) {
			out, err := writeOutput(in, okChecks)
			_ = os.Remove(in.OutPath)

			return out, err
		},
		"another size": func(_ context.Context, in service.ExportInput) (*service.ExportOutput, error) {
			out, err := writeOutput(in, okChecks)
			out.Bytes += 7

			return out, err
		},
	}

	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			m := newMediaEnv(t, mediaCfg{exporter: fn})
			project := m.exportable()

			job, err := m.startExport(project)
			if err != nil {
				t.Fatal(err)
			}

			if job.State != service.JobFailed || job.Error == nil || job.Error.Code != "E_VERIFY_FAILED" ||
				job.Result != nil {
				t.Fatalf("job = %+v", job)
			}

			if _, err := os.Stat(m.ws + "/exports/" + project + "/" + job.JobID + ".mp4"); err == nil {
				t.Error("a file that failed verification is on disk")
			}

			if !strings.Contains(job.Text, "error E_VERIFY_FAILED") {
				t.Errorf("text = %s", job.Text)
			}
		})
	}
}

func TestExportErrorsOfTheEngineTravelInTheJob(t *testing.T) {
	m := newMediaEnv(
		t,
		mediaCfg{exporter: exportFn(func(context.Context, service.ExportInput) (*service.ExportOutput, error) {
			return nil, &ffmpeg.Error{
				Code:    "E_EXPORT_FAILED",
				Reason:  "encode",
				Message: "The encoder stopped.",
				Hint:    "try again",
			}
		})},
	)

	job, err := m.startExport(m.exportable())
	if err != nil || job.State != service.JobFailed || job.Error.Code != "E_EXPORT_FAILED" ||
		job.Error.Details["reason"] != "encode" || job.Error.Hint != "try again" {
		t.Fatalf("job = %+v, %v", job, err)
	}

	crash := newMediaEnv(
		t,
		mediaCfg{exporter: exportFn(func(context.Context, service.ExportInput) (*service.ExportOutput, error) {
			return nil, errors.New("open C:/secret/path: access denied")
		})},
	)

	j, _ := crash.startExport(crash.exportable())
	if j.Error == nil || j.Error.Code != command.CodeInternal || strings.Contains(j.Error.Message, "secret") {
		t.Errorf("an unknown error must be E_INTERNAL without its text: %+v", j.Error)
	}
}

func TestJobStartGates(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter(), noQueue: true})

	// An empty timeline has lint errors: the export is refused.
	empty := m.newProject("empty")
	_, err := m.startExport(empty)

	se := serviceError(t, err)
	if se.Code != service.CodeLintBlocking || se.Details["total"] == nil {
		t.Errorf("an empty timeline: %+v", se)
	}

	project := m.exportable()

	for name, req := range map[string]service.JobStartRequest{
		"unknown kind":     {ProjectID: project, Kind: "render"},
		"unknown argument": {ProjectID: project, Kind: "export", Args: json.RawMessage(`{"codec":"x"}`)},
		"range past end":   {ProjectID: project, Kind: "export", Args: json.RawMessage(`{"end":"10m"}`)},
		"an empty range":   {ProjectID: project, Kind: "export", Args: json.RawMessage(`{"start":"1f","end":"1f"}`)},
		"not a time":       {ProjectID: project, Kind: "export", Args: json.RawMessage(`{"start":"soon"}`)},
		"wait too long":    {ProjectID: project, Kind: "export", WaitMS: 99999},
	} {
		if _, err := m.svc.JobStart(m.as(ui), req); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	_, rerr := m.svc.JobStart(m.as(ui), service.JobStartRequest{
		ProjectID: project, Kind: "export", Args: json.RawMessage(`{"start":"1f","end":"1f"}`),
	})
	if se := serviceError(t, rerr); se.Code != command.CodeValidation || !strings.Contains(se.Message, "range") {
		t.Errorf("an empty range = %+v, want E_VALIDATION about the range", se)
	}

	if _, err := m.svc.JobStart(
		m.as(ui),
		service.JobStartRequest{ProjectID: project, Kind: "transcribe", Args: []byte(`{"asset_id":"ast_0000000001"}`)},
	); code(
		err,
	) != "E_NOT_FOUND" {
		t.Errorf("transcribe: %v", err)
	}

	dry, err := m.svc.JobStart(m.as(ui), service.JobStartRequest{ProjectID: project, Kind: "export", DryRun: true})
	if err != nil || dry.State != service.JobQueued {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}

	if rows, _ := m.store.Jobs().List(context.Background(), project, nil, 10); len(rows) != 0 {
		t.Errorf("a dry run queued %d jobs", len(rows))
	}

	scoped := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:k", Scopes: []string{service.ScopeRead, service.ScopeEdit}},
	)
	if _, err := m.svc.JobStart(
		scoped,
		service.JobStartRequest{ProjectID: project, Kind: "export"},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("an export needs render: %v", err)
	}
}

// With STUARTY_MAX_EXPORTS=2 the third start is E_QUEUE_FULL naming the two
// jobs and creates no row; 20 concurrent starts create exactly 2 (check 30).
func TestExportLimit(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter(), noQueue: true, limits: service.Limits{MaxExports: 2}})
	project := m.exportable()

	var ids []string

	for i, actor := range []string{"local:a", "local:b"} {
		job, err := m.svc.JobStart(m.as(actor), service.JobStartRequest{ProjectID: project, Kind: "export"})
		if err != nil {
			t.Fatalf("start %d: %v", i, err)
		}

		ids = append(ids, job.JobID)
	}

	_, err := m.svc.JobStart(m.as("local:c"), service.JobStartRequest{ProjectID: project, Kind: "export"})

	se := serviceError(t, err)
	if se.Code != service.CodeQueueFull || !se.Retryable || se.HTTPStatus() != 429 ||
		se.Details["limit"] != "exports" ||
		se.Details["variable"] != "STUARTY_MAX_EXPORTS" {
		t.Fatalf("error = %+v", se)
	}

	if after, _ := se.Details["retry_after"].(int); after < 1 {
		t.Errorf("details.retry_after = %v, want seconds to wait (VV-116)", se.Details["retry_after"])
	}

	running, _ := se.Details["running"].([]string)
	if len(running) != 2 || running[0] != ids[0] || running[1] != ids[1] {
		t.Errorf("details.running = %v, want %v", se.Details["running"], ids)
	}

	if rows, _ := m.store.Jobs().List(context.Background(), project, nil, 10); len(rows) != 2 {
		t.Errorf("rows = %d, want 2", len(rows))
	}
}

func TestConcurrentExportStartsCreateExactlyTheLimit(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter(), noQueue: true, limits: service.Limits{MaxExports: 2}})
	project := m.exportable()

	var (
		wg      sync.WaitGroup
		started atomic.Int64
		full    atomic.Int64
	)

	for i := range 20 {
		wg.Go(func() {
			actor := "local:u" + string(rune('a'+i))

			_, err := m.svc.JobStart(m.as(actor), service.JobStartRequest{ProjectID: project, Kind: "export"})

			switch code(err) {
			case "":
				started.Add(1)
			case service.CodeQueueFull:
				full.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		})
	}

	wg.Wait()

	if started.Load() != 2 || full.Load() != 18 {
		t.Errorf("started %d, refused %d, want 2 and 18", started.Load(), full.Load())
	}
}

func TestOneActorMayHaveOneRunningAndTwoWaitingExports(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter(), noQueue: true, limits: service.Limits{MaxExports: 10}})
	project := m.exportable()

	for i := range 3 {
		if _, err := m.startExport(project); err != nil {
			t.Fatalf("export %d: %v", i, err)
		}
	}

	if _, err := m.startExport(project); code(err) != service.CodeQueueFull {
		t.Errorf("the fourth export of one actor: %v", err)
	}
}

// A handler that reports 1,000 times in a second writes at most 5 updates and
// 5 events, and the last state reaches the stream (check 31).
func TestProgressIsThrottled(t *testing.T) {
	var reports atomic.Int64

	m := newMediaEnv(
		t,
		mediaCfg{exporter: exportFn(func(_ context.Context, in service.ExportInput) (*service.ExportOutput, error) {
			deadline := time.Now().Add(time.Second)

			for i := range 1000 {
				in.OnProgress(i, 1000)
				reports.Add(1)

				if wait := time.Until(deadline) / time.Duration(1000-i); wait > 0 {
					time.Sleep(wait)
				}
			}

			return writeOutput(in, okChecks)
		})},
	)

	project := m.exportable()

	job, err := m.startExport(project)
	if err != nil || job.State != service.JobSucceeded {
		t.Fatalf("job = %+v, %v", job, err)
	}

	if reports.Load() != 1000 {
		t.Fatalf("handler reported %d times", reports.Load())
	}

	var running, last int

	for _, ev := range m.eventsOf("job.updated") {
		if ev.State == "running" {
			running++
		}

		last++
	}

	// queued, the claim, at most 5 progress events, and the final one.
	if running-1 > 5 || last > 8 {
		t.Errorf("%d running events (one is the claim), %d in all", running, last)
	}

	events := m.eventsOf("job.updated")
	if final := events[len(events)-1]; final.State != "succeeded" || final.Progress != 1 || final.Result == nil {
		t.Errorf("the last event = %+v", final)
	}
}

func TestCancelQueuedAndRunningAndFinished(t *testing.T) {
	started := make(chan struct{})

	m := newMediaEnv(
		t,
		mediaCfg{exporter: exportFn(func(ctx context.Context, in service.ExportInput) (*service.ExportOutput, error) {
			_ = os.WriteFile(in.OutPath, []byte("partial"), 0o600)
			close(started)
			<-ctx.Done()

			return nil, ctx.Err()
		}), noQueue: true},
	)

	project := m.exportable()

	// Queued: cancelled at once, before any claim, and twice is the same.
	queued, err := m.startExport(project)
	if err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := m.runner.Cancel(context.Background(), queued.JobID); err != nil {
			t.Fatal(err)
		}
	}

	if j := m.job(queued.JobID); j.State != service.JobCancelled || j.Error == nil || j.Error.Code != "E_CANCELLED" {
		t.Fatalf("queued job after cancel = %+v", j)
	}

	if err := m.runner.RunJob(
		context.Background(),
		queued.JobID,
	); err != nil ||
		m.job(queued.JobID).State != service.JobCancelled {
		t.Error("a delivery for a cancelled job must do nothing")
	}

	// Running: cancelled within 2 s with no file at the final path.
	run, err := m.startExport(project)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)

	go func() { done <- m.runner.RunJob(context.Background(), run.JobID) }()

	<-started

	begin := time.Now()

	if err := m.runner.Cancel(context.Background(), run.JobID); err != nil {
		t.Fatal(err)
	}

	if err := <-done; err != nil {
		t.Fatalf("RunJob: %v", err)
	}

	if d := time.Since(begin); d > 2*time.Second {
		t.Errorf("cancellation took %v", d)
	}

	j := m.job(run.JobID)
	if j.State != service.JobCancelled || j.Error == nil || j.Error.Code != "E_CANCELLED" {
		t.Fatalf("running job after cancel = %+v", j)
	}

	if _, err := os.Stat(m.ws + "/exports/" + project + "/" + run.JobID + ".mp4"); err == nil {
		t.Error("a cancelled export left a file at the final path")
	}

	// Finished: the final row comes back unchanged.
	if err := m.runner.Cancel(
		context.Background(),
		run.JobID,
	); err != nil ||
		m.job(run.JobID).State != service.JobCancelled {
		t.Errorf("cancel of a final job changed it: %v", err)
	}
}

func TestTrashingAProjectCancelsItsExports(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter(), noQueue: true})
	project := m.exportable()

	job, err := m.startExport(project)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.ProjectTrash(m.as(ui), service.ProjectTrashRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	if j := m.job(job.JobID); j.State != service.JobCancelled {
		t.Errorf("job after the trash = %+v", j)
	}
}

// A job found running at start-up is requeued with a new message and ends
// succeeded; one interrupted three times fails; a graceful shutdown requeues and
// the next start completes it (check 34).
func TestRecoveryRequeuesAndGivesUp(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	project := m.exportable()
	ctx := context.Background()

	// A previous owner claimed the job and died: the row is running, attempts 1.
	var queued string

	m.insertExport(project, func(id string) { queued = id })

	if ok, err := m.store.Jobs().Claim(ctx, queued, 1); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}

	if err := m.runner.Recover(ctx); err != nil {
		t.Fatal(err)
	}

	if j := m.job(queued); j.State != service.JobSucceeded {
		t.Fatalf("after recovery: %+v", j)
	}

	// Interrupted three times: recovery gives up.
	var third string

	m.insertExport(project, func(id string) { third = id })

	for range 2 {
		if ok, _ := m.store.Jobs().Claim(ctx, third, 2); !ok {
			t.Fatal("claim")
		}

		if _, err := m.store.Jobs().Requeue(ctx, third, "x", 3); err != nil {
			t.Fatal(err)
		}
	}

	if ok, _ := m.store.Jobs().Claim(ctx, third, 4); !ok {
		t.Fatal("third claim")
	}

	// The job was claimed three times and died running: attempts is 3.
	if err := m.runner.Recover(ctx); err != nil {
		t.Fatal(err)
	}

	j := m.job(third)
	if j.State != service.JobFailed || j.Error == nil || j.Error.Code != service.CodeExportFailed ||
		j.Error.Details["reason"] != "interrupted" {
		t.Errorf("a job interrupted too often: %+v", j)
	}
}

// A queued row whose message was lost gets one more message once it is older
// than a minute (SDD-005 section 14.5 item 3).
func TestRecoveryRepairsAQueuedJobWhoseMessageWasLost(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	project := m.exportable()

	var id string

	m.insertExport(project, func(got string) { id = got })

	if err := m.runner.Recover(context.Background()); err != nil || m.job(id).State != service.JobQueued {
		t.Fatalf("a young queued job must be left alone: %v, %+v", err, m.job(id))
	}

	m.clock.advance(2 * time.Minute)

	if err := m.runner.Recover(context.Background()); err != nil || m.job(id).State != service.JobSucceeded {
		t.Errorf("after two minutes: %v, %+v", err, m.job(id))
	}
}

func TestGracefulShutdownRequeuesAndTheNextStartCompletes(t *testing.T) {
	entered := make(chan struct{}, 1)
	block := atomic.Bool{}
	block.Store(true)

	m := newMediaEnv(
		t,
		mediaCfg{
			noQueue: true,
			exporter: exportFn(func(ctx context.Context, in service.ExportInput) (*service.ExportOutput, error) {
				if block.Load() {
					entered <- struct{}{}
					<-ctx.Done()

					return nil, ctx.Err()
				}

				return writeOutput(in, okChecks)
			}),
		},
	)

	project := m.exportable()

	job, err := m.startExport(project)
	if err != nil {
		t.Fatal(err)
	}

	shutdown, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- m.runner.RunJob(shutdown, job.JobID) }()

	<-entered
	stop()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("RunJob = %v, want the context error so the queue keeps the message", err)
	}

	j := m.job(job.JobID)
	if j.State != service.JobQueued || j.Message != "interrupted by shutdown" {
		t.Fatalf("after the shutdown: %+v", j)
	}

	block.Store(false)

	if err := m.runner.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	if j := m.job(job.JobID); j.State != service.JobSucceeded {
		t.Errorf("after the next start: %+v", j)
	}
}

func TestAPanickingJobFailsInsteadOfStayingRunning(t *testing.T) {
	m := newMediaEnv(
		t,
		mediaCfg{
			noQueue: true,
			exporter: exportFn(func(context.Context, service.ExportInput) (*service.ExportOutput, error) {
				panic("boom")
			}),
		},
	)

	project := m.exportable()

	job, err := m.startExport(project)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.runner.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatalf("RunJob = %v, want nil: the outcome is written", err)
	}

	j := m.job(job.JobID)
	if j.State != service.JobFailed || j.Error == nil || j.Error.Code != command.CodeInternal {
		t.Fatalf("after the panic: %+v", j)
	}

	if _, err := m.startExport(project); err != nil {
		t.Errorf("a second export after the panic: %v", err)
	}
}

func TestAJobPastItsQueueDeadlineFails(t *testing.T) {
	m := newMediaEnv(
		t,
		mediaCfg{
			noQueue: true,
			exporter: exportFn(func(ctx context.Context, _ service.ExportInput) (*service.ExportOutput, error) {
				<-ctx.Done()

				return nil, ctx.Err()
			}),
		},
	)

	job, err := m.startExport(m.exportable())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := m.runner.RunJob(ctx, job.JobID); err != nil {
		t.Fatalf("RunJob = %v, want nil: the deadline ends the job", err)
	}

	j := m.job(job.JobID)
	if j.State != service.JobFailed || j.Error == nil || j.Error.Details["reason"] != "timeout" {
		t.Fatalf("after the deadline: %+v", j)
	}
}

// insertExport creates a queued export row that no queue message points at.
func (m *mediaEnv) insertExport(project string, got func(id string)) {
	m.t.Helper()

	m.hold.Store(true)
	defer m.hold.Store(false)

	job, err := m.svc.JobStart(m.as(ui), service.JobStartRequest{ProjectID: project, Kind: "export"})
	if err != nil {
		m.t.Fatal(err)
	}

	got(job.JobID)
}

// The durable queue: the message is inserted with the job row, a worker runs
// it within the 1.5 s of SDD-005 section 14.1, and with the tables of vukem
// next to a jobs table of our own (check 36).
func TestDurableQueueRunsAnExportWithAWorker(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter(), durable: true})
	project := m.exportable()

	ctx, stop := context.WithCancel(context.Background())
	worked := make(chan error, 1)

	go func() {
		worked <- m.queue.Work(ctx, queue.WorkOptions{
			Queues: []string{"ingest", "export", "default"}, Concurrency: 2, Sleep: 50 * time.Millisecond,
			StopTimeout: 5 * time.Second,
		})
	}()

	begin := time.Now()

	job, err := m.startExport(project)
	if err != nil || job.State != service.JobQueued {
		t.Fatalf("job = %+v, %v", job, err)
	}

	eventually(t, "the export to succeed", func() bool { return m.job(job.JobID).State == service.JobSucceeded })

	if d := time.Since(begin); d > 1500*time.Millisecond {
		t.Errorf("start latency %v, want under 1.5 s", d)
	}

	stop()

	if err := <-worked; err != nil {
		t.Errorf("worker: %v", err)
	}

	var tables []string

	rows, err := m.store.DB().QueryContext(context.Background(),
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name IN ('queue_jobs','queue_failed_jobs','jobs') ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}

	defer rows.Close()

	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		tables = append(tables, n)
	}

	if strings.Join(tables, ",") != "jobs,queue_failed_jobs,queue_jobs" {
		t.Errorf("tables = %v", tables)
	}
}

func TestIngestOnTheDurableQueueSurvivesARestartOfTheWorker(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{durable: true})
	project := m.newProject("p")

	a := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0]
	if a.State != "pending" || a.JobID == "" {
		t.Fatalf("entry = %+v", a)
	}

	// No worker ran: the message waits in the table with the job row.
	if n, err := m.queue.Driver().(*queue.Database).Pending(context.Background()); err != nil || n != 1 {
		t.Fatalf("pending messages = %d, %v", n, err)
	}

	ctx, stop := context.WithCancel(context.Background())
	worked := make(chan error, 1)

	go func() {
		worked <- m.queue.Work(ctx, queue.WorkOptions{Queues: []string{"ingest"}, Sleep: 20 * time.Millisecond})
	}()

	eventually(t, "the ingest", func() bool { return m.job(a.JobID).State == service.JobSucceeded })
	stop()
	<-worked

	res, err := m.svc.MediaList(m.as(ui), service.MediaListRequest{ProjectID: project})
	if err != nil || res.Assets[0].State != "ready" {
		t.Errorf("list = %+v, %v", res, err)
	}
}

// Ingest jobs run at most MaxIngests at once; the others wait queued.
func TestIngestConcurrencyIsBounded(t *testing.T) {
	var running, peak atomic.Int64

	var m *mediaEnv

	m = newMediaEnv(
		t,
		mediaCfg{
			noQueue: true,
			limits:  service.Limits{MaxIngests: 2},
			ingest: func(_ context.Context, in ingest.Input) (*ingest.Result, error) {
				n := running.Add(1)
				defer running.Add(-1)

				for {
					p := peak.Load()
					if n <= p || peak.CompareAndSwap(p, n) {
						break
					}
				}

				time.Sleep(40 * time.Millisecond)

				return fakeResult(m.ws, in)
			},
		},
	)

	project := m.newProject("p")

	var ids []string

	for i := range 6 {
		a := m.importOne(project, m.write("clip"+string(rune('a'+i))+".mp4", clipBytes+string(rune('a'+i)))).Assets[0]
		ids = append(ids, a.JobID)
	}

	var wg sync.WaitGroup

	for _, id := range ids {
		wg.Go(func() { _ = m.runner.RunJob(context.Background(), id) })
	}

	wg.Wait()

	if peak.Load() > 2 || peak.Load() < 1 {
		t.Errorf("peak concurrent ingests = %d, want at most 2", peak.Load())
	}

	for _, id := range ids {
		if j := m.job(id); j.State != service.JobSucceeded {
			t.Errorf("job %s = %s", id, j.State)
		}
	}
}

// An export renders the document of the revision recorded at the start, even
// when an edit lands before the job runs (SDD-003 section 2.8 rule 9).
func TestExportRendersTheRevisionPinnedAtStart(t *testing.T) {
	var rendered []int

	m := newMediaEnv(t, mediaCfg{noQueue: true, exporter: exportFn(
		func(_ context.Context, in service.ExportInput) (*service.ExportOutput, error) {
			rendered = append(rendered, len(in.Doc.Items))

			return writeOutput(in, okChecks)
		},
	)})
	project, track := m.withTrack()
	if _, err := m.addText(ui, project, track, 1, "0s", "hello"); err != nil {
		t.Fatal(err)
	}

	job, err := m.startExport(project)
	if err != nil {
		t.Fatal(err)
	}

	head := m.job(job.JobID)
	if head.State != service.JobQueued {
		t.Fatalf("state = %s, want queued", head.State)
	}

	if _, err := m.addText(ui, project, track, 2, "1s", "later"); err != nil {
		t.Fatal(err)
	}

	if err := m.runner.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	done := m.job(job.JobID)
	if done.State != service.JobSucceeded || done.Result.Export.Revision != 2 {
		t.Fatalf("job = %+v", done)
	}

	if len(rendered) != 1 || rendered[0] != 1 {
		t.Errorf("the export rendered %v items, want the 1 of the pinned revision", rendered)
	}
}

// The real engine renders the pinned revision: its file has the frames of the
// document at the start, not of the document after the later edit.
func TestRealExportHasTheFramesOfThePinnedRevision(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})

	project, track := m.withTrack()
	if _, err := m.addText(ui, project, track, 1, "0s", "hello"); err != nil {
		t.Fatal(err)
	}

	job, err := m.startExport(project)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.addText(ui, project, track, 2, "1s", "later"); err != nil {
		t.Fatal(err)
	}

	if err := m.runner.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	done := m.job(job.JobID)
	if done.State != service.JobSucceeded || done.Result == nil || done.Result.Export == nil {
		t.Fatalf("job = %+v", done)
	}

	e := done.Result.Export
	if e.Revision != 2 || e.Duration.Seconds != 1 || e.Bytes == 0 || len(e.SHA256) != 64 {
		t.Errorf("result = %+v", e)
	}
}

// SDD-005 section 18 check 32 through the operation: a queued export ends
// cancelled, twice is the same, a final job comes back unchanged, and the scope
// is render, or media for the caller's own ingest.
func TestJobCancelOperation(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, exporter: goodExporter()})
	project := m.exportable()

	queued, err := m.startExport(project)
	if err != nil {
		t.Fatal(err)
	}

	read := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:r", Scopes: []string{service.ScopeRead, service.ScopeEdit}},
	)
	if _, err := m.svc.JobCancel(
		read,
		service.JobCancelRequest{JobID: queued.JobID},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("cancel without render = %v", err)
	}

	for range 2 {
		j, err := m.svc.JobCancel(m.as(ui), service.JobCancelRequest{JobID: queued.JobID})
		if err != nil || j.State != service.JobCancelled || j.Error == nil || j.Error.Code != service.CodeCancelled {
			t.Fatalf("JobCancel of a queued export = %+v, %v", j, err)
		}
	}

	if _, err := m.svc.JobCancel(
		m.as(ui),
		service.JobCancelRequest{JobID: "job_0000000000"},
	); code(
		err,
	) != "E_NOT_FOUND" {
		t.Errorf("unknown job = %v", err)
	}

	// A job that already succeeded is reported as it is.
	done := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	finished, err := done.startExport(done.exportable())
	if err != nil || finished.State != service.JobSucceeded {
		t.Fatalf("export = %+v, %v", finished, err)
	}

	if j, err := done.svc.JobCancel(done.as(ui), service.JobCancelRequest{JobID: finished.JobID}); err != nil ||
		j.State != service.JobSucceeded {
		t.Errorf("cancel after completion = %+v, %v", j, err)
	}

	// Media is enough for the caller's own ingest, and for no one else's.
	ing := newMediaEnv(t, mediaCfg{noQueue: true})
	p := ing.newProject("p")
	owner := service.Caller{Actor: "key:owner", Scopes: []string{service.ScopeMedia}}

	res, err := ing.svc.MediaImport(service.WithCaller(context.Background(), owner), service.MediaImportRequest{
		ProjectID: p, Files: []service.ImportFile{{Path: ing.write("a.mp4", clipBytes)}},
	})
	if err != nil || res.Assets[0].JobID == "" {
		t.Fatalf("import = %+v, %v", res, err)
	}

	other := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:other", Scopes: []string{service.ScopeMedia}},
	)
	if _, err := ing.svc.JobCancel(
		other,
		service.JobCancelRequest{JobID: res.Assets[0].JobID},
	); code(
		err,
	) != service.CodeForbidden {
		t.Errorf("another actor's ingest = %v", err)
	}

	j, err := ing.svc.JobCancel(
		service.WithCaller(context.Background(), owner),
		service.JobCancelRequest{JobID: res.Assets[0].JobID},
	)
	if err != nil || j.State != service.JobCancelled {
		t.Errorf("own ingest with media = %+v, %v", j, err)
	}
}

// An ingest is shared by content hash: a caller who cannot see every project
// that waits on it unlinks its own reference and leaves the job running.
func TestJobCancelOfASharedIngestSparesTheOtherProjects(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true})
	alice := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:alice", OwnerUserID: "alice", Scopes: service.AllScopes,
	})
	bob := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:bob", OwnerUserID: "bob", Scopes: service.AllScopes,
	})

	var projects [2]string

	for i, c := range []context.Context{alice, bob} {
		res, err := m.svc.ProjectCreate(c, service.ProjectCreateRequest{Name: "p"})
		if err != nil {
			t.Fatal(err)
		}

		projects[i] = res.ProjectID
	}

	clip := m.write("shared.mp4", clipBytes)

	var assets [2]service.ImportedAsset

	for i, c := range []context.Context{alice, bob} {
		res, err := m.svc.MediaImport(c, service.MediaImportRequest{
			ProjectID: projects[i], Files: []service.ImportFile{{Path: clip}},
		})
		if err != nil {
			t.Fatal(err)
		}

		assets[i] = res.Assets[0]
	}

	if assets[0].JobID == "" || assets[0].JobID != assets[1].JobID {
		t.Fatalf("the two imports must share one ingest job: %+v", assets)
	}

	job := assets[0].JobID

	// Alice cannot see Bob's project: her cancel unlinks her reference only.
	j, err := m.svc.JobCancel(alice, service.JobCancelRequest{JobID: job})
	if err != nil || j.State != service.JobQueued {
		t.Fatalf("a cancel by one of two owners = %+v, %v; the job must keep running", j, err)
	}

	if m.assetRow(projects[0], assets[0].AssetID).State != service.ProjectAssetTrashed {
		t.Error("the reference of the caller was not unlinked")
	}

	if m.assetRow(projects[1], assets[1].AssetID).State != service.ProjectAssetPending {
		t.Error("the reference of the other project was touched")
	}

	// Bob is now the only one waiting: his cancel ends the job.
	if j, err = m.svc.JobCancel(
		bob,
		service.JobCancelRequest{JobID: job},
	); err != nil ||
		j.State != service.JobCancelled {
		t.Errorf("a cancel by the last waiter = %+v, %v", j, err)
	}
}

// refsFailStore makes the read of the references of a hash fail.
type refsFailStore struct{ service.Store }

func (s refsFailStore) Write(ctx context.Context, fn func(context.Context, service.Tx) error) error {
	return s.Store.Write(ctx, func(ctx context.Context, tx service.Tx) error { return fn(ctx, refsFailTx{tx}) })
}

type refsFailTx struct{ service.Tx }

func (t refsFailTx) Assets() service.AssetRepo { return refsFailAssets{t.Tx.Assets()} }

type refsFailAssets struct{ service.AssetRepo }

func (refsFailAssets) ProjectsReferencing(context.Context, string) ([]string, error) {
	return nil, errors.New("lookup failed")
}

// The decision of a cancellation of a shared ingest fails closed: when the
// references cannot be read the job keeps running and the caller gets an error.
func TestJobCancelOfAnIngestFailsClosedWhenTheReferencesCannotBeRead(t *testing.T) {
	var failing atomic.Bool

	m := newMediaEnv(t, mediaCfg{noQueue: true, wrapStore: func(s service.Store) service.Store {
		return switchStore{Store: s, failing: &failing}
	}})
	project := m.newProject("p")
	job := m.importOne(project, m.write("a.mp4", clipBytes)).Assets[0].JobID

	failing.Store(true)

	if _, err := m.svc.JobCancel(m.as(ui), service.JobCancelRequest{JobID: job}); err == nil {
		t.Fatal("JobCancel succeeded although the references could not be read")
	}

	failing.Store(false)

	if j := m.job(job); j.State != service.JobQueued {
		t.Errorf("the job is %s after a failed decision; it must keep running", j.State)
	}
}

// switchStore fails the reference lookup while failing is set.
type switchStore struct {
	service.Store
	failing *atomic.Bool
}

func (s switchStore) Write(ctx context.Context, fn func(context.Context, service.Tx) error) error {
	if s.failing.Load() {
		return refsFailStore{s.Store}.Write(ctx, fn)
	}

	return s.Store.Write(ctx, fn)
}

// P2-SRV-020: a job writes a line when it starts and when it ends, with its id,
// kind, actor and project, and never the text of the media (a failure names its
// code and reason, not its message).
func TestJobsWriteLogLines(t *testing.T) {
	var buf bytes.Buffer

	failing := func(context.Context, ingest.Input) (*ingest.Result, error) {
		return nil, &ffmpeg.Error{
			Code: "E_UNSUPPORTED_MEDIA", Reason: "undecodable", Message: "secret-title.mp4 is broken",
		}
	}
	m := newMediaEnv(t, mediaCfg{
		ingest: failing, log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	project := m.newProject("p")

	imported := m.importOne(project, m.write("secret-title.mp4", clipBytes))
	job := imported.Assets[0].JobID

	out := buf.String()
	for _, want := range []string{
		"msg=\"job started\"", "msg=\"job failed\"", "job_id=" + job, "kind=ingest", "actor=" + ui,
		"code=E_UNSUPPORTED_MEDIA", "reason=undecodable",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the log lacks %q:\n%s", want, out)
		}
	}

	if strings.Contains(out, "secret-title") {
		t.Errorf("the log holds text from the media:\n%s", out)
	}
}
