package servicetest

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

// fakeCapture plays the capture helper: no process starts, the test drives the events.
type fakeCapture struct {
	mu       sync.Mutex
	starts   atomic.Int64
	sessions []*fakeSession
	startErr error
	// inspect says what the file holds; the default is a file of 2 s.
	inspect func(path string) (service.CaptureInfo, error)
}

type fakeSession struct {
	out     string
	events  chan capturehost.Event
	stopped atomic.Bool
	killed  atomic.Bool
	once    sync.Once
}

func (f *fakeCapture) Sources(context.Context) (capturehost.Sources, error) {
	return capturehost.Sources{
		Displays: []capturehost.Source{{ID: "d1", Name: "Display 1", Width: 1920, Height: 1080}},
		Windows:  []capturehost.Source{{ID: "w9", Name: "Doc\x1b[31m title", Width: 800, Height: 600}},
	}, nil
}

func (f *fakeCapture) Start(_ context.Context, r service.CaptureStart) (service.CaptureSession, error) {
	f.starts.Add(1)

	if f.startErr != nil {
		return nil, f.startErr
	}

	// the file the helper would be writing
	if err := os.WriteFile(r.Output, []byte("fragmented mp4 bytes"), 0o600); err != nil {
		return nil, err
	}

	s := &fakeSession{out: r.Output, events: make(chan capturehost.Event, 64)}

	f.mu.Lock()
	f.sessions = append(f.sessions, s)
	f.mu.Unlock()

	return s, nil
}

func (f *fakeCapture) Inspect(_ context.Context, path string) (service.CaptureInfo, error) {
	if f.inspect != nil {
		return f.inspect(path)
	}

	if _, err := os.Stat(path); err != nil {
		return service.CaptureInfo{}, err
	}

	return service.CaptureInfo{DurationMS: 2000, Width: 320, Height: 240}, nil
}

func (f *fakeCapture) last() *fakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.sessions[len(f.sessions)-1]
}

func (s *fakeSession) Started() capturehost.Started {
	return capturehost.Started{Width: 320, Height: 240, FPS: 30}
}
func (s *fakeSession) Encoder() string                  { return "h264_nvenc" }
func (s *fakeSession) Events() <-chan capturehost.Event { return s.events }
func (s *fakeSession) Close() error                     { return nil }

func (s *fakeSession) end(e capturehost.Event) {
	s.once.Do(func() {
		s.events <- e

		close(s.events)
	})
}

// Stop is a graceful end: the helper finishes the file and says so.
func (s *fakeSession) Stop() error {
	s.stopped.Store(true)
	s.end(capturehost.Finished{Reason: "stopped", Frames: 60, Dropped: 1, DurationMS: 2000, Bytes: 20})

	return nil
}

func (s *fakeSession) Kill() {
	s.killed.Store(true)
	s.end(capturehost.Lost{Err: capturehost.ErrLost})
}

func personCtx() context.Context {
	return service.WithCaller(
		context.Background(), service.Caller{Actor: ui, Scopes: service.AllScopes, Browser: true},
	)
}

type recEnv struct {
	*mediaEnv
	cap *fakeCapture
}

func newRecEnv(t *testing.T) *recEnv {
	t.Helper()

	r := &recEnv{cap: &fakeCapture{}}
	r.mediaEnv = newMediaEnv(t, mediaCfg{with: func(_ *env, o *service.Options) {
		o.Capture = r.cap
		o.Limits.MinFreeBytes = 1 << 20
	}})

	return r
}

func (r *recEnv) start(t *testing.T, project string) *service.Recording {
	t.Helper()

	rec, err := r.svc.RecordingStart(personCtx(), service.RecordingStartRequest{
		ProjectID: project, Source: service.RecordingSource{Kind: "window", ID: "w9", Name: "My doc"}, FPS: 30,
	})
	if err != nil {
		t.Fatalf("RecordingStart: %v", err)
	}

	// A test that leaves the recording running ends it before the workspace goes, and waits for the service to
	// settle the row, so no goroutine of the service is writing while the folder is removed.
	s := r.cap.last()

	t.Cleanup(func() {
		s.end(capturehost.Lost{Err: capturehost.ErrLost})

		deadline := time.Now().Add(5 * time.Second)

		for time.Now().Before(deadline) {
			if open, oerr := r.store.Recordings().Open(context.Background()); oerr == nil && len(open) == 0 {
				return
			}

			time.Sleep(10 * time.Millisecond)
		}
	})

	return rec
}

