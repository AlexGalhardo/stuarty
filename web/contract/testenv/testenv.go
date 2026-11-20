// Package testenv builds a real service.Service over a real SQLite store in a
// temporary workspace, with the media and job machinery wired to fakes that
// need no FFmpeg: an ingest that writes the artifacts of a 2 s video with audio
// and an exporter that writes a small file. The adapters of web/app and the
// contract tests of web/contract use it; it is not part of any binary.
package testenv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/cache"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/frame"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/engine/transcribe"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/pathguard"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/vault"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/jobs"
	"github.com/AlexGalhardo/stuarty/web/app/store"

	"github.com/AlexGalhardo/vukem/queue"
)

// ExportFunc renders one export for the fake exporter.
type ExportFunc func(ctx context.Context, in service.ExportInput) (*service.ExportOutput, error)

// Export implements service.Exporter.
func (f ExportFunc) Export(ctx context.Context, in service.ExportInput) (*service.ExportOutput, error) {
	return f(ctx, in)
}

// Options tune the environment.
type Options struct {
	// Export replaces the exporter; the default writes a small file and passes
	// every verification check.
	Export ExportFunc
	// Clock is the service clock; default time.Now.
	Clock func() time.Time
	// Frames wires the real frame renderer of view over the pinned FFmpeg; the
	// test is skipped when that build is not installed.
	Frames bool
	// Limits are the ceilings of the service; zero fields keep the defaults.
	Limits service.Limits
	// Transcriber is the transcription provider of the transcribe job; nil
	// leaves transcription unavailable. StubTranscriber needs no helper.
	Transcriber transcribe.Provider
	// DurationMS is the length of every imported file as the fake ingest
	// reports it; default 2000.
	DurationMS int64
	// SilenceBlocks makes the fake silence the ranges the real analysis reports for
	// a silence from 0.5 s to 1 s (it works in blocks of 512 samples, so the range
	// is 24576 to 48128 and not 24000 to 48000), for a run that must match the real
	// binary on a real clip.
	SilenceBlocks bool
	// MasterKey gives the workspace a vault whose master key is this one (32 bytes), so a test can
	// create connections; nil leaves a vault with no key source.
	MasterKey []byte
	// Providers are the AI providers to register (SDD-016); they need MasterKey, since a provider is reached
	// through a connection.
	Providers []providers.Provider
	// Social is the registry of the social network connectors (SDD-020); it needs MasterKey, since an account
	// is reached through a connection.
	Social *social.Registry
}

// Env is a service with a worker running behind it.
type Env struct {
	Service service.Service
	Dir     workspace.Dir
	// Store is the SQLite store under the service.
	Store *store.Store
	// Files is a directory for the media files of the test.
	Files      string
	durationMS int64
	silence    string
	// Ingests counts the calls of the fake ingest.
	Ingests atomic.Int64

	ids atomic.Int64
}

