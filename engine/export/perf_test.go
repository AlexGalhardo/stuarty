package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// perfClip is the common case of SDD-004 section 12: a full-frame clip of the size of the project.
var perfClip = fixtureSpec{Name: "perf-1080x1920.mp4", Width: 1080, Height: 1920, FPS: 30, Seconds: 60}

// perfDoc is a 1080x1920 project at 30 fps with the clip over its first secs seconds and a caption on top.
func perfDoc(tb testing.TB, secs int) *timeline.Document {
	tb.Helper()
	frames := secs * 30
	data := fmt.Sprintf(`{
  "schema_version": 1, "id": "prj_p3r4f5g6h7", "name": "Common case",
  "settings": {"width": 1080, "height": 1920, "fps": {"num": 30, "den": 1}},
  "assets": {"ast_p1p1p1p1p1": {"id": "ast_p1p1p1p1p1", "hash": "sha256:%s", "kind": "video", "name": "perf.mp4",
    "frame_rate": {"num": 30, "den": 1}, "frame_count": 1800, "width": 1080, "height": 1920}},
  "tracks": {
    "trk_c1c1c1c1c1": {"id": "trk_c1c1c1c1c1", "kind": "visual", "name": "Captions", "order": "a0", "caption_style": {"highlight": "word"}},
    "trk_v1v1v1v1v1": {"id": "trk_v1v1v1v1v1", "kind": "visual", "name": "Video", "order": "a1"}},
  "items": {
    "itm_m1m1m1m1m1": {"id": "itm_m1m1m1m1m1", "type": "clip", "track": "trk_v1v1v1v1v1", "start": 0, "duration": %d, "asset": "ast_p1p1p1p1p1"},
    "itm_s1s1s1s1s1": {"id": "itm_s1s1s1s1s1", "type": "caption", "track": "trk_c1c1c1c1c1", "start": 0, "duration": %d,
      "words": [{"text": "Six", "start": 0, "duration": 20}, {"text": "words", "start": 20, "duration": 20},
        {"text": "on", "start": 40, "duration": 20}, {"text": "two", "start": 60, "duration": 20},
        {"text": "lines", "start": 80, "duration": 20}, {"text": "here", "start": 100, "duration": 20}]}}
}`, strings.Repeat("0", 64), frames, frames)
	doc, err := timeline.Decode([]byte(data))
	if err != nil {
		tb.Fatal(err)
	}
	useAsset(doc, "ast_p1p1p1p1p1", ingestClip(tb, perfClip))
	return doc
}

// memoryHeld is the memory the Go process holds from the system and has not returned to it.
func memoryHeld() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Sys - m.HeapReleased
}

// peakMemory samples the memory the Go process holds from the system, minus what it has returned, until
// stop is called, and returns the highest value in bytes.
func peakMemory() (stop func() uint64) {
	var (
		mu   sync.Mutex
		peak uint64
		done = make(chan struct{})
		wg   sync.WaitGroup
	)
	sample := func() {
		v := memoryHeld()
		mu.Lock()
		peak = max(peak, v)
		mu.Unlock()
	}
	wg.Go(func() {
		tick := time.NewTicker(50 * time.Millisecond)
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

// NFR-006, NFR-007: the 60 s export of the common case runs faster than real time and the Go process
// stays under 500 MB. The numbers are logged for SDD-004 section 12.
func TestExportSixtySecondsStaysUnder500MB(t *testing.T) {
	if testing.Short() {
		t.Skip("a 60 s export takes about a minute")
	}
	if raceEnabled {
		t.Skip("the race detector multiplies memory and time; NFR-006 and NFR-007 are measured without it")
	}
	doc := perfDoc(t, 60)
	job := newJob(t, doc, "export-60s", filepath.Join("perf", "60s.mp4"))

	// Hand back what the tests before this one left, so that the sample is the export's.
	debug.FreeOSMemory()
	before := memoryHeld()
	stop := peakMemory()
	start := time.Now()
	res, err := Run(context.Background(), job)
	elapsed := time.Since(start)
	peak := stop()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("60 s export with %s: %s (%.2f x real time), Go process peak %d MB (%d MB before it started), file %d MB",
		res.Encoder, elapsed.Round(time.Millisecond), 60/elapsed.Seconds(), peak>>20, before>>20, res.Size>>20)
	if peak > 500<<20 {
		t.Errorf("the Go process held %d MB, want under 500 MB (NFR-007)", peak>>20)
	}
	_ = os.Remove(res.Path)
}

// BenchmarkExportCommonCase is the first row of the table of SDD-004 section 12: 30 s of one full-frame
// clip plus a caption. It reports the speed in times real time and fails below the floor in
// STUARTY_BENCH_MIN_RT (the CI of the reference runner sets 1.5).
func BenchmarkExportCommonCase(b *testing.B) {
	if raceEnabled {
		b.Skip("the race detector multiplies memory and time")
	}
	const secs = 30
	doc := perfDoc(b, secs)
	floor, _ := strconv.ParseFloat(os.Getenv("STUARTY_BENCH_MIN_RT"), 64)

	b.ResetTimer()
	for i := range b.N {
		job := newJob(b, doc, "bench-"+strconv.Itoa(i), filepath.Join("bench", "out.mp4"))
		start := time.Now()
		res, err := Run(context.Background(), job)
		elapsed := time.Since(start)
		if err != nil {
			b.Fatal(err)
		}
		rt := secs / elapsed.Seconds()
		b.ReportMetric(rt, "x_realtime")
		b.Logf("%s: %.2f x real time", res.Encoder, rt)
		if floor > 0 && rt < floor {
			b.Fatalf("%.2f x real time, want at least %.2f", rt, floor)
		}
		_ = os.Remove(res.Path)
	}
}

// BenchmarkExport60s is NFR-006 and NFR-007: the export of a 60 s 1080x1920 30 fps project with captions
// completes within 60 s with the encoder the machine detects (a hardware or operating system one where
// there is one), and the Go process stays under 500 MB. It reports s_export, x_realtime and peak_MB.
func BenchmarkExport60s(b *testing.B) {
	if raceEnabled {
		b.Skip("the race detector multiplies memory and time")
	}
	doc := perfDoc(b, 60)
	r, err := ffmpeg.NewRunner(shared.runner.In, shared.dir)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := range b.N {
		job := newJob(b, doc, "bench60-"+strconv.Itoa(i), filepath.Join("bench60", "out.mp4"))
		job.Runner, job.Encoder = r, ffmpeg.DetectOptions{}

		debug.FreeOSMemory()
		stop := peakMemory()
		start := time.Now()
		res, err := Run(context.Background(), job)
		elapsed := time.Since(start)
		peak := stop()
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(elapsed.Seconds(), "s_export")
		b.ReportMetric(60/elapsed.Seconds(), "x_realtime")
		b.ReportMetric(float64(peak>>20), "peak_MB")
		b.Logf("%s: %.1f s, peak %d MB", res.Encoder, elapsed.Seconds(), peak>>20)
		_ = os.Remove(res.Path)
	}
}
