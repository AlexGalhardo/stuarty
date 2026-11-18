package export

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/rasterize"
)

// The benchmarks of the ALPHA phase run on the project of NFR-009: 5 minutes at 30 fps, 200 items and
// about 2,000 caption words, made by core/command (P2-ENG-018). Its media is the ingested 60 s clip of the
// common case, so the benchmarks look at the first 60 s of it.

const scaleProjectPath = "../../core/command/testdata/scale/project.json"

// scaleFrames is the first minute: the length of the media of the project.
const scaleFrames = 60 * 30

// scaleDoc reads the project of NFR-009 (go generate ./core/command writes it; it is not committed),
// puts the ingested clip under the video asset, drops the music (the common case has none) and cuts every
// item to the first scaleFrames frames.
func scaleDoc(tb testing.TB) *timeline.Document {
	tb.Helper()

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

	in := ingestClip(tb, perfClip)

	for id := range doc.Assets {
		a := doc.Assets[id]

		switch a.Kind {
		case timeline.AssetVideo:
			useAsset(doc, id, in)
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
				it.Duration = min(it.Duration, in.facts.FrameCount-it.SourceIn)
			}

			doc.Items[id] = it
		}
	}

	if vs := timeline.Validate(doc); len(vs) != 0 {
		tb.Fatalf("the scale project is invalid: %v", vs)
	}

	return doc
}

// peakRSS samples the resident memory of the process every 250 ms until stop is called and returns the
// highest value in bytes; zero when this system cannot say.
func peakRSS() (stop func() uint64) {
	var (
		mu   sync.Mutex
		peak uint64
		done = make(chan struct{})
		wg   sync.WaitGroup
	)

	sample := func() {
		if v, ok := residentBytes(); ok {
			mu.Lock()
			peak = max(peak, v)
			mu.Unlock()
		}
	}

	wg.Go(func() {
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()

		for {
			sample()

			select {
			case <-done:
				return
			case <-tick.C:
			}
		}
	})

	return func() uint64 {
		close(done)
		wg.Wait()
		sample()
		mu.Lock()
		defer mu.Unlock()

		return peak
	}
}

func TestResidentSampler(t *testing.T) {
	if v, ok := residentBytes(); ok && v == 0 {
		t.Error("the resident set of a running process is not zero")
	}

	stop := peakRSS()
	ballast := make([]byte, 64<<20)

	for i := range ballast {
		ballast[i] = 1
	}

	if got := stop(); got != 0 && got < uint64(len(ballast)) {
		t.Errorf("peak %d MB with a 64 MB buffer in use", got>>20)
	}

	_ = ballast[len(ballast)-1]
}

// BenchmarkPlanWindow300Scale is the plan of 300 frames, with the text measured by the rasterizer, over
// the 200 items and 2,000 caption words of the project of NFR-009 (SDD-004 section 12).
func BenchmarkPlanWindow300Scale(b *testing.B) {
	setup(b)

	doc := scaleDoc(b)
	opts := plan.Options{Measure: rasterize.Measure(rasterize.Bundled())}

	b.ReportAllocs()

	for b.Loop() {
		plan.WindowWith(doc, 0, 300, opts)
	}
}

// BenchmarkExportScale60s exports the first 60 s of the project of NFR-009 with the encoder the machine
// detects (NFR-006, NFR-007, SDD-004 section 13 check 5). It reports the time, the speed and the
// resident memory of the Go process, sampled every 250 ms, and fails above 60 s or 500 MB.
func BenchmarkExportScale60s(b *testing.B) {
	if raceEnabled {
		b.Skip("the race detector multiplies memory and time")
	}

	setup(b)

	doc := scaleDoc(b)

	r, err := ffmpeg.NewRunner(shared.runner.In, shared.dir)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := range b.N {
		job := newJob(b, doc, "scale60-"+strconv.Itoa(i), filepath.Join("scale60", "out.mp4"))
		job.Runner, job.Encoder = r, ffmpeg.DetectOptions{}

		debug.FreeOSMemory()

		stop := peakRSS()
		start := time.Now()
		res, err := Run(context.Background(), job)
		elapsed := time.Since(start)
		peak := stop()

		if err != nil {
			b.Fatal(err)
		}

		b.ReportMetric(elapsed.Seconds(), "s_export")
		b.ReportMetric(60/elapsed.Seconds(), "x_realtime")
		b.ReportMetric(float64(peak>>20), "peak_rss_MB")
		b.Logf("%s: %.1f s, resident peak %d MB", res.Encoder, elapsed.Seconds(), peak>>20)

		if elapsed > time.Minute {
			b.Errorf("%.1f s for 60 s of video, want at most 60 (NFR-006)", elapsed.Seconds())
		}

		if peak > 500<<20 {
			b.Errorf("the Go process held %d MB resident, want under 500 (NFR-007)", peak>>20)
		}

		_ = os.Remove(res.Path)
	}
}

// NFR-010: two exports of the same project and media on the same machine compose the same frames. The
// frames are hashed as the compositor makes them (the encoder's bytes may differ, the composition may not).
func TestScaleProjectComposesTheSameTwice(t *testing.T) {
	setup(t)

	doc := scaleDoc(t)
	var frames []int
	for i, f := range sceneFrames(scaleFrames) {
		if i%3 == 0 {
			frames = append(frames, f)
		}
	}

	first := renderDoc(t, doc, "scale-a", 0, frames)
	second := renderDoc(t, doc, "scale-b", 0, frames)

	for i, f := range frames {
		if !bytes.Equal(first[i], second[i]) {
			t.Fatalf("frame %d differs between two runs of the same project", f)
		}
	}
}
