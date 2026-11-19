package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/library"
)

// An in-memory store of the job tables and one project, enough to run ExportBatch and its worker: the sqlite
// store lives in web/app/store, which imports this package.

type memJobs struct {
	JobRepo
	mu   sync.Mutex
	rows map[string]*JobRow
	// full, when set, hears of every creation refused because the ceiling is reached, so a test waits for the
	// refusal instead of for a duration.
	full chan struct{}
}

func (m *memJobs) refused() {
	if m.full != nil {
		select {
		case m.full <- struct{}{}:
		default:
		}
	}
}

func (m *memJobs) CreateLimited(_ context.Context, row JobRow, lim *JobLimit) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.rows[row.ID]; ok {
		return ErrExists
	}

	if lim != nil {
		var kind, actor []string

		for _, r := range m.rows {
			if r.Kind == lim.Kind && (r.State == JobQueued || r.State == JobRunning) {
				kind = append(kind, r.ID)

				if r.Actor == lim.Actor {
					actor = append(actor, r.ID)
				}
			}
		}

		if lim.Max > 0 && len(kind) >= lim.Max {
			m.refused()

			return &QueueFullError{Running: kind, Kind: lim.Kind, Ceiling: lim.Max}
		}

		if lim.Actor != "" && lim.MaxPerActor > 0 && len(actor) >= lim.MaxPerActor {
			m.refused()

			return &QueueFullError{Running: actor, Kind: lim.Kind, Ceiling: lim.MaxPerActor, PerActor: true}
		}
	}

	row.State = JobQueued
	m.rows[row.ID] = &row

	return nil
}

func (m *memJobs) Get(_ context.Context, id string) (JobRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.rows[id]
	if !ok {
		return JobRow{}, ErrNotFound
	}

	return *r, nil
}

func (m *memJobs) Claim(_ context.Context, id string, now int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.rows[id]
	if !ok || r.State != JobQueued {
		return false, nil
	}

	r.State, r.StartedMs, r.Attempts = JobRunning, &now, r.Attempts+1

	return true, nil
}

func (m *memJobs) Progress(_ context.Context, id string, p float64, msg string, _ int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.rows[id]
	if !ok || r.State != JobRunning {
		return false, nil
	}

	r.Progress, r.Message = p, msg

	return true, nil
}

func (m *memJobs) Finish(
	_ context.Context,
	id, state string,
	result, errJSON json.RawMessage,
	now int64,
) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.rows[id]
	if !ok || r.State != JobRunning {
		return false, nil
	}

	r.State, r.Result, r.Error, r.FinishedMs = state, result, errJSON, &now
	if state == JobSucceeded {
		r.Progress = 1
	}

	return true, nil
}

func (m *memJobs) Requeue(_ context.Context, id, msg string, _ int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.rows[id]
	if !ok || r.State != JobRunning {
		return false, nil
	}

	r.State, r.Message = JobQueued, msg

	return true, nil
}

func (m *memJobs) RequestCancel(_ context.Context, id string, now int64) (JobRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.rows[id]
	if !ok {
		return JobRow{}, ErrNotFound
	}

	if !JobFinal(r.State) {
		r.CancelRequested = true
		if r.State == JobQueued {
			r.State, r.FinishedMs = JobCancelled, &now
		}
	}

	return *r, nil
}

func (m *memJobs) states() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := map[string]string{}
	for id, r := range m.rows {
		out[id] = r.State
	}

	return out
}

type batchAudit struct{ AuditRepo }

func (batchAudit) Append(context.Context, AuditRow) error { return nil }

type batchProjects struct {
	ProjectRepo
	row ProjectRow
}

func (p batchProjects) Get(_ context.Context, id string) (ProjectRow, error) {
	if id != p.row.ID {
		return ProjectRow{}, ErrNotFound
	}

	return p.row, nil
}

type batchTx struct {
	Tx
	st    *batchStore
	hooks *[]func()
}

func (t batchTx) Jobs() JobRepo         { return t.st.jobs }
func (t batchTx) Audit() AuditRepo      { return batchAudit{} }
func (t batchTx) AfterCommit(fn func()) { *t.hooks = append(*t.hooks, fn) }

