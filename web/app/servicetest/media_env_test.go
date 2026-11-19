package servicetest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/cache"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/frame"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/engine/transcribe"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/exporter"
	"github.com/AlexGalhardo/stuarty/service/pathguard"
	"github.com/AlexGalhardo/stuarty/web/app/jobs"

	"github.com/AlexGalhardo/vukem/queue"
)

// lazyRunner lets the queue handlers exist before the service that they call.
type lazyRunner struct {
	mu sync.Mutex
	r  service.JobRunner
}

func (l *lazyRunner) get() service.JobRunner {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.r
}

func (l *lazyRunner) RunJob(ctx context.Context, id string) error { return l.get().RunJob(ctx, id) }
func (l *lazyRunner) Recover(ctx context.Context) error           { return l.get().Recover(ctx) }
func (l *lazyRunner) Cancel(ctx context.Context, id string) error { return l.get().Cancel(ctx, id) }

type mediaCfg struct {
	ingest   service.Ingester
	exporter service.Exporter
	// noQueue leaves the jobs with no worker: the rows are created and stay queued.
	noQueue bool
	limits  service.Limits
	guard   *pathguard.Config
	// realIngest runs engine/ingest instead of the fake; with no FFmpeg installed it
	// still refuses what is not media, which is decided by the first bytes.
	realIngest bool
	// realExport renders with engine/export and the pinned FFmpeg; the test skips without it.
	realExport bool
	// onEvent runs after each published event, on the goroutine that published it.
	onEvent func(service.Event)
	// durable runs the jobs on the database queue with workers instead of inline.
	durable bool
	// tools is the install step of the helper.
	tools *service.Tools
	// transcriber is the provider of the transcribe job.
	transcriber transcribe.Provider
	// wrapStore puts a decorator between the service and its store.
	wrapStore func(service.Store) service.Store
	// log receives the job lines of the service.
	log *slog.Logger
	// jobWait is how long TrashEmpty waits for a job to stop; zero is the default.
	jobWait time.Duration
}

// mediaEnv is an env whose service has the media and job machinery, with a
// fake ingest and a fake exporter unless the test brings its own.
type mediaEnv struct {
	*env
	ws      string
	runner  service.JobRunner
	queue   *queue.Queue
	ingests atomic.Int64
	// free is what the volume of the workspace reports as free, in bytes.
	free atomic.Uint64
	// hold makes the queue drop what is enqueued, as a crash between the commit
	// and the delivery would.
	hold  atomic.Bool
	files string
}

func newMediaEnv(tb testing.TB, cfg mediaCfg) *mediaEnv {
	tb.Helper()

	m := &mediaEnv{files: tb.TempDir()}
	m.free.Store(1 << 40)
	lazy := &lazyRunner{}

	if cfg.ingest == nil {
		cfg.ingest = m.fakeIngest
	}

	m.env = newEnvWith(tb, func(e *env, o *service.Options) {
		m.ws = string(e.dir)

		gc := pathguard.Config{Workspace: m.ws}

		if cfg.guard != nil {
			gc = *cfg.guard
			gc.Workspace = m.ws
		}

		guard, err := pathguard.New(gc)
		if err != nil {
			tb.Fatal(err)
		}

		if cfg.wrapStore != nil {
			o.Store = cfg.wrapStore(o.Store)
		}

		o.Transcriber = cfg.transcriber
		o.Log = cfg.log
		o.JobWait = cfg.jobWait
		o.Tools = cfg.tools
		o.Workspace = m.ws
		o.Media = service.Media{
			Content: enginestore.New(m.ws), Guard: guard, Ingest: cfg.ingest,
			FreeBytes: func(string) (uint64, error) { return m.free.Load(), nil },
		}
		if cfg.realIngest {
			runner, rerr := ffmpeg.NewRunner(&ffmpeg.Installation{}, m.ws)
			if rerr != nil {
				tb.Fatal(rerr)
			}

			o.Media.Runner, o.Media.Ingest = runner, ingest.Run
		}

		o.Exporter = cfg.exporter

		if cfg.onEvent != nil {
			record := o.Publish

			o.Publish = func(ev service.Event) {
				record(ev)
				cfg.onEvent(ev)
			}
		}

		if cfg.realExport {
			runner, rerr := ffmpeg.NewRunner(pinnedFFmpeg(tb), m.ws)
			if rerr != nil {
				tb.Fatal(rerr)
			}

			o.Media.Runner = runner

			frames, cerr := cache.Open(m.ws, 0)
			if cerr != nil {
				tb.Fatal(cerr)
			}

			o.Media.Frames = &frame.Frames{
				Runner:       runner,
				ArtifactsDir: filepath.Join(m.ws, "artifacts"),
				Cache:        frames,
			}
			o.Exporter = exporter.New(exporter.Config{Runner: runner, Workspace: m.ws})
		}
		o.Limits = cfg.limits

		if cfg.noQueue {
			return
		}

		var driver queue.Driver = queue.Sync{}

		if cfg.durable {
			db, err := queue.NewDatabase(
				e.store.DB(),
				queue.DatabaseOptions{Table: "queue_jobs", FailedTable: "queue_failed_jobs"},
			)
			if err != nil {
				tb.Fatal(err)
			}

			driver = db
		}

		m.queue, err = jobs.NewQueue(driver, lazy)
		if err != nil {
			tb.Fatal(err)
		}

		o.Enqueue = gated{inner: jobs.Dispatcher{Queue: m.queue}, hold: &m.hold}
	})

	r, ok := service.RunnerOf(m.svc)
	if !ok {
		tb.Fatal("the service has no job runner")
	}

	lazy.mu.Lock()
	lazy.r, m.runner = r, r
	lazy.mu.Unlock()

	return m
}

