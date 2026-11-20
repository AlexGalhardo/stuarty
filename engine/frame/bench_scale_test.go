package frame

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
	"github.com/AlexGalhardo/stuarty/internal/perfgate"
)

// The ViewFrame and ViewSheet benchmarks of the ALPHA phase (P2-RND-014) run on the project of NFR-009:
// 5 minutes, 200 items and about 2,000 caption words, made by core/command (P2-ENG-018). Its media is a
// real 60 s clip, so the frames asked for lie in the first minute.

const (
	scaleProjectPath = "../../core/command/testdata/scale/project.json"
	scaleFrames      = 60 * 30
)

// scaleDoc reads the project (go generate ./core/command writes it; it is not committed), puts a real
// ingested clip under its video asset, drops the music and cuts the items to the first minute.
func scaleDoc(tb testing.TB) *timeline.Document {
	tb.Helper()
	setup(tb)

	data, err := os.ReadFile(scaleProjectPath)
	if err != nil {
		gen := exec.CommandContext(
			context.Background(), "go", "test", "-count=1", "-run", "TestScaleProject", "./core/command", "-gen-scale",
		)
		gen.Dir = filepath.Join("..", "..")

		if out, gerr := gen.CombinedOutput(); gerr != nil {
			tb.Skipf("the project of NFR-009 is missing and could not be generated: %v\n%s", gerr, out)
		}

		if data, err = os.ReadFile(scaleProjectPath); err != nil {
			tb.Fatal(err)
		}
	}

	doc, err := timeline.Decode(data)
	if err != nil {
		tb.Fatal(err)
	}

	ctx := context.Background()

	path, err := shared.gen.Clip(ctx, "frames-scale.mp4", 1080, 1920, 30, 60)
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

	res, err := ingest.Run(ctx, ingest.Input{
		Runner: shared.runner, Caps: shared.caps, Hash: put.Hash, ArtifactsDir: artifactsDir(), JobID: "ingest_scale",
	})
	if err != nil {
		tb.Fatal(err)
	}

	facts := res.Facts

	for id := range doc.Assets {
		a := doc.Assets[id]

		switch a.Kind {
		case timeline.AssetVideo:
			a.Hash = "sha256:" + put.Hash
			a.FrameRate = timeline.Rational{Num: int(facts.FrameRate.Num), Den: int(facts.FrameRate.Den)}
			a.FrameCount, a.SampleCount, a.Width, a.Height = facts.FrameCount, int(
				facts.SampleCount,
			), facts.Width, facts.Height
			doc.Assets[id] = a
		case timeline.AssetAudio:
			delete(doc.Assets, id)
		case timeline.AssetImage, timeline.AssetLUT:
		}
	}

	for id := range doc.Items {
		it := doc.Items[id]

		switch {
		case it.Type == timeline.ItemAudio || it.Start >= scaleFrames:
			delete(doc.Items, id)
		default:
			it.Duration = min(it.Duration, scaleFrames-it.Start)
			if it.Type == timeline.ItemClip {
				it.Duration = min(it.Duration, facts.FrameCount-it.SourceIn)
			}

			doc.Items[id] = it
		}
	}

	if vs := timeline.Validate(doc); len(vs) != 0 {
		tb.Fatalf("the scale project is invalid: %v", vs)
	}

	return doc
}

// coldAt asks for count different frames at a width with no cache and returns the time of each.
func coldAt(tb testing.TB, doc *timeline.Document, count, width int) []time.Duration {
	tb.Helper()

	// The renderer module compiles once per server start, which service.New does ahead of the first view.
	rasterize.Warm()

	f := &Frames{Runner: shared.runner, ArtifactsDir: artifactsDir()}
	out := make([]time.Duration, 0, count)

	for i := range count {
		at := 1 + i*(scaleFrames-3)/max(count-1, 1)
		start := time.Now()

		if _, err := f.Frame(
			context.Background(),
			doc,
			Ref{Project: doc.ID, Revision: int64(i)},
			at,
			width,
		); err != nil {
			tb.Fatal(err)
		}

		out = append(out, time.Since(start))
	}

	return out
}