type batchStore struct {
	Store
	jobs     *memJobs
	projects batchProjects
}

// No test here reads a connection; the manager the service builds only needs a store to hold.
func (b *batchStore) Connections() connections.Store { return nil }

// Nor does any test here use the library; its manager only needs an index to hold.
func (b *batchStore) Library() library.Index { return nil }

// The project of these tests has one timeline, whose id is its own (SDD-003 section 1.7).
func (b *batchStore) Productions() ProductionRepo { return batchProductions{} }

type batchProductions struct{ ProductionRepo }

func (batchProductions) ActiveTimelines(context.Context, string) ([]TimelineRef, error) {
	return nil, nil
}

func (b *batchStore) Jobs() JobRepo         { return b.jobs }
func (b *batchStore) Audit() AuditRepo      { return batchAudit{} }
func (b *batchStore) Projects() ProjectRepo { return b.projects }

func (b *batchStore) Write(ctx context.Context, fn func(context.Context, Tx) error) error {
	var hooks []func()

	if err := fn(ctx, batchTx{st: b, hooks: &hooks}); err != nil {
		return err
	}

	for _, h := range hooks {
		h()
	}

	return nil
}

// batchEnv is a service over the memory store, a project of the given seconds at 1080x1920 and 30 fps, and an
// exporter that the test controls.
type batchEnv struct {
	s     *svc
	jobs  *memJobs
	ws    string
	calls atomic.Int32
	peak  atomic.Int32
	live  atomic.Int32
}

const batchProject = "prj_7k2m9x4q1b"

type exportHook func(ctx context.Context, in ExportInput) error

