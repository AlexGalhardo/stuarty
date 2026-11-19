package export

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/engine/ingest/mediagen"
	"github.com/AlexGalhardo/stuarty/engine/store"
)

const envTestFFmpegDir = "STUARTY_TEST_FFMPEG_DIR"

var update = flag.Bool("update", false, "rewrite the golden files of testdata/golden")

// The tests share one workspace and the pinned FFmpeg: ingesting a fixture takes seconds, so each is
// made once per test binary.
var shared struct {
	once    sync.Once
	skip    string
	dir     string
	runner  *ffmpeg.Runner
	caps    *ffmpeg.Capabilities
	store   *store.Store
	gen     mediagen.Gen
	mu      sync.Mutex
	ingests map[string]*ingested
}

type ingested struct {
	hash  string
	facts ingest.Facts
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

// setup finds the pinned LGPL FFmpeg and makes the workspace, or records why the tests skip. A test
// never downloads: run scripts/install-ffmpeg.sh and set STUARTY_TEST_FFMPEG_DIR to the folder it prints.
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
				ToolsDir:   os.Getenv("STUARTY_TEST_TOOLS_DIR"),
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
		shared.ingests = map[string]*ingested{}
	})
	if shared.skip != "" {
		tb.Skip(shared.skip)
	}
}

func artifactsDir() string { return filepath.Join(shared.dir, "artifacts") }

// fixtureSpec describes a generated clip: the file name keys the cache.
type fixtureSpec struct {
	Name    string  `json:"name"`
	Width   int     `json:"width"`
	Height  int     `json:"height"`
	FPS     int     `json:"fps"`
	Seconds float64 `json:"seconds"`
	// DelaysCS makes the fixture an animated GIF with these frame delays in hundredths of a second;
	// Sticker makes it a PNG with a straight alpha. Both are images: FPS and Seconds are not used.
	DelaysCS []int `json:"delays_cs,omitempty"`
	Sticker  bool  `json:"sticker,omitempty"`
	// Alpha makes the fixture a ProRes 4444 video with an alpha channel.
	Alpha bool `json:"alpha,omitempty"`
	// AlphaVP9 makes the fixture a VP9 video in WebM with an alpha plane.
	AlphaVP9 bool `json:"alpha_vp9,omitempty"`
	// NoisyTone makes the fixture a WAV of a tone under white noise.
	NoisyTone bool `json:"noisy_tone,omitempty"`
	// OneClick makes the fixture a WAV of silence with one 10 ms burst at 0.5 s.
	OneClick bool `json:"one_click,omitempty"`
	// LUT makes the fixture a lookup table (lutFixture: "warm" or "teal"); the asset has no media.
	LUT string `json:"lut,omitempty"`
}

// generate writes the media file of the spec into the fixtures folder.
func (spec fixtureSpec) generate(ctx context.Context) (string, error) {
	switch {
	case spec.OneClick:
		return shared.gen.OneClick(ctx, spec.Name)
	case spec.NoisyTone:
		return shared.gen.NoisyTone(ctx, spec.Name)
	case len(spec.DelaysCS) > 0:
		return shared.gen.AnimatedGIF(spec.Name, spec.Width, spec.Height, spec.DelaysCS)
	case spec.Sticker:
		return shared.gen.StickerPNG(spec.Name, spec.Width, spec.Height)
	case spec.AlphaVP9:
		return shared.gen.AlphaWebM(ctx, spec.Name, spec.Width, spec.Height, spec.FPS, spec.Seconds)
	case spec.Alpha:
		return shared.gen.AlphaClip(ctx, spec.Name, spec.Width, spec.Height, spec.FPS, spec.Seconds)
	}
	return shared.gen.Clip(ctx, spec.Name, spec.Width, spec.Height, spec.FPS, spec.Seconds)
}

// ingestClip generates the clip, imports it and runs the ingest, once.
func ingestClip(tb testing.TB, spec fixtureSpec) *ingested {
	tb.Helper()
	setup(tb)
	shared.mu.Lock()
	defer shared.mu.Unlock()
	if in, ok := shared.ingests[spec.Name]; ok {
		return in
	}
	ctx := context.Background()
	path, err := spec.generate(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		tb.Fatal(err)
	}
	put, err := shared.store.Put(ctx, f, 1<<33)
	_ = f.Close()
	if err != nil {
		tb.Fatal(err)
	}
	_ = os.Remove(path)
	res, err := ingest.Run(ctx, ingest.Input{
		Runner:       shared.runner,
		Caps:         shared.caps,
		Hash:         put.Hash,
		ArtifactsDir: artifactsDir(),
		JobID:        "ingest_" + strings.ReplaceAll(spec.Name, ".", "_"),
	})
	if err != nil {
		tb.Fatalf("ingest of %s: %v", spec.Name, err)
	}
	in := &ingested{hash: put.Hash, facts: res.Facts}
	shared.ingests[spec.Name] = in
	return in
}

// useAsset points the asset of the document at an ingested clip.
func useAsset(doc *timeline.Document, id string, in *ingested) {
	a := doc.Assets[id]
	a.Hash = "sha256:" + in.hash
	a.FrameRate = timeline.Rational{Num: int(in.facts.FrameRate.Num), Den: int(in.facts.FrameRate.Den)}
	a.FrameCount, a.SampleCount = in.facts.FrameCount, int(in.facts.SampleCount)
	a.Width, a.Height, a.HasAudio = in.facts.Width, in.facts.Height, in.facts.HasAudio
	a.HasAlpha = in.facts.HasAlpha
	a.FrameDelays = in.facts.FrameDelays
	doc.Assets[id] = a
}

// loadScene reads a reference scene of testdata/scenes and ingests the media its fixtures.json names.
func loadScene(tb testing.TB, name string) *timeline.Document {
	tb.Helper()
	return loadDir(tb, filepath.Join("testdata", "scenes", name))
}

// loadDir reads project.json of a folder and ingests the media its fixtures.json names.
func loadDir(tb testing.TB, dir string) *timeline.Document {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		tb.Fatal(err)
	}
	doc, err := timeline.Decode(data)
	if err != nil {
		tb.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "fixtures.json"))
	if err != nil {
		tb.Fatal(err)
	}
	var specs map[string]fixtureSpec
	if err := json.Unmarshal(raw, &specs); err != nil {
		tb.Fatal(err)
	}
	for id, spec := range specs {
		if spec.LUT != "" {
			useLUT(tb, doc, id, spec)
			continue
		}
		useAsset(doc, id, ingestClip(tb, spec))
	}
	return doc
}

// newJob is a job for the document in the shared workspace.
func newJob(tb testing.TB, doc *timeline.Document, jobID, out string) Job {
	tb.Helper()
	setup(tb)
	return Job{
		Runner: shared.runner, Caps: shared.caps, Doc: doc, JobID: jobID, ArtifactsDir: artifactsDir(),
		OutPath: filepath.Join(shared.dir, "exports", out),
		// The tests of the exported file use libopenh264, the encoder of the bundled build (SDD-004 section 11).
		Encoder: ffmpeg.DetectOptions{Forced: "libopenh264"},
	}
}

// testState makes the state of a job without running an export, for the tests of the composed frames.
func testState(tb testing.TB, job Job, id string) *state {
	tb.Helper()
	dir, err := job.Runner.NewRunDir(id)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = dir.Remove() })
	return newState(context.Background(), &job, dir, endFrame(job.Doc))
}

// mkShared makes the workspace that every test of the binary shares; TestMain removes it.
func mkShared() (string, error) {
	return os.MkdirTemp("", "vukem-export-test-")
}