// BenchmarkViewFrameColdScale is NFR-004 and NFR-023 on the big project: a frame with a cold cache at the
// width of the editor (540) and at the width of an agent (384), p50 and p95 over frames of the first minute.
func BenchmarkViewFrameColdScale(b *testing.B) {
	doc := scaleDoc(b)

	for _, w := range []struct {
		name  string
		width int
	}{{"UI540", DefaultUIWidth}, {"Agent384", DefaultAgentWidth}} {
		b.Run(w.name, func(b *testing.B) {
			for range b.N {
				d := coldAt(b, doc, 40, w.width)
				b.ReportMetric(float64(percentile(d, 0.5).Milliseconds()), "p50_ms")
				b.ReportMetric(float64(percentile(d, 0.95).Milliseconds()), "p95_ms")
			}
		})
	}
}

// BenchmarkViewSheet9Scale is a contact sheet of nine cells of the big project with a cold cache.
func BenchmarkViewSheet9Scale(b *testing.B) {
	doc := scaleDoc(b)
	times := SheetTimes(0, scaleFrames-3, DefaultSheetCells)

	for i := range b.N {
		f := &Frames{Runner: shared.runner, ArtifactsDir: artifactsDir()}
		start := time.Now()

		s, err := f.Sheet(
			context.Background(),
			doc,
			Ref{Project: doc.ID, Revision: int64(i)},
			SheetRequest{Times: times},
		)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportMetric(float64(time.Since(start).Milliseconds()), "sheet_ms")
		b.ReportMetric(float64(s.Tokens), "tokens")
	}
}

// The timing gates of SDD-004 section 13 item 6 on the project of NFR-009: a cold frame within 500 ms at
// the UI width at the 95th percentile (NFR-004) and within 2 s at the agent width (NFR-023). They are
// absolute time budgets: they run only on the idle machine with STUARTY_PERF=1 (SDD-008 section 10).
func TestScaleProjectViewTimeBudgets(t *testing.T) {
	perfgate.Require(t)

	if raceEnabled || testing.CoverMode() != "" {
		t.Skip("the race detector and coverage slow every step") // verify:env-skip an instrumented build
	}

	doc := scaleDoc(t)

	ui := percentile(coldAt(t, doc, 20, DefaultUIWidth), 0.95)
	agent := percentile(coldAt(t, doc, 20, DefaultAgentWidth), 0.95)
	t.Logf("cold frame on the scale project: p95 %s at 540, %s at 384", ui, agent)

	if ui > 500*time.Millisecond {
		t.Errorf("UI width p95 %s, want at most 500 ms (NFR-004)", ui)
	}

	if agent > 2*time.Second {
		t.Errorf("agent width p95 %s, want at most 2 s (NFR-023)", agent)
	}
}

// The image token estimate of an agent frame is at most 800 and of a sheet at most 1,600 (NFR-022), and
// the scale project yields a frame at both widths. None of it depends on the load.
func TestScaleProjectViewGates(t *testing.T) {
	doc := scaleDoc(t)

	for _, width := range []int{DefaultUIWidth, DefaultAgentWidth} {
		if d := coldAt(t, doc, 3, width); len(d) != 3 {
			t.Fatalf("%d frames at width %d, want 3", len(d), width)
		}
	}

	st := doc.Settings

	frameTokens := Tokens(DefaultAgentWidth, DefaultAgentWidth*st.Height/st.Width)
	if frameTokens > 800 {
		t.Errorf("an agent frame costs %d tokens, want at most 800", frameTokens)
	}

	f := &Frames{Runner: shared.runner, ArtifactsDir: artifactsDir()}

	s, err := f.Sheet(context.Background(), doc, Ref{Project: doc.ID, Revision: 1},
		SheetRequest{Times: SheetTimes(0, scaleFrames-3, DefaultSheetCells)})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("agent frame %d tokens, sheet %dx%d %d tokens", frameTokens, s.Width, s.Height, s.Tokens)

	if s.Tokens > 1600 {
		t.Errorf("a sheet costs %d tokens, want at most 1,600", s.Tokens)
	}
}