func (r *recEnv) waitState(t *testing.T, id, want string) *service.Recording {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for {
		rec, err := r.svc.RecordingGet(personCtx(), service.RecordingRef{ID: id})
		if err != nil {
			t.Fatal(err)
		}

		if rec.State == want {
			return rec
		}

		if time.Now().After(deadline) {
			t.Fatalf("recording %s is %s, want %s", id, rec.State, want)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func TestRecordingRecordStopConfirmAndPlace(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")

	rec := r.start(t, project)
	if rec.State != "recording" || rec.Width != 320 || rec.Encoder != "h264_nvenc" {
		t.Fatalf("started = %+v", rec)
	}

	// an agent reads it and may stop it
	agentCtx := r.as(agent)

	got, err := r.svc.RecordingGet(agentCtx, service.RecordingRef{ID: rec.ID})
	if err != nil || got.State != "recording" {
		t.Fatalf("an agent reads a recording in progress: %+v %v", got, err)
	}

	if _, err := r.svc.RecordingStop(agentCtx, service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatalf("an agent may stop a capture: %v", err)
	}

	ready := r.waitState(t, rec.ID, "ready")
	if ready.Review != "pending" || ready.DurationMS != 2000 || ready.Frames != 60 || ready.StopReason != "stopped" {
		t.Fatalf("ready = %+v", ready)
	}

	// not placeable before a person adds it
	if _, err := r.svc.RecordingPlace(
		agentCtx,
		service.RecordingPlaceRequest{ID: rec.ID},
	); code(
		err,
	) != "E_INVALID_STATE" {
		t.Errorf("place before confirm: %v", err)
	}

	// an agent cannot add it
	if _, err := r.svc.RecordingConfirm(
		agentCtx,
		service.RecordingRef{ID: rec.ID},
	); code(
		err,
	) != "E_PRIVILEGED_SESSION" {
		t.Errorf("confirm by an agent: %v", err)
	}

	added, err := r.svc.RecordingConfirm(personCtx(), service.RecordingRef{ID: rec.ID})
	if err != nil {
		t.Fatal(err)
	}

	if added.Review != "added" || added.AssetID == "" {
		t.Fatalf("added = %+v", added)
	}

	if _, err := os.Stat(filepath.Join(r.ws, "recordings", rec.ID)); !os.IsNotExist(err) {
		t.Errorf("the recording own copy must go once it is in the content store: %v", err)
	}

	// the asset becomes ready and joins the document; then one batch places it
	var placed *service.EditResult

	deadline := time.Now().Add(10 * time.Second)

	for {
		placed, err = r.svc.RecordingPlace(agentCtx, service.RecordingPlaceRequest{ID: rec.ID})
		if err == nil {
			break
		}

		if code(err) != "E_ASSET_NOT_READY" || time.Now().After(deadline) {
			t.Fatalf("RecordingPlace: %v", err)
		}

		time.Sleep(20 * time.Millisecond)
	}

	if !placed.Applied || len(placed.Created) == 0 {
		t.Fatalf("placed = %+v", placed)
	}

	itemsOf := func() int {
		pg, err := r.svc.ProjectGet(agentCtx, service.ProjectGetRequest{ProjectID: project, Detail: "document"})
		if err != nil {
			t.Fatal(err)
		}

		return len(pg.Document.Items)
	}

	if itemsOf() != 1 {
		t.Fatalf("one batch must add one clip, the document has %d items", itemsOf())
	}

	if _, err := r.svc.Undo(agentCtx, service.UndoRequest{ProjectID: project}); err != nil {
		t.Fatalf("undo: %v", err)
	}

	if itemsOf() != 0 {
		t.Errorf("one undo must remove the clip, the document has %d items", itemsOf())
	}
}

func TestSecurity_T118_OnlyAPersonStartsACapture(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")

	req := service.RecordingStartRequest{
		ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"}, FPS: 30,
	}

	callers := map[string]context.Context{
		"the local token": service.WithCaller(context.Background(), service.Caller{
			Actor: "local:cli", Scopes: service.AllScopes,
		}),
		"an agent": r.as(agent),
		"an api key": service.WithCaller(
			context.Background(),
			service.Caller{Actor: "key:key_0000000001", Scopes: service.AllScopes},
		),
		"an mcp client": service.WithCaller(
			context.Background(),
			service.Caller{Actor: "local:mcp", Scopes: service.AllScopes},
		),
		"an operator": service.WithCaller(context.Background(), service.Caller{
			Actor: "local:cli", Scopes: service.AllScopes, Operator: true,
		}),
	}

	for name, ctx := range callers {
		if _, err := r.svc.RecordingStart(ctx, req); code(err) != "E_PRIVILEGED_SESSION" {
			t.Errorf("%s: err = %v, want E_PRIVILEGED_SESSION", name, err)
		}
	}

	if n := r.cap.starts.Load(); n != 0 {
		t.Fatalf("the helper was started %d times for callers that are not a person", n)
	}

	rows, err := r.store.Recordings().ListByProject(context.Background(), project)
	if err != nil || len(rows) != 0 {
		t.Fatalf("a refused start left rows: %v %v", rows, err)
	}

	// a person with the media scope does start it
	r.start(t, project)

	if r.cap.starts.Load() != 1 {
		t.Fatal("a person must be able to start")
	}
}

func TestSecurity_T118_NoApprovalRuleStartsACapture(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")

	// even a caller with every scope, in the name of a trusted agent, is refused: there is nothing to approve
	_, err := r.svc.RecordingStart(r.as("local:trusted-agent"), service.RecordingStartRequest{
		ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"},
	})
	if code(err) != "E_PRIVILEGED_SESSION" || r.cap.starts.Load() != 0 {
		t.Fatalf("err = %v, starts = %d", err, r.cap.starts.Load())
	}
}

func TestSecurity_T119_ADismissedRecordingIsInvisibleToAgents(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")
	rec := r.start(t, project)

	if _, err := r.svc.RecordingStop(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	r.waitState(t, rec.ID, "ready")

	// pending: an agent sees the state, never the bytes
	if got, err := r.svc.RecordingGet(
		r.as(agent),
		service.RecordingRef{ID: rec.ID},
	); err != nil ||
		got.Review != "pending" {
		t.Fatalf("an agent reads a pending recording: %+v %v", got, err)
	}

	if _, err := r.svc.RecordingDismiss(
		r.as(agent),
		service.RecordingRef{ID: rec.ID},
	); code(
		err,
	) != "E_PRIVILEGED_SESSION" {
		t.Fatalf("dismiss by an agent: %v", err)
	}

	if _, err := r.svc.RecordingDismiss(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	a := r.as(agent)

	if _, err := r.svc.RecordingGet(a, service.RecordingRef{ID: rec.ID}); code(err) != "E_RECORDING_NOT_FOUND" {
		t.Errorf("get of a dismissed recording by an agent: %v", err)
	}

	list, err := r.svc.RecordingList(a, service.RecordingListRequest{ProjectID: project})
	if err != nil || len(list.Recordings) != 0 {
		t.Errorf("list for an agent: %+v %v", list, err)
	}

	if _, err := r.svc.JobGet(
		a,
		service.JobGetRequest{JobID: rec.ID},
	); code(err) != "E_JOB_NOT_FOUND" &&
		code(err) != "E_NOT_FOUND" &&
		code(err) != "E_RECORDING_NOT_FOUND" {
		t.Errorf("job_get of a dismissed recording by an agent: %v", err)
	}

	if _, err := r.svc.RecordingCancel(a, service.RecordingRef{ID: rec.ID}); code(err) != "E_RECORDING_NOT_FOUND" {
		t.Errorf("cancel of a dismissed recording by an agent: %v", err)
	}

	if _, err := r.svc.RecordingPlace(
		a,
		service.RecordingPlaceRequest{ID: rec.ID},
	); code(
		err,
	) != "E_RECORDING_NOT_FOUND" {
		t.Errorf("place of a dismissed recording by an agent: %v", err)
	}

	// a person still sees it, and can add it later
	list, err = r.svc.RecordingList(personCtx(), service.RecordingListRequest{ProjectID: project})
	if err != nil || len(list.Recordings) != 1 || list.Recordings[0].Review != "dismissed" {
		t.Fatalf("list for a person: %+v %v", list, err)
	}

	if got, err := r.svc.RecordingConfirm(
		personCtx(),
		service.RecordingRef{ID: rec.ID},
	); err != nil ||
		got.Review != "added" {
		t.Fatalf("confirm after dismiss: %+v %v", got, err)
	}

	if got, err := r.svc.RecordingGet(a, service.RecordingRef{ID: rec.ID}); err != nil || got.Review != "added" {
		t.Errorf("an added recording is visible again: %+v %v", got, err)
	}
}

func TestRecordingDeleteOfAFinishedOneIsForAPerson(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")
	rec := r.start(t, project)

	if _, err := r.svc.RecordingStop(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	r.waitState(t, rec.ID, "ready")

	if _, err := r.svc.RecordingCancel(
		r.as(agent),
		service.RecordingRef{ID: rec.ID},
	); code(
		err,
	) != "E_PRIVILEGED_SESSION" {
		t.Fatalf("delete by an agent: %v", err)
	}

	got, err := r.svc.RecordingCancel(personCtx(), service.RecordingRef{ID: rec.ID})
	if err != nil || got.State != "cancelled" {
		t.Fatalf("delete by a person: %+v %v", got, err)
	}

	if _, err := os.Stat(filepath.Join(r.ws, "recordings", rec.ID)); !os.IsNotExist(err) {
		t.Errorf("the file must be removed: %v", err)
	}
}

func TestRecordingCancelDiscardsOneInProgress(t *testing.T) {
	r := newRecEnv(t)
	rec := r.start(t, r.newProject("p"))

	got, err := r.svc.RecordingCancel(r.as(agent), service.RecordingRef{ID: rec.ID})
	if err != nil || got.State != "cancelled" {
		t.Fatalf("cancel: %+v %v", got, err)
	}

	if !r.cap.last().killed.Load() {
		t.Error("a cancel kills the helper")
	}

	if _, err := os.Stat(filepath.Join(r.ws, "recordings", rec.ID)); !os.IsNotExist(err) {
		t.Errorf("a discarded recording leaves no file: %v", err)
	}
}

func TestRecordingOneAtATime(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")
	r.start(t, project)

	_, err := r.svc.RecordingStart(personCtx(), service.RecordingStartRequest{
		ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"},
	})
	if code(err) != "E_QUEUE_FULL" {
		t.Fatalf("a second start: %v", err)
	}
}

func TestRecordingStartRefusals(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")
	ctx := personCtx()

	bad := []service.RecordingStartRequest{
		{ProjectID: "nope", Source: service.RecordingSource{Kind: "display", ID: "d1"}},
		{ProjectID: project, Source: service.RecordingSource{Kind: "camera", ID: "d1"}},
		{ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d 1"}},
		{ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"}, FPS: 59},
	}
	for _, req := range bad {
		if _, err := r.svc.RecordingStart(ctx, req); code(err) != "E_VALIDATION" {
			t.Errorf("%+v: %v", req, err)
		}
	}

	if _, err := r.svc.RecordingStart(ctx, service.RecordingStartRequest{
		ProjectID: "prj_0000000099", Source: service.RecordingSource{Kind: "display", ID: "d1"},
	}); code(err) != "E_NOT_FOUND" {
		t.Errorf("an unknown project: %v", err)
	}

	// the disk is already under the limit: nothing starts
	r.free.Store(1 << 10)

	if _, err := r.svc.RecordingStart(ctx, service.RecordingStartRequest{
		ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"},
	}); code(err) != "E_LIMIT_EXCEEDED" {
		t.Errorf("a full disk: %v", err)
	}

	if r.cap.starts.Load() != 0 {
		t.Error("no helper may start with the disk under the limit")
	}
}

func TestRecordingUnavailableWhereThereIsNoCapture(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{with: func(_ *env, o *service.Options) { o.CaptureReason = "web" }})
	project := m.newProject("p")

	_, err := m.svc.RecordingStart(personCtx(), service.RecordingStartRequest{
		ProjectID: project, Source: service.RecordingSource{Kind: "display", ID: "d1"},
	})
	se := serviceError(t, err)

	if se.Code != "E_CAPTURE_UNAVAILABLE" || se.Details["reason"] != "web" {
		t.Fatalf("err = %+v", se)
	}

	if _, err := m.svc.RecordingSources(
		m.as(agent),
		service.RecordingSourcesRequest{},
	); code(
		err,
	) != "E_CAPTURE_UNAVAILABLE" {
		t.Errorf("sources: %v", err)
	}
}

func TestRecordingStartFailureLeavesAFailedRowAndNoFile(t *testing.T) {
	r := newRecEnv(t)
	project := r.newProject("p")
	r.cap.startErr = &capturehost.HelperError{Code: "source_gone", Message: "the window is not there"}

	_, err := r.svc.RecordingStart(personCtx(), service.RecordingStartRequest{
		ProjectID: project, Source: service.RecordingSource{Kind: "window", ID: "w1"},
	})
	if se := serviceError(t, err); se.Code != "E_CAPTURE_FAILED" || se.Details["reason"] != "source_gone" {
		t.Fatalf("err = %+v", se)
	}

	rows, _ := r.store.Recordings().ListByProject(context.Background(), project)
	if len(rows) != 1 || rows[0].State != "failed" {
		t.Fatalf("rows = %+v", rows)
	}

	if _, err := os.Stat(filepath.Join(r.ws, "recordings", rows[0].ID)); !os.IsNotExist(err) {
		t.Errorf("a failed start leaves no folder: %v", err)
	}

	// the next start is not blocked by it
	r.cap.startErr = nil
	r.start(t, project)
}

func TestSecurity_T121_ALimitStopsTheCaptureAndKeepsTheFile(t *testing.T) {
	r := newRecEnv(t)
	rec := r.start(t, r.newProject("p"))
	s := r.cap.last()

	s.events <- capturehost.Progress{ElapsedMS: 1000, Frames: 30, Bytes: 10}

	r.free.Store(1 << 10) // the disk fills while it records

	s.events <- capturehost.Progress{ElapsedMS: 2000, Frames: 60, Bytes: 20}

	got := r.waitState(t, rec.ID, "ready")
	if got.StopReason != "disk" || !s.stopped.Load() || s.killed.Load() {
		t.Fatalf("ready = %+v stopped=%v killed=%v", got, s.stopped.Load(), s.killed.Load())
	}

	if got.Review != "pending" {
		t.Errorf("what was written is kept for a person to decide: %+v", got)
	}
}

func TestRecordingDurationLimitStops(t *testing.T) {
	r := &recEnv{cap: &fakeCapture{}}
	r.mediaEnv = newMediaEnv(t, mediaCfg{with: func(_ *env, o *service.Options) {
		o.Capture = r.cap
		o.RecordingMaxSeconds = 1
	}})

	rec := r.start(t, r.newProject("p"))
	r.cap.last().events <- capturehost.Progress{ElapsedMS: 1000, Frames: 30, Bytes: 10}

	if got := r.waitState(t, rec.ID, "ready"); got.StopReason != "duration" {
		t.Fatalf("ready = %+v", got)
	}
}

func TestSecurity_T121_AHelperThatDiesLeavesWhatItWroteOrAReason(t *testing.T) {
	for name, tc := range map[string]struct {
		inspect func(string) (service.CaptureInfo, error)
		want    string
	}{
		"a file that plays is kept": {nil, "ready"},
		"a file with no frame is a failure": {
			func(string) (service.CaptureInfo, error) { return service.CaptureInfo{}, os.ErrInvalid }, "failed",
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRecEnv(t)
			r.cap.inspect = tc.inspect
			rec := r.start(t, r.newProject("p"))

			r.cap.last().Kill()
			r.cap.last().killed.Store(false)

			got := r.waitState(t, rec.ID, tc.want)
			if tc.want == "failed" && (got.Error == nil || got.Error.Code != "E_CAPTURE_FAILED") {
				t.Errorf("a failure carries its reason: %+v", got)
			}

			if tc.want == "ready" && (got.StopReason != "recovered" || got.Review != "pending") {
				t.Errorf("a salvaged recording: %+v", got)
			}
		})
	}
}

// A recording the last process left open is settled when the service starts, and never stays recording.
func TestSecurity_T121_RecoveryNeverLeavesARecordingRunning(t *testing.T) {
	for name, tc := range map[string]struct {
		inspect func(string) (service.CaptureInfo, error)
		want    string
	}{
		"frames were written": {nil, "ready"},
		"nothing was written": {func(string) (service.CaptureInfo, error) { return service.CaptureInfo{}, os.ErrNotExist }, "failed"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRecEnv(t)
			r.cap.inspect = tc.inspect
			project := r.newProject("p")

			ids := map[string]string{}

			for _, state := range []string{"prepared", "recording", "finishing"} {
				id := "rec_" + map[string]string{"prepared": "0000000001", "recording": "0000000002", "finishing": "0000000003"}[state]
				ids[state] = id

				if err := r.store.Recordings().Insert(context.Background(), recordings.Row{
					ID: id, ProjectID: project, State: state, SourceKind: "display", FPS: 30, Actor: ui, CreatedMs: 1,
				}); err != nil {
					t.Fatal(err)
				}

				dir := filepath.Join(r.ws, "recordings", id)
				_ = os.MkdirAll(dir, 0o700)
				_ = os.WriteFile(filepath.Join(dir, "capture.mp4"), []byte("x"), 0o600)
			}

			runner, _ := service.RunnerOf(r.svc)
			if err := runner.Recover(context.Background()); err != nil {
				t.Fatal(err)
			}

			for state, id := range ids {
				got, err := r.svc.RecordingGet(personCtx(), service.RecordingRef{ID: id})
				if err != nil {
					t.Fatal(err)
				}

				if got.State != tc.want {
					t.Errorf("a recording left %s is %s, want %s", state, got.State, tc.want)
				}

				if tc.want == "ready" && (got.Review != "pending" || got.StopReason != "recovered") {
					t.Errorf("%s: %+v", state, got)
				}

				if tc.want == "failed" && (got.Error == nil || got.Error.Code != "E_CAPTURE_FAILED") {
					t.Errorf("%s: a failure carries a reason: %+v", state, got)
				}
			}

			open, err := r.store.Recordings().Open(context.Background())
			if err != nil || len(open) != 0 {
				t.Errorf("open recordings after recovery: %v %v", open, err)
			}
		})
	}
}

func TestRecordingIsAJobOfKindCaptureForMCP(t *testing.T) {
	r := newRecEnv(t)
	rec := r.start(t, r.newProject("p"))
	a := r.as(agent)

	job, err := r.svc.JobGet(a, service.JobGetRequest{JobID: rec.ID})
	if err != nil || job.Kind != "capture" || job.State != service.JobRunning || job.ProjectID == nil {
		t.Fatalf("job = %+v %v", job, err)
	}

	job, err = r.svc.JobCancel(a, service.JobCancelRequest{JobID: rec.ID})
	if err != nil || job.Kind != "capture" {
		t.Fatalf("job_cancel = %+v %v", job, err)
	}

	r.waitState(t, rec.ID, "ready")

	job, err = r.svc.JobGet(a, service.JobGetRequest{JobID: rec.ID})
	if err != nil || job.State != service.JobSucceeded || job.Progress != 1 {
		t.Fatalf("job after the stop = %+v %v", job, err)
	}

	// a key with only the render scope cannot stop a capture through job_cancel
	renderOnly := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:key_0000000002", Scopes: []string{service.ScopeRender}},
	)
	if _, err := r.svc.JobCancel(renderOnly, service.JobCancelRequest{JobID: rec.ID}); code(err) != "E_FORBIDDEN" {
		t.Errorf("render only: %v", err)
	}
}

func TestRecordingSourcesAreCleaned(t *testing.T) {
	r := newRecEnv(t)

	got, err := r.svc.RecordingSources(r.as(agent), service.RecordingSourcesRequest{})
	if err != nil || len(got.Displays) != 1 || len(got.Windows) != 1 {
		t.Fatalf("sources = %+v %v", got, err)
	}

	for _, r := range got.Windows[0].Name {
		if r < 0x20 {
			t.Errorf("a control character reached the surface: %q", got.Windows[0].Name)
		}
	}
}

// The clock of a recording that runs is the one the helper reported a moment ago, not the one saved to the
// database every few seconds.
func TestRecordingShowsTheClockOfTheHelperWhileItRuns(t *testing.T) {
	r := newRecEnv(t)
	rec := r.start(t, r.newProject("p"))

	r.cap.last().events <- capturehost.Progress{ElapsedMS: 3000, Frames: 90, Dropped: 2, Bytes: 5000}

	deadline := time.Now().Add(5 * time.Second)

	for {
		got, err := r.svc.RecordingGet(r.as(agent), service.RecordingRef{ID: rec.ID})
		if err != nil {
			t.Fatal(err)
		}

		if got.State == "recording" && got.DurationMS == 3000 && got.Frames == 90 && got.Dropped == 2 {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("the live view never showed the progress: %+v", got)
		}

		time.Sleep(10 * time.Millisecond)
	}
}
