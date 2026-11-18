package frame

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/ingest"
	"github.com/AlexGalhardo/vukem-video/engine/ingest/mediagen"
	"github.com/AlexGalhardo/vukem-video/engine/store"
)

const envTestFFmpegDir = "VUKEM_VIDEO_TEST_FFMPEG_DIR"

// The tests share one workspace and the pinned FFmpeg: ingesting a fixture takes seconds.
var shared struct {
	once   sync.Once
	skip   string
	dir    string
	runner *ffmpeg.Runner
	caps   *ffmpeg.Capabilities
	store  *store.Store
	gen    mediagen.Gen
	clip   *ingest.Result
	hash   string
}

func TestMain(m *testing.M) {
	code := m.Run()
	if shared.dir != "" {
		_ = os.RemoveAll(shared.dir)
	}
	os.Exit(code)
}

func exe(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}
	return n
}

func mkShared() (string, error) {
	return os.MkdirTemp("", "vukem-frame-test-")
}

// setup finds the pinned LGPL FFmpeg, makes the workspace and ingests the portrait clip, or skips.
func setup(tb testing.TB) {
	tb.Helper()
	shared.once.Do(func() {
		dir := os.Getenv(envTestFFmpegDir)
		if dir == "" {
			for _, base := range []string{os.Getenv("RUNNER_TEMP"), os.Getenv("TEMP"), os.Getenv("TMPDIR")} {
				if base == "" {
					continue
				}
				if m, _ := filepath.Glob(filepath.Join(base, "ffmpeg", "*", "*", "bin", exe("ffmpeg"))); len(m) > 0 {
					dir = filepath.Dir(m[len(m)-1])
					break
				}
			}
		}
		if dir == "" {
			shared.skip = "no pinned FFmpeg: run scripts/install-ffmpeg.sh and set " + envTestFFmpegDir
			return
		}
		in, err := ffmpeg.Discover(
			context.Background(),
			ffmpeg.Config{
				FFmpegPath: filepath.Join(dir, exe("ffmpeg")),
				CPUCount:   runtime.NumCPU(),
				ToolsDir:   os.Getenv("VUKEM_VIDEO_TEST_TOOLS_DIR"),
			},
		)
		if err != nil || in.Info.License.Name != "lgpl" {
			shared.skip = "the FFmpeg in " + envTestFFmpegDir + " is not the pinned LGPL build"
			return
		}
		ws, err := mkShared()
		if err != nil {
			shared.skip = err.Error()
			return
		}
		shared.dir = ws
		r, err := ffmpeg.NewRunner(in, ws)
		if err != nil {
			shared.skip = err.Error()
			return
		}
		caps, err := r.ReadCapabilities(context.Background())
		if err != nil {
			shared.skip = "capabilities: " + err.Error()
			return
		}
		genDir := filepath.Join(ws, "fixtures")
		if err := os.Mkdir(genDir, 0o700); err != nil {
			shared.skip = err.Error()
			return
		}
		shared.runner, shared.caps, shared.store = r, caps, store.New(ws)
		shared.gen = mediagen.Gen{
			Dir: genDir,
			Run: func(ctx context.Context, dir string, args ...string) (string, error) {
				res, err := in.FFmpeg.Run(ctx, dir, args...)
				return res.Stderr, err
			},
		}

		ctx := context.Background()
		path, err := shared.gen.Clip(ctx, "frames-portrait.mp4", 360, 640, 30, 6)
		if err != nil {
			shared.skip = err.Error()
			return
		}
		f, err := os.Open(path)
		if err != nil {
			shared.skip = err.Error()
			return
		}
		put, err := shared.store.Put(ctx, f, 1<<33)
		_ = f.Close()
		if err != nil {
			shared.skip = err.Error()
			return
		}
		res, err := ingest.Run(ctx, ingest.Input{
			Runner: r, Caps: caps, Hash: put.Hash, ArtifactsDir: artifactsDir(), JobID: "ingest_frames",
		})
		if err != nil {
			shared.skip = "ingest: " + err.Error()
			return
		}
		shared.clip, shared.hash = res, put.Hash
	})
	if shared.skip != "" {
		tb.Skip(shared.skip)
	}
}

func artifactsDir() string { return filepath.Join(shared.dir, "artifacts") }

// sceneDoc is the reference scene "a clip with a caption" of the export tests, on the generated clip.
func sceneDoc(tb testing.TB) *timeline.Document {
	tb.Helper()
	setup(tb)
	data, err := os.ReadFile(filepath.Join("..", "export", "testdata", "scenes", "clip-caption", "project.json"))
	if err != nil {
		tb.Fatal(err)
	}
	doc, err := timeline.Decode(data)
	if err != nil {
		tb.Fatal(err)
	}
	const id = "ast_3c5v8n2w6d"
	a := doc.Assets[id]
	f := shared.clip.Facts
	a.Hash = "sha256:" + shared.hash
	a.FrameRate = timeline.Rational{Num: int(f.FrameRate.Num), Den: int(f.FrameRate.Den)}
	a.FrameCount, a.SampleCount, a.Width, a.Height, a.HasAudio = f.FrameCount, int(
		f.SampleCount,
	), f.Width, f.Height, f.HasAudio
	doc.Assets[id] = a
	return doc
}
