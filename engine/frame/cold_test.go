package frame

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/ingest"
	"github.com/AlexGalhardo/vukem-video/engine/rasterize"
)

// phoneDoc is the clip-caption scene on a 1080x1920 portrait clip of the given length, the shape of the
// project a person pauses on in the editor.
func phoneDoc(tb testing.TB, seconds float64) *timeline.Document {
	tb.Helper()
	setup(tb)

	ctx := context.Background()

	path, err := shared.gen.Clip(ctx, "frames-phone.mp4", 1080, 1920, 30, seconds)
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
		Runner: shared.runner, Caps: shared.caps, Hash: put.Hash, ArtifactsDir: artifactsDir(), JobID: "ingest_phone",
	})
	if err != nil {
		tb.Fatal(err)
	}

	doc := sceneDoc(tb)
	doc.Settings.Width, doc.Settings.Height = 1080, 1920

	const id = "ast_3c5v8n2w6d"

	a := doc.Assets[id]
	facts := res.Facts
	a.Hash = "sha256:" + put.Hash
	a.FrameRate = timeline.Rational{Num: int(facts.FrameRate.Num), Den: int(facts.FrameRate.Den)}
	a.FrameCount, a.SampleCount, a.Width, a.Height = facts.FrameCount, int(facts.SampleCount), facts.Width, facts.Height
	doc.Assets[id] = a

	for iid := range doc.Items {
		it := doc.Items[iid]

		if it.Asset == id {
			it.Duration = min(it.Duration, facts.FrameCount)
			doc.Items[iid] = it
		}
	}

	return doc
}

// coldRun asks for count different frames at the UI width with no cache and returns the time of each.
func coldRun(tb testing.TB, doc *timeline.Document, count int) []time.Duration {
	tb.Helper()

	// The renderer module compiles once per server start, which service.New does ahead of the first view.
	rasterize.Warm()

	f := &Frames{Runner: shared.runner, ArtifactsDir: artifactsDir()}
	last := lastFrame(doc)
	out := make([]time.Duration, 0, count)

	for i := range count {
		at := 1 + i*(last-2)/max(count-1, 1)
		start := time.Now()

		if _, err := f.Frame(context.Background(), doc, Ref{Project: doc.ID, Revision: int64(i)}, at, 540); err != nil {
			tb.Fatal(err)
		}

		out = append(out, time.Since(start))
	}

	return out
}

// busyStartLimit is the median time that starting FFmpeg may take for a timing gate to mean anything.
// A cold frame is mostly one process start and a few decoded frames; on the reference machine a start
// takes 60 to 80 ms and a cold frame at the UI width 90 to 150 ms. With the CPU taken by other builds a
// start alone takes 150 ms and a frame 400 ms to 1.7 s, which says nothing about the engine.
const busyStartLimit = 150 * time.Millisecond

// startTime is the median time of five FFmpeg starts.
func startTime(tb testing.TB) time.Duration {
	tb.Helper()
	setup(tb)

	var starts []time.Duration

	for range 5 {
		start := time.Now()

		if err := exec.Command(shared.runner.In.FFmpeg.Path(), "-version").Run(); err != nil {
			tb.Fatal(err)
		}

		starts = append(starts, time.Since(start))
	}

	return percentile(starts, 0.5)
}

// skipIfBusy skips a timing gate on a machine that is too loaded to time a process start. It does not
// loosen the limit: on a quiet machine the gate is the one of the requirement. A gate calls it before
// measuring and again before it fails, because the load of other builds comes and goes.
func skipIfBusy(tb testing.TB) {
	tb.Helper()

	if med, busy := machineBusy(tb); busy {
		tb.Skipf("the machine is busy: starting FFmpeg takes %s (median of 5), over %s", med, busyStartLimit)
	}
}

// machineBusy says whether starting FFmpeg is slower than busyStartLimit now.
func machineBusy(tb testing.TB) (time.Duration, bool) {
	tb.Helper()

	med := startTime(tb)

	return med, med > busyStartLimit
}

func percentile(d []time.Duration, p float64) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)

	return s[min(len(s)-1, int(p*float64(len(s))))]
}

// BenchmarkViewFrameCold is NFR-004: a frame at the width of the editor (540) with a cold cache. It
// reports the median and the 95th percentile over frames spread across the clip.
func BenchmarkViewFrameCold(b *testing.B) {
	doc := phoneDoc(b, 12)

	b.ResetTimer()

	for range b.N {
		d := coldRun(b, doc, 40)
		b.ReportMetric(float64(percentile(d, 0.5).Milliseconds()), "p50_ms")
		b.ReportMetric(float64(percentile(d, 0.95).Milliseconds()), "p95_ms")
	}
}

// A cold frame at the editor width stays under the 350 ms of SDD-004 section 13 item 6 at the 95th
// percentile: the budget of NFR-004 (500 ms) less the 150 ms delay. The race detector and coverage
// instrumentation slow every step, so the gate is for normal builds.
func TestViewFrameColdP95(t *testing.T) {
	if raceEnabled || testing.CoverMode() != "" {
		t.Skip("the race detector and coverage instrumentation slow every step")
	}

	skipIfBusy(t)

	doc := phoneDoc(t, 6)
	d := coldRun(t, doc, 20)
	p50, p95 := percentile(d, 0.5), percentile(d, 0.95)
	t.Logf("cold frame at 540: p50 %s, p95 %s", p50, p95)

	if p95 > 350*time.Millisecond {
		skipIfBusy(t)
		t.Errorf("p95 %s, want at most 350 ms", p95)
	}
}

// pipDoc is phoneDoc with a second track holding a copy of the clip at half its size: a frame of two
// video layers, where each decoder is a process of its own.
func pipDoc(tb testing.TB, seconds float64) *timeline.Document {
	tb.Helper()

	doc := phoneDoc(tb, seconds)

	var clip timeline.Item

	for id := range doc.Items {
		if it := doc.Items[id]; it.Type == timeline.ItemClip {
			clip = it
		}
	}

	const (
		track = "trk_p1p2p3p4p5"
		item  = "itm_p1p2p3p4p5"
	)

	doc.Tracks[track] = timeline.Track{ID: track, Kind: timeline.TrackVisual, Order: "a00"}
	pip := clip
	pip.ID, pip.Track, pip.Scale = item, track, 0.5
	doc.Items[item] = pip

	return doc
}

// BenchmarkViewFrameColdPiP is NFR-004 on a frame of two clips: the processes of the two decoders
// start together, so the frame costs one start, not two.
func BenchmarkViewFrameColdPiP(b *testing.B) {
	doc := pipDoc(b, 12)

	b.ResetTimer()

	for range b.N {
		d := coldRun(b, doc, 40)
		b.ReportMetric(float64(percentile(d, 0.5).Milliseconds()), "p50_ms")
		b.ReportMetric(float64(percentile(d, 0.95).Milliseconds()), "p95_ms")
	}
}
