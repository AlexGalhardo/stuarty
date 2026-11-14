package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
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
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		mu.Lock()
		peak = max(peak, m.Sys-m.HeapReleased)
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
	doc := perfDoc(t, 60)
	job := newJob(t, doc, "export-60s", filepath.Join("perf", "60s.mp4"))

	runtime.GC()
	stop := peakMemory()
	start := time.Now()
	res, err := Run(context.Background(), job)
	elapsed := time.Since(start)
	peak := stop()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("60 s export with %s: %s (%.2f x real time), Go process peak %d MB, file %d MB",
		res.Encoder, elapsed.Round(time.Millisecond), 60/elapsed.Seconds(), peak>>20, res.Size>>20)
	if peak > 500<<20 {
		t.Errorf("the Go process held %d MB, want under 500 MB (NFR-007)", peak>>20)
	}
	_ = os.Remove(res.Path)
}

// BenchmarkExportCommonCase is the first row of the table of SDD-004 section 12: 30 s of one full-frame
// clip plus a caption. It reports the speed in times real time and fails below the floor in
// VUKEM_VIDEO_BENCH_MIN_RT (the CI of the reference runner sets 1.5).
func BenchmarkExportCommonCase(b *testing.B) {
	const secs = 30
	doc := perfDoc(b, secs)
	floor, _ := strconv.ParseFloat(os.Getenv("VUKEM_VIDEO_BENCH_MIN_RT"), 64)

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