// New builds the environment and starts a queue worker that stops with the test.
func New(tb testing.TB, o Options) *Env {
	tb.Helper()

	e := &Env{Files: tb.TempDir(), durationMS: o.DurationMS, silence: `{"start":24000,"end":48000}`}
	if o.SilenceBlocks {
		e.silence = `{"start":24576,"end":48128}`
	}

	if e.durationMS == 0 {
		e.durationMS = 2000
	}

	dir, err := workspace.Resolve(func(string) string { return filepath.Join(tb.TempDir(), "ws") })
	if err != nil {
		tb.Fatal(err)
	}

	if err := dir.EnsureLayout(); err != nil {
		tb.Fatal(err)
	}

	e.Dir = dir

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = st.Close() })

	e.Store = st

	guard, err := pathguard.New(pathguard.Config{Workspace: string(dir)})
	if err != nil {
		tb.Fatal(err)
	}

	runner := &lazyRunner{}

	db, err := queue.NewDatabase(st.DB(), queue.DatabaseOptions{Table: "queue_jobs", FailedTable: "queue_failed_jobs"})
	if err != nil {
		tb.Fatal(err)
	}

	q, err := jobs.NewQueue(db, runner)
	if err != nil {
		tb.Fatal(err)
	}

	exporter := o.Export
	if exporter == nil {
		exporter = GoodExport
	}

	media := service.Media{
		Content: enginestore.New(string(dir)), Guard: guard, Ingest: e.fakeIngest,
		FreeBytes: func(string) (uint64, error) { return 1 << 40, nil },
	}

	if o.Frames {
		media.Frames = realFrames(tb, string(dir))
	}

	var conns *connections.Manager
	if o.MasterKey != nil {
		conns = connections.NewManager(connections.Options{
			Store: st.Connections(), Now: o.Clock,
			Vault: vault.Open(filepath.Join(string(dir), connections.VaultFile), vault.NewMemoryProvider(o.MasterKey)),
		})
	}

	var reg *providers.Registry

	if len(o.Providers) > 0 {
		reg = providers.NewRegistry(
			conns, providers.NewPriceBook(filepath.Join(string(dir), "providers", providers.PriceFile)), o.Clock,
		)

		for _, p := range o.Providers {
			if err := reg.Register(p); err != nil {
				tb.Fatal(err)
			}
		}
	}

	svc, err := service.New(service.Options{
		Connections: conns, Providers: reg, Social: o.Social,
		Store: st, Registry: command.Default(), Now: o.Clock, NewIDs: e.newID,
		Workspace:   string(dir),
		Limits:      o.Limits,
		Transcriber: o.Transcriber,
		Media:       media,
		Exporter:    exporter,
		Lint:        lint.Env{Measurer: rasterMeasurer{workspace: string(dir)}},
		Enqueue:     jobs.Dispatcher{Queue: q},
	})
	if err != nil {
		tb.Fatal(err)
	}

	jr, ok := service.RunnerOf(svc)
	if !ok {
		tb.Fatal("testenv: the service has no job runner")
	}

	runner.set(jr)

	e.Service = svc

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)

		_ = q.Work(ctx, queue.WorkOptions{
			Queues: []string{jobs.QueueIngest, jobs.QueueExport, jobs.QueueDefault}, Concurrency: 2,
			Sleep: 20 * time.Millisecond, StopTimeout: 5 * time.Second,
		})
	}()

	tb.Cleanup(func() {
		stop()
		<-done
	})

	return e
}

// newID is a valid identifier from a counter: digits are in the alphabet.
func (e *Env) newID(prefix string) string {
	return fmt.Sprintf("%s_%010d", prefix, e.ids.Add(1))
}

// Caller is the context of a local caller with every scope.
func Caller(actor string) context.Context {
	return service.WithCaller(context.Background(), service.Caller{Actor: actor, Scopes: service.AllScopes})
}

// WriteFile makes a file of the media directory and returns its absolute path.
func (e *Env) WriteFile(tb testing.TB, name, content string) string {
	tb.Helper()

	p := filepath.Join(e.Files, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		tb.Fatal(err)
	}

	return p
}

// GoodExport is the default exporter: the file under OutPath, then its facts.
func GoodExport(_ context.Context, in service.ExportInput) (*service.ExportOutput, error) {
	body := []byte("an mp4, more or less")
	if err := os.WriteFile(in.OutPath, body, 0o600); err != nil {
		return nil, err
	}

	sum := sha256.Sum256(body)

	return &service.ExportOutput{
		Bytes: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), Frames: int(in.End - in.Start),
		VideoCodec: "h264", AudioCodec: "aac",
		Checks: []service.VerifyCheck{
			{Name: "streams", OK: true, Detail: "video and audio"},
			{Name: "duration", OK: true, Detail: "within one frame"},
			{Name: "dimensions", OK: true, Detail: "1080x1920"},
			{Name: "pixel_format", OK: true, Detail: "yuv420p"},
			{Name: "faststart", OK: true, Detail: "moov before mdat"},
		},
	}, nil
}

// lazyRunner lets the queue handlers exist before the service that they call.
type lazyRunner struct {
	mu sync.Mutex
	r  service.JobRunner
}

func (l *lazyRunner) set(r service.JobRunner) {
	l.mu.Lock()
	l.r = r
	l.mu.Unlock()
}

func (l *lazyRunner) get() service.JobRunner {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.r
}

func (l *lazyRunner) RunJob(ctx context.Context, id string) error { return l.get().RunJob(ctx, id) }
func (l *lazyRunner) Recover(ctx context.Context) error           { return l.get().Recover(ctx) }
func (l *lazyRunner) Cancel(ctx context.Context, id string) error { return l.get().Cancel(ctx, id) }

// FakeMedia is the content of a test file that the fake ingest probes as a
// video of the given length and size instead of the default one; the ceilings of
// the service are then checked against it, as the probe of the real ingest does.
func FakeMedia(seconds int64, width, height int) string {
	return fmt.Sprintf("%s seconds=%d width=%d height=%d\n", fakeMediaMarker, seconds, width, height)
}