// write makes a file of the media directory and returns its path.
func (m *mediaEnv) write(name, content string) string {
	m.t.Helper()

	p := filepath.Join(m.files, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		m.t.Fatal(err)
	}

	return p
}

// fakeIngest is an ingest that produces the artifacts of a 2 s video with audio
// without FFmpeg. It counts its calls.
func (m *mediaEnv) fakeIngest(_ context.Context, in ingest.Input) (*ingest.Result, error) {
	m.ingests.Add(1)

	return fakeResult(m.ws, in)
}

func fakeResult(ws string, in ingest.Input) (*ingest.Result, error) {
	dir := filepath.Join(ws, "artifacts", in.Hash)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	sum := &ingest.Summary{
		Version: 1, Kind: ingest.KindVideo, Container: "mov,mp4", SizeBytes: 1000, DurationMS: 2000,
		Video: &ingest.VideoFacts{
			Codec: "h264", DisplayWidth: 320, DisplayHeight: 240, NominalFPS: ingest.Rational{Num: 30, Den: 1},
		},
		Audio: &ingest.AudioFacts{Codec: "aac", SampleRate: 48000, Channels: 2},
	}

	// The shape engine/ingest writes (SDD-011 section 3.2): the summary next to the raw ffprobe JSON.
	probe, err := json.Marshal(struct {
		Summary *ingest.Summary `json:"summary"`
		FFprobe json.RawMessage `json:"ffprobe"`
	}{sum, json.RawMessage(`{"format":{"filename":""}}`)})
	if err != nil {
		return nil, err
	}
	silence := `{"version":1,"sample_rate":48000,"sample_count":96000,"min_duration_ms":150,"sets":[` +
		`{"threshold_db":-35,"ranges":[{"start":24000,"end":48000}]}]}`
	loud := `{"version":1,"integrated_lufs":-17.2,"lra_lu":5.1,"true_peak_dbtp":-1.4,"silent":false,"sample_count":96000}`

	files := []struct {
		kind, variant, ext, body string
		profile                  int
		meta                     any
	}{
		{"probe", "", "json", string(probe), 1, nil},
		{"normalized", "", "mov", "normalized", 1, nil},
		{"proxy", "", "mp4", "proxy-bytes", 1, nil},
		{"chunks", "", "json", `{"version":1,"codec":"vp09.00.31.08.00.01.01.01.00"}`, 1, nil},
		{
			"thumbnails",
			"",
			"jpg",
			"sheet",
			1,
			ingest.ThumbLayout{TileW: 90, TileH: 160, Columns: 2, Count: 2, IntervalMS: 1000},
		},
		{"thumbnails", "w320", "jpg", "thumb320", 1, nil},
		{"peaks", "", "bin", "peaks", 1, nil},
		{"silence", "", "json", silence, 1, nil},
		{"loudness", "", "json", loud, 1, nil},
	}

	// The service asks for the sound maps (RF-043, RF-044); the real ingest then adds both.
	if in.SoundMaps {
		files = append(files,
			struct {
				kind, variant, ext, body string
				profile                  int
				meta                     any
			}{"speech", "", "json", `{"version":1,"sample_rate":48000,"sample_count":96000,"source":"envelope",` +
				`"threshold_db":-35,"ranges":[{"start":0,"end":24000},{"start":48000,"end":96000}]}`, 1, nil},
			struct {
				kind, variant, ext, body string
				profile                  int
				meta                     any
			}{"beats", "", "json", `{"version":1,"sample_rate":48000,"sample_count":96000,"tempo_bpm":120,` +
				`"confidence":1,"beats":[0,24000,48000,72000]}`, 1, nil},
		)
	}

	res := &ingest.Result{
		Kind: ingest.KindVideo, Summary: sum, TimingsMS: map[string]int64{"probe": 1},
		Facts: ingest.Facts{
			FrameRate: ingest.Rational{Num: 30, Den: 1}, FrameCount: 60, SampleCount: 96000, Width: 320, Height: 240,
			HasAudio: true,
		},
	}

	for _, f := range files {
		name := ingest.ArtifactName(f.kind, f.profile, f.variant, f.ext)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(f.body), 0o600); err != nil {
			return nil, err
		}

		sum := sha256.Sum256([]byte(f.body))
		res.Artifacts = append(res.Artifacts, ingest.Artifact{
			Kind: f.kind, Profile: f.profile, Variant: f.variant, Ext: f.ext,
			RelPath: "artifacts/" + in.Hash + "/" + name, Size: int64(len(f.body)), SHA256: hex.EncodeToString(sum[:]),
			Meta: f.meta,
		})
	}

	return res, nil
}