func newBatchEnv(t *testing.T, seconds int, limits Limits, hook exportHook) *batchEnv {
	t.Helper()

	doc := &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion, ID: batchProject, Name: "Batch",
		Settings: timeline.DefaultSettings(),
		Assets:   map[string]timeline.Asset{}, Tracks: map[string]timeline.Track{}, Items: map[string]timeline.Item{},
	}
	doc.Settings.Width, doc.Settings.Height, doc.Settings.FPS = 1080, 1920, timeline.Rational{Num: 30, Den: 1}
	doc.Tracks["trk_aaaaaaaaaa"] = timeline.Track{
		ID:    "trk_aaaaaaaaaa",
		Kind:  timeline.TrackVisual,
		Name:  "Text",
		Order: "a0",
	}
	it := timeline.NewItem("itm_aaaaaaaaaa", timeline.ItemText, "trk_aaaaaaaaaa", 0, seconds*30)
	it.Text = "hello"
	doc.Items[it.ID] = it

	if vs := timeline.Validate(doc); len(vs) > 0 {
		t.Fatalf("the test project is not valid: %+v", vs[0])
	}

	canon := timeline.Canonical(doc)
	st := &batchStore{
		jobs: &memJobs{rows: map[string]*JobRow{}},
		projects: batchProjects{row: ProjectRow{
			ProjectHead: ProjectHead{ID: batchProject, Name: "Batch", State: ProjectActive, Revision: 7},
			Document:    canon,
		}},
	}

	env := &batchEnv{jobs: st.jobs, ws: t.TempDir()}

	exporter := exportFn(func(ctx context.Context, in ExportInput) (*ExportOutput, error) {
		env.calls.Add(1)

		n := env.live.Add(1)
		defer env.live.Add(-1)

		for {
			p := env.peak.Load()
			if n <= p || env.peak.CompareAndSwap(p, n) {
				break
			}
		}

		if hook != nil {
			if err := hook(ctx, in); err != nil {
				return nil, err
			}
		}

		body := []byte("an mp4 for " + in.Preset)
		if err := os.WriteFile(in.OutPath, body, 0o600); err != nil {
			return nil, err
		}

		return &ExportOutput{
			Bytes: int64(
				len(body),
			),
			SHA256:     "00",
			Frames:     int(in.End - in.Start),
			VideoCodec: "h264",
			AudioCodec: "aac",
			Checks:     []VerifyCheck{{Name: "streams", OK: true, Detail: "ok"}},
		}, nil
	})

	n := 0

	limits.MinFreeBytes = -1

	built, err := New(Options{
		Store: st, Workspace: env.ws, Exporter: exporter, Limits: limits,
		NewIDs: func(prefix string) string {
			n++

			return fmt.Sprintf("%s_%010x", prefix, n)[:14]
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	env.s = built.(*svc)

	return env
}

type exportFn func(ctx context.Context, in ExportInput) (*ExportOutput, error)

func (f exportFn) Export(ctx context.Context, in ExportInput) (*ExportOutput, error) {
	return f(ctx, in)
}

func renderCtx() context.Context {
	return WithCaller(context.Background(), Caller{Actor: "local:ui", Scopes: []string{ScopeRead, ScopeRender}})
}

func batchItems(presets ...string) []ExportBatchItem {
	items := make([]ExportBatchItem, len(presets))
	for i, p := range presets {
		items[i] = ExportBatchItem{Preset: p}
	}

	return items
}

// TestExportBatchPartialFailure is acceptance criterion 5: a batch of three exports, one of which cannot fit its
// preset, ends with two files and one actionable error, and the failure stops no other export.
func TestExportBatchPartialFailure(t *testing.T) {
	t.Parallel()

	// 100 seconds: it fits TikTok and YouTube Shorts (180 s), not Facebook Reels (90 s).
	env := newBatchEnv(t, 100, Limits{MaxExports: 2}, nil)
	ctx := renderCtx()

	job, err := env.s.ExportBatch(ctx, ExportBatchRequest{
		ProjectID: batchProject, Items: batchItems("tiktok", "facebook-reels", "youtube-shorts"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if job.Kind != JobExportBatch || job.State != JobQueued || job.Result == nil || job.Result.Batch.Total != 3 {
		t.Fatalf("the parent after the start: %+v", job)
	}

	if err := env.s.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	got, err := env.s.JobGet(ctx, JobGetRequest{JobID: job.JobID})
	if err != nil {
		t.Fatal(err)
	}

	if got.State != JobFailed || got.Error == nil || got.Error.Code != command.CodeValidation {
		t.Fatalf("the parent ends %s with %+v", got.State, got.Error)
	}

	b := got.Result.Batch
	if b.Total != 3 || b.Succeeded != 2 || b.Failed != 1 || b.Cancelled != 0 {
		t.Fatalf("batch %+v", b)
	}

	for i, want := range []string{JobSucceeded, JobFailed, JobSucceeded} {
		if b.Items[i].State != want || b.Items[i].JobID != batchChildID(job.JobID, i) {
			t.Errorf("item %d: %+v, want %s", i, b.Items[i], want)
		}
	}

	for _, i := range []int{0, 2} {
		r := b.Items[i].Result
		if r == nil || r.Preset != b.Items[i].Preset || r.VideoCodec != "h264" {
			t.Errorf("item %d result %+v", i, r)
		}

		if _, err := os.Stat(r.Path); err != nil {
			t.Errorf("item %d: its file is not there: %v", i, err)
		}
	}

	// The error is the actionable one of the child: what did not fit and what to do, and the batch count.
	e := got.Error
	if !strings.Contains(e.Message, "Export 2 of 3 (facebook-reels) failed") || !strings.Contains(e.Message, "90 s") ||
		e.Hint == "" || e.Details["reason"] != "preset_mismatch" || e.Details["findings"] == nil {
		t.Errorf("error %+v", e)
	}

	sum, _ := e.Details["batch"].(map[string]any)
	if sum["total"] != float64(3) || sum["succeeded"] != float64(2) || sum["failed"] != float64(1) ||
		sum["item"] != float64(1) {
		t.Errorf("details.batch %v", e.Details["batch"])
	}

	if n := env.calls.Load(); n != 2 {
		t.Errorf("the exporter ran %d times, want 2 (the item that does not fit never encodes)", n)
	}

	if !strings.Contains(got.Text, "batch: 2 of 3 done, 1 failed") || !strings.Contains(got.Text, "facebook-reels") {
		t.Errorf("text:\n%s", got.Text)
	}
}

// TestExportBatchRunsOneEncodeAtATime: the children run one after the other, so the batch never holds more than
// one encode, whatever the ceiling of the server is.
func TestExportBatchRunsOneEncodeAtATime(t *testing.T) {
	t.Parallel()

	// Each encode looks at the jobs while it runs: only the parent and its own child may be unfinished, so no
	// other child is queued or running beside it, whatever the ceiling of the server allows.
	var (
		env     *batchEnv
		overlap atomic.Int32
	)

	env = newBatchEnv(t, 10, Limits{MaxExports: 4}, func(context.Context, ExportInput) error {
		active := 0

		for _, state := range env.jobs.states() {
			if !JobFinal(state) {
				active++
			}
		}

		if active != 2 {
			overlap.Add(1)
		}

		return nil
	})

	job, err := env.s.ExportBatch(renderCtx(), ExportBatchRequest{
		ProjectID: batchProject, Items: batchItems("tiktok", "youtube-shorts", "instagram-reels", "x-video"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := env.s.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	got, _ := env.s.JobGet(renderCtx(), JobGetRequest{JobID: job.JobID})
	if got.State != JobSucceeded || got.Error != nil || got.Progress != 1 || got.Result.Batch.Succeeded != 4 {
		t.Fatalf("batch %s %+v", got.State, got.Result.Batch)
	}

	if env.peak.Load() != 1 || env.calls.Load() != 4 || overlap.Load() != 0 {
		t.Errorf("peak %d, calls %d, encodes beside another child %d: want one encode at a time, four in all",
			env.peak.Load(), env.calls.Load(), overlap.Load())
	}
}

// TestExportBatchWaitsForASlotInsteadOfRefusing is VV-220 and the ceilings of the epic: when the exports of the
// server are all busy, the batch waits for a slot and goes on, and does not fail with E_QUEUE_FULL.
func TestExportBatchWaitsForASlotInsteadOfRefusing(t *testing.T) {
	t.Parallel()

	env := newBatchEnv(t, 10, Limits{MaxExports: 1}, nil)

	// Another caller's export holds the only slot.
	busy := JobRow{ID: "job_busybusyb0", Kind: JobExport, Actor: "local:other", State: JobQueued, Input: []byte("{}")}
	project := batchProject
	busy.ProjectID = &project

	if err := env.jobs.CreateLimited(context.Background(), busy, nil); err != nil {
		t.Fatal(err)
	}

	job, err := env.s.ExportBatch(renderCtx(), ExportBatchRequest{
		ProjectID: batchProject, Items: batchItems("tiktok", "youtube-shorts"),
	})
	if err != nil {
		t.Fatal(err)
	}

	env.jobs.full = make(chan struct{}, 1)
	done := make(chan error, 1)

	go func() { done <- env.s.RunJob(context.Background(), job.JobID) }()

	// While the slot is taken the parent asks for it, is refused, and asks again: two refusals prove it waits
	// instead of failing, and nothing is created meanwhile.
	for range 2 {
		select {
		case <-env.jobs.full:
		case err := <-done:
			t.Fatalf("the batch ended while the slot was busy: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("the batch never asked for the busy slot")
		}
	}

	if got := env.jobs.states(); len(got) != 2 || got[job.JobID] != JobRunning {
		t.Fatalf("jobs while the slot is busy: %v", got)
	}

	if _, err := env.jobs.Claim(context.Background(), busy.ID, 1); err != nil {
		t.Fatal(err)
	}

	if ok, err := env.jobs.Finish(context.Background(), busy.ID, JobSucceeded, nil, nil, 2); err != nil || !ok {
		t.Fatalf("finish: %v %v", ok, err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the batch never went on after the slot was free")
	}

	got, _ := env.s.JobGet(renderCtx(), JobGetRequest{JobID: job.JobID})
	if got.State != JobSucceeded || got.Result.Batch.Succeeded != 2 {
		t.Fatalf("batch %s %+v error %+v", got.State, got.Result.Batch, got.Error)
	}
}

// TestExportBatchCancelCancelsTheRest: cancelling the parent cancels the child that runs and leaves the others
// cancelled; nothing is exported after it.
func TestExportBatchCancelCancelsTheRest(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	env := newBatchEnv(t, 10, Limits{MaxExports: 2}, func(ctx context.Context, _ ExportInput) error {
		close(started)
		<-ctx.Done()

		return ctx.Err()
	})
	ctx := renderCtx()

	job, err := env.s.ExportBatch(ctx, ExportBatchRequest{
		ProjectID: batchProject, Items: batchItems("tiktok", "youtube-shorts", "x-video"),
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)

	go func() { done <- env.s.RunJob(context.Background(), job.JobID) }()

	select {
	case <-started:
	case <-time.After(20 * time.Second):
		t.Fatal("the first export never started")
	}

	if _, err := env.s.JobCancel(ctx, JobCancelRequest{JobID: job.JobID}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the batch did not end after its cancellation")
	}

	got, _ := env.s.JobGet(ctx, JobGetRequest{JobID: job.JobID})
	if got.State != JobCancelled {
		t.Fatalf("the parent ends %s: %+v", got.State, got.Error)
	}

	for i, it := range got.Result.Batch.Items {
		if it.State != JobCancelled {
			t.Errorf("item %d is %s, want cancelled", i, it.State)
		}
	}

	if env.calls.Load() != 1 {
		t.Errorf("the exporter ran %d times, want only the first", env.calls.Load())
	}

	if _, ok := env.jobs.states()[batchChildID(job.JobID, 1)]; ok {
		t.Error("a child was made after the cancellation")
	}
}

// TestExportBatchResumesAfterARestart: a parent that starts again skips the children that are final and runs
// the others, so a restart never exports a file twice.
func TestExportBatchResumesAfterARestart(t *testing.T) {
	t.Parallel()

	env := newBatchEnv(t, 10, Limits{MaxExports: 2}, nil)

	job, err := env.s.ExportBatch(renderCtx(), ExportBatchRequest{
		ProjectID: batchProject, Items: batchItems("tiktok", "youtube-shorts"),
	})
	if err != nil {
		t.Fatal(err)
	}

	// The first child already ended before the restart.
	first := JobRow{
		ID: batchChildID(job.JobID, 0), Kind: JobExport, Actor: "local:ui", State: JobQueued, Input: []byte("{}"),
	}
	first.ProjectID = job.ProjectID

	if err := env.jobs.CreateLimited(context.Background(), first, nil); err != nil {
		t.Fatal(err)
	}

	_, _ = env.jobs.Claim(context.Background(), first.ID, 1)
	_, _ = env.jobs.Finish(context.Background(), first.ID, JobSucceeded, nil, nil, 2)

	if err := env.s.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	got, _ := env.s.JobGet(renderCtx(), JobGetRequest{JobID: job.JobID})
	if got.State != JobSucceeded || got.Result.Batch.Succeeded != 2 {
		t.Fatalf("batch %s %+v %+v", got.State, got.Result.Batch, got.Error)
	}

	if env.calls.Load() != 1 {
		t.Errorf("the exporter ran %d times, want 1 (the finished child is not run again)", env.calls.Load())
	}
}

// TestExportBatchRefusesWhatCannotRun: the start checks every item before it makes a job, with the path of the
// item, and the limits of a batch.
func TestExportBatchRefusesWhatCannotRun(t *testing.T) {
	t.Parallel()

	env := newBatchEnv(t, 10, Limits{}, nil)
	ctx := renderCtx()

	many := make([]ExportBatchItem, MaxBatchItems+1)
	for i := range many {
		many[i] = ExportBatchItem{Preset: "tiktok"}
	}

	cases := []struct {
		name  string
		items []ExportBatchItem
		path  string
	}{
		{"no item", nil, "items"},
		{"too many", many, "items"},
		{"unknown preset", []ExportBatchItem{{Preset: "tiktok"}, {Preset: "myspace"}}, "items[1].preset"},
		{"unknown format", []ExportBatchItem{{Format: "avi"}}, "items[0].format"},
		{"png", []ExportBatchItem{{Format: "png"}}, "items[0].format"},
		{"preset with gif", []ExportBatchItem{{Preset: "tiktok", Format: "gif"}}, "items[0].preset"},
		{"codec with gif", []ExportBatchItem{{Codec: "hevc", Format: "gif"}}, "items[0].codec"},
		{"unknown codec", []ExportBatchItem{{Preset: "tiktok", Codec: "libx265"}}, "items[0].codec"},
		{"range outside", []ExportBatchItem{{Preset: "tiktok", Start: "1000s"}}, "items[0]"},
		{"bad start", []ExportBatchItem{{Preset: "tiktok", Start: "1h"}}, "items[0].start"},
		{"bad time", []ExportBatchItem{{Preset: "tiktok", End: "soon"}}, "items[0].end"},
	}

	for _, c := range cases {
		_, err := env.s.ExportBatch(ctx, ExportBatchRequest{ProjectID: batchProject, Items: c.items})

		var se *Error
		if !errors.As(err, &se) || se.Code != command.CodeValidation && se.Code != command.CodeInvalidTime {
			t.Errorf("%s: %v", c.name, err)

			continue
		}

		if se.Path != c.path {
			t.Errorf("%s: path %q, want %q (%s)", c.name, se.Path, c.path, se.Message)
		}
	}

	if len(env.jobs.states()) != 0 {
		t.Errorf("a refused batch made jobs: %v", env.jobs.states())
	}

	// A caller without the render scope is refused.
	read := WithCaller(context.Background(), Caller{Actor: "local:ui", Scopes: []string{ScopeRead}})
	if _, err := env.s.ExportBatch(
		read,
		ExportBatchRequest{ProjectID: batchProject, Items: batchItems("tiktok")},
	); err == nil {
		t.Error("a read caller started a batch")
	}
}

// TestExportBatchChildID: the identifier of a child is a function of the parent and the position, in the form
// of every job identifier.
func TestExportBatchChildID(t *testing.T) {
	t.Parallel()

	re := regexp.MustCompile(`^job_[0-9a-hjkmnp-tv-z]{10}$`)
	seen := map[string]bool{}

	for _, parent := range []string{"job_aaaaaaaaaa", "job_bbbbbbbbbb"} {
		for i := range MaxBatchItems {
			id := batchChildID(parent, i)
			if !re.MatchString(id) || seen[id] || id != batchChildID(parent, i) {
				t.Errorf("%s/%d: %q", parent, i, id)
			}

			seen[id] = true
		}
	}
}

// TestExportBatchLimitsTheParents: too many batches waiting at once are E_QUEUE_FULL, with a hint that names
// batches and not the exports.
func TestExportBatchLimitsTheParents(t *testing.T) {
	t.Parallel()

	env := newBatchEnv(t, 10, Limits{}, nil)
	ctx := renderCtx()

	for range maxActiveBatchesPerActor {
		if _, err := env.s.ExportBatch(
			ctx,
			ExportBatchRequest{ProjectID: batchProject, Items: batchItems("tiktok")},
		); err != nil {
			t.Fatal(err)
		}
	}

	_, err := env.s.ExportBatch(ctx, ExportBatchRequest{ProjectID: batchProject, Items: batchItems("tiktok")})

	var se *Error
	if !errors.As(err, &se) || se.Code != CodeQueueFull || !strings.Contains(se.Hint, "batch") {
		t.Fatalf("the fourth batch: %v", err)
	}
}

// TestExportBatchChildrenCarryThePresetAndCodec: the child export job holds the preset and the codec of its item,
// and the exporter receives them.
func TestExportBatchChildrenCarryThePresetAndCodec(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var got []string

	env := newBatchEnv(t, 10, Limits{MaxExports: 2}, func(_ context.Context, in ExportInput) error {
		mu.Lock()
		defer mu.Unlock()

		got = append(got, in.Preset+"/"+in.Codec+"/"+in.Format)

		return nil
	})

	job, err := env.s.ExportBatch(renderCtx(), ExportBatchRequest{ProjectID: batchProject, Items: []ExportBatchItem{
		{Preset: "tiktok", Codec: "hevc"}, {Preset: "x-video"}, {Format: "gif"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	if err := env.s.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	if want := "tiktok/hevc/mp4 x-video//mp4 //gif"; strings.Join(got, " ") != want {
		t.Errorf("the exporter got %q, want %q", strings.Join(got, " "), want)
	}
}