// FakeMediaTags is FakeMedia whose probe also reports the given metadata tags:
// each one is a `tag.<key>=<quoted value>` line after the header line, the value
// quoted as Go does so that control and bidirectional characters survive.
func FakeMediaTags(seconds int64, width, height int, tags map[string]string) string {
	var b strings.Builder

	b.WriteString(FakeMedia(seconds, width, height))

	for _, k := range slices.Sorted(maps.Keys(tags)) {
		fmt.Fprintf(&b, "tag.%s=%s\n", k, strconv.Quote(tags[k]))
	}

	return b.String()
}

const fakeMediaMarker = "fake-media"

// FakeImage is the content of a test file that the fake ingest probes as a still image of the given size, so an
// image import (epic 48) has a file that is an image.
func FakeImage(width, height int) string {
	return fmt.Sprintf("%s width=%d height=%d\n", fakeImageMarker, width, height)
}

const fakeImageMarker = "fake-image"

// fakeTags reads the tag lines of a FakeMediaTags file; a line that does not
// parse is ignored.
func fakeTags(content string) map[string]ingest.Tag {
	tags := map[string]ingest.Tag{}

	for line := range strings.SplitSeq(content, "\n") {
		rest, ok := strings.CutPrefix(line, "tag.")
		if !ok {
			continue
		}

		key, quoted, ok := strings.Cut(rest, "=")
		if !ok {
			continue
		}

		if value, err := strconv.Unquote(quoted); err == nil {
			tags[key] = ingest.Tag{Value: value}
		}
	}

	if len(tags) == 0 {
		return nil
	}

	return tags
}

func (e *Env) fakeIngest(_ context.Context, in ingest.Input) (*ingest.Result, error) {
	e.Ingests.Add(1)

	seconds, width, height := e.durationMS/1000, 320, 240
	if e.durationMS%1000 != 0 {
		seconds++
	}

	var tags map[string]ingest.Tag

	if path, err := enginestore.New(string(e.Dir)).Path(in.Hash); err == nil {
		head, rerr := readHead(path, 64<<10)

		switch {
		case rerr == nil && strings.HasPrefix(head, fakeImageMarker):
			var w, h int

			_, _ = fmt.Sscanf(head, fakeImageMarker+" width=%d height=%d", &w, &h)

			return &ingest.Result{
				Kind: ingest.KindImage, Summary: &ingest.Summary{Version: 1, Kind: ingest.KindImage},
				TimingsMS: map[string]int64{"probe": 1}, Facts: ingest.Facts{Width: w, Height: h},
			}, nil
		case rerr == nil && strings.HasPrefix(head, fakeMediaMarker):
			_, _ = fmt.Sscanf(head, fakeMediaMarker+" seconds=%d width=%d height=%d", &seconds, &width, &height)
			tags = fakeTags(head)
		}
	}

	probe := fmt.Sprintf(`{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","nb_streams":2,"duration":"%d.0"},`+
		`"streams":[{"index":0,"codec_type":"video","codec_name":"h264","width":%d,"height":%d},`+
		`{"index":1,"codec_type":"audio","codec_name":"aac"}]}`, seconds, width, height)
	if _, err := ffmpeg.CheckProbe([]byte(probe), in.Ceilings); err != nil {
		return nil, err
	}

	ms := max(e.durationMS, seconds*1000)

	return fakeResult(string(e.Dir), in, ms, tags, e.silence)
}

func readHead(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	defer func() { _ = f.Close() }()

	buf := make([]byte, n)
	got, err := f.Read(buf)

	if err != nil && got == 0 {
		return "", err
	}

	return string(buf[:got]), nil
}