// eventsOf copies the events published so far.
func (e *env) eventsOf(typ string) []service.Event {
	e.mu.Lock()
	defer e.mu.Unlock()

	var out []service.Event

	for i := range e.events {
		if e.events[i].Type == typ {
			out = append(out, e.events[i])
		}
	}

	return out
}

// eventually polls cond for up to 5 s.
func eventually(tb testing.TB, what string, cond func() bool) {
	tb.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for !cond() {
		if time.Now().After(deadline) {
			tb.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// gated is an Enqueuer that can be told to lose its messages.
type gated struct {
	inner service.Enqueuer
	hold  *atomic.Bool
}

func (g gated) Enqueue(ctx context.Context, tx service.Tx, kind, jobID string) error {
	if g.hold.Load() {
		return nil
	}

	return g.inner.Enqueue(ctx, tx, kind, jobID)
}

// pinnedFFmpeg finds the pinned LGPL FFmpeg of scripts/install-ffmpeg.sh, or
// skips the test; a test never downloads.
func pinnedFFmpeg(tb testing.TB) *ffmpeg.Installation {
	tb.Helper()

	exe := "ffmpeg"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}

	dir := os.Getenv("STUARTY_TEST_FFMPEG_DIR")

	if dir == "" {
		for _, base := range []string{os.Getenv("RUNNER_TEMP"), os.Getenv("TEMP"), os.Getenv("TMPDIR")} {
			if base == "" {
				continue
			}

			if m, _ := filepath.Glob(filepath.Join(base, "ffmpeg", "*", "*", "bin", exe)); len(m) > 0 {
				dir = filepath.Dir(m[len(m)-1])

				break
			}
		}
	}

	if dir == "" {
		tb.Skip("no pinned FFmpeg: run scripts/install-ffmpeg.sh and set STUARTY_TEST_FFMPEG_DIR")
	}

	in, err := ffmpeg.Discover(
		context.Background(),
		ffmpeg.Config{FFmpegPath: filepath.Join(dir, exe), CPUCount: runtime.NumCPU()},
	)
	if err != nil || in.Info.License.Name != "lgpl" {
		tb.Skip("the FFmpeg in STUARTY_TEST_FFMPEG_DIR is not the pinned LGPL build")
	}

	return in
}
