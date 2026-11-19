package ingest

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
	"github.com/AlexGalhardo/stuarty/engine/ingest/mediagen"
	"github.com/AlexGalhardo/stuarty/engine/store"
)

const envTestFFmpegDir = "STUARTY_TEST_FFMPEG_DIR"

var (
	pinOnce sync.Once
	pinInst *ffmpeg.Installation
	pinCaps *ffmpeg.Capabilities
	pinSkip string
)

func exe(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}
	return n
}

// pinnedInstall returns the pinned LGPL FFmpeg and its capabilities, or skips the test. A test never
// downloads: run scripts/install-ffmpeg.sh and set STUARTY_TEST_FFMPEG_DIR to the folder it prints.
func pinnedInstall(tb testing.TB) (*ffmpeg.Installation, *ffmpeg.Capabilities) {
	tb.Helper()
	pinOnce.Do(func() {
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
			pinSkip = "no pinned FFmpeg: run scripts/install-ffmpeg.sh and set " + envTestFFmpegDir + " to the folder it prints"
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
			pinSkip = "the FFmpeg in " + envTestFFmpegDir + " is not the pinned LGPL build"
			return
		}
		r, err := ffmpeg.NewRunner(in, os.TempDir())
		if err != nil {
			pinSkip = err.Error()
			return
		}
		caps, err := r.ReadCapabilities(context.Background())
		if err != nil {
			pinSkip = "capabilities: " + err.Error()
			return
		}
		pinInst, pinCaps = in, caps
	})
	if pinSkip != "" {
		tb.Skip(pinSkip)
	}
	return pinInst, pinCaps
}

// env is one workspace with the pinned FFmpeg.
type env struct {
	t     testing.TB
	r     *ffmpeg.Runner
	caps  *ffmpeg.Capabilities
	store *store.Store
	gen   mediagen.Gen
	ws    string
	n     int
	// soundMaps asks Run for the speech map and the beat grid.
	soundMaps bool
}

func newEnv(tb testing.TB) *env {
	tb.Helper()
	in, caps := pinnedInstall(tb)
	ws := tb.TempDir()
	r, err := ffmpeg.NewRunner(in, ws)
	if err != nil {
		tb.Fatal(err)
	}
	genDir := tb.TempDir()
	return &env{t: tb, r: r, caps: caps, store: store.New(ws), ws: ws, gen: mediagen.Gen{
		Dir: genDir,
		Run: func(ctx context.Context, dir string, args ...string) (string, error) {
			res, err := in.FFmpeg.Run(ctx, dir, args...)
			return res.Stderr, err
		},
	}}
}

func (e *env) artifacts() string { return filepath.Join(e.ws, "artifacts") }

// put copies a file into the store and returns its hash.
func (e *env) put(path string) string {
	e.t.Helper()
	f, err := os.Open(path)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	res, err := e.store.Put(context.Background(), f, 1<<33)
	if err != nil {
		e.t.Fatal(err)
	}
	return res.Hash
}

// ingest imports a file and runs the ingest.
func (e *env) ingest(ctx context.Context, path string, progress func(Progress)) (string, *Result, error) {
	e.t.Helper()
	hash := e.put(path)
	e.n++
	res, err := Run(ctx, Input{
		Runner: e.r, Caps: e.caps, Hash: hash, ArtifactsDir: e.artifacts(),
		JobID: "job_" + itoa(e.n), OnProgress: progress, SoundMaps: e.soundMaps,
	})
	return hash, res, err
}

func (e *env) mustIngest(path string) (string, *Result) {
	e.t.Helper()
	hash, res, err := e.ingest(context.Background(), path, nil)
	if err != nil {
		e.t.Fatalf("ingest of %s: %v", filepath.Base(path), err)
	}
	return hash, res
}

// probeArtifact probes a published artifact.
func (e *env) probeArtifact(hash, name string) *Doc {
	e.t.Helper()
	dir, err := e.r.NewRunDir("probe_" + itoa(e.n) + "_" + name[:3])
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = dir.Remove() }()
	argv, err := ffargs.ProbeJSON(
		e.r.Policy(dir, filepath.Join(e.artifacts(), hash)),
		filepath.Join(e.artifacts(), hash, name),
	)
	if err != nil {
		e.t.Fatal(err)
	}
	out, _, err := e.r.RunFFprobe(
		context.Background(),
		dir,
		argv,
		ffmpeg.ProbeTimeout,
		filepath.Join(e.artifacts(), hash),
	)
	if err != nil {
		e.t.Fatalf("probe of %s: %v", name, err)
	}
	d, err := ParseDoc(out)
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}

func (r *Result) find(kind, variant string) *Artifact {
	for i := range r.Artifacts {
		if r.Artifacts[i].Kind == kind && r.Artifacts[i].Variant == variant {
			return &r.Artifacts[i]
		}
	}
	return nil
}

func (r *Result) names() []string {
	var n []string
	for _, a := range r.Artifacts {
		n = append(n, ArtifactName(a.Kind, a.Profile, a.Variant, a.Ext))
	}
	return n
}

func readJSONFile(tb testing.TB, path string) []byte {
	tb.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	return bytes.TrimSpace(b)
}