func fakeResult(
	ws string, in ingest.Input, durationMS int64, tags map[string]ingest.Tag, silentRange string,
) (*ingest.Result, error) {
	dir := filepath.Join(ws, "artifacts", in.Hash)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	sum := &ingest.Summary{
		Version: 1, Kind: ingest.KindVideo, Container: "mov,mp4", SizeBytes: 1000, DurationMS: durationMS,
		Video: &ingest.VideoFacts{
			Codec: "h264", DisplayWidth: 320, DisplayHeight: 240, NominalFPS: ingest.Rational{Num: 30, Den: 1},
		},
		Audio: &ingest.AudioFacts{Codec: "aac", SampleRate: 48000, Channels: 2},
		Tags:  tags,
	}

	// The real ingest writes the summary next to the raw ffprobe output (SDD-011 section 3.2).
	probe, err := json.Marshal(map[string]any{"summary": sum, "ffprobe": json.RawMessage("{}")})
	if err != nil {
		return nil, err
	}

	samples := durationMS * 48

	silence := fmt.Sprintf(
		`{"version":1,"sample_rate":48000,"sample_count":%d,"min_duration_ms":150,"sets":[`,
		samples,
	) +
		`{"threshold_db":-35,"ranges":[` + silentRange + `]}]}`
	loud := fmt.Sprintf(
		`{"version":1,"integrated_lufs":-17.2,"lra_lu":5.1,"true_peak_dbtp":-1.4,"silent":false,"sample_count":%d}`,
		samples,
	)

	files := []struct {
		kind, variant, ext, body string
		profile                  int
		meta                     any
	}{
		{"probe", "", "json", string(probe), 1, nil},
		{"normalized", "", "mov", "normalized", 1, nil},
		{"proxy", "", "mp4", "proxy-bytes", 1, nil},
		{
			"thumbnails", "", "jpg", "sheet", 1,
			ingest.ThumbLayout{TileW: 90, TileH: 160, Columns: 2, Count: 2, IntervalMS: 1000},
		},
		{"thumbnails", "w320", "jpg", "thumb320", 1, nil},
		{"peaks", "", "bin", "peaks", 1, nil},
		{"silence", "", "json", silence, 1, nil},
		{"loudness", "", "json", loud, 1, nil},
	}

	res := &ingest.Result{
		Kind: ingest.KindVideo, Summary: sum, TimingsMS: map[string]int64{"probe": 1},
		Facts: ingest.Facts{
			FrameRate: ingest.Rational{
				Num: 30,
				Den: 1,
			},
			FrameCount:  int(durationMS * 30 / 1000),
			SampleCount: samples,
			Width:       320,
			Height:      240,
			HasAudio:    true,
		},
	}

	for _, f := range files {
		name := ingest.ArtifactName(f.kind, f.profile, f.variant, f.ext)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(f.body), 0o600); err != nil {
			return nil, err
		}

		h := sha256.Sum256([]byte(f.body))
		res.Artifacts = append(res.Artifacts, ingest.Artifact{
			Kind: f.kind, Profile: f.profile, Variant: f.variant, Ext: f.ext,
			RelPath: "artifacts/" + in.Hash + "/" + name, Size: int64(len(f.body)), SHA256: hex.EncodeToString(h[:]),
			Meta: f.meta,
		})
	}

	return res, nil
}

// realFrames is the frame renderer of view over the pinned LGPL FFmpeg: the
// directory comes from STUARTY_TEST_FFMPEG_DIR or from the install of
// scripts/install-ffmpeg.sh under the temporary directory.
func realFrames(tb testing.TB, dir string) *frame.Frames {
	tb.Helper()

	exe := "ffmpeg"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}

	ffdir := os.Getenv("STUARTY_TEST_FFMPEG_DIR")

	if ffdir == "" {
		for _, base := range []string{os.Getenv("RUNNER_TEMP"), os.Getenv("TEMP"), os.Getenv("TMPDIR")} {
			if base == "" {
				continue
			}

			if m, _ := filepath.Glob(filepath.Join(base, "ffmpeg", "*", "*", "bin", exe)); len(m) > 0 {
				ffdir = filepath.Dir(m[len(m)-1])

				break
			}
		}
	}

	if ffdir == "" {
		tb.Skip("no pinned FFmpeg: run scripts/install-ffmpeg.sh and set STUARTY_TEST_FFMPEG_DIR")
	}

	in, err := ffmpeg.Discover(
		context.Background(),
		ffmpeg.Config{FFmpegPath: filepath.Join(ffdir, exe), CPUCount: runtime.NumCPU()},
	)
	if err != nil || in.Info.License.Name != "lgpl" {
		tb.Skip("the FFmpeg in STUARTY_TEST_FFMPEG_DIR is not the pinned LGPL build")
	}

	runner, err := ffmpeg.NewRunner(in, dir)
	if err != nil {
		tb.Fatal(err)
	}

	frames, err := cache.Open(dir, 0)
	if err != nil {
		tb.Fatal(err)
	}

	return &frame.Frames{Runner: runner, ArtifactsDir: filepath.Join(dir, "artifacts"), Cache: frames}
}
