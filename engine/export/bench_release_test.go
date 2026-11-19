package export

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// The benchmarks of the release review (P4-RND-001): the BETA scenes of SDD-004 section 12 that the
// ALPHA benchmarks do not cover. Each one exports a fixture with the encoder the machine detects (a
// hardware or operating system one when there is one) and reports the wall time and the speed in times
// real time. The blur cost per frame is BenchmarkComposeBlur in this package.

// benchRelease exports doc b.N times and reports s_export, x_realtime and peak_MB.
func benchRelease(b *testing.B, doc *timeline.Document, format Format) {
	b.Helper()

	if raceEnabled {
		b.Skip("the race detector multiplies memory and time")
	}

	setup(b)

	r, err := ffmpeg.NewRunner(shared.runner.In, shared.dir)
	if err != nil {
		b.Fatal(err)
	}

	secs := float64(endFrame(doc)) * float64(doc.Settings.FPS.Den) / float64(doc.Settings.FPS.Num)

	b.ResetTimer()

	for i := range b.N {
		id := "release-" + strconv.Itoa(i)
		job := newJob(b, doc, id, filepath.Join("release", id+format.Ext()))
		job.Runner, job.Encoder, job.Format = r, ffmpeg.DetectOptions{}, format

		debug.FreeOSMemory()

		stop := peakMemory()
		start := time.Now()
		res, err := Run(context.Background(), job)
		elapsed := time.Since(start)
		peak := stop()

		if err != nil {
			if ee, ok := errors.AsType[*ffmpeg.Error](err); ok && ee.Details["reason"] == ReasonUnsupportedSize {
				b.Skipf("no encoder of this machine does this size: %v", err)
			}

			b.Fatal(err)
		}

		b.ReportMetric(elapsed.Seconds(), "s_export")
		b.ReportMetric(secs/elapsed.Seconds(), "x_realtime")
		b.ReportMetric(float64(peak>>20), "peak_MB")
		frames := secs * float64(doc.Settings.FPS.Num) / float64(doc.Settings.FPS.Den)
		b.Logf("%s: %.0f frames, %.1f s, peak %d MB", res.Encoder, frames, elapsed.Seconds(), peak>>20)

		_ = os.Remove(res.Path)
	}
}

func BenchmarkReleaseFPS60(b *testing.B) {
	benchRelease(b, loadDir(b, formatDir("fps60")), FormatMP4)
}

// BenchmarkReleaseFourK is half a second of 3840x2160 (15 frames, the length TestFormat4K uses).
func BenchmarkReleaseFourK(b *testing.B) {
	doc := loadDir(b, formatDir("fps60"))
	doc.Settings.Width, doc.Settings.Height = 3840, 2160
	doc.Settings.FPS = timeline.Rational{Num: 30, Den: 1}

	it := doc.Items["itm_m1n2p3q4r5"]
	it.Duration = 15
	doc.Items["itm_m1n2p3q4r5"] = it

	benchRelease(b, doc, FormatMP4)
}

func BenchmarkReleaseGIF(b *testing.B) {
	benchRelease(b, loadDir(b, formatDir("gif2s")), FormatGIF)
}

// BenchmarkReleaseAnimatedImage is the animated sticker of the golden scenes, exported to MP4.
func BenchmarkReleaseAnimatedImage(b *testing.B) {
	benchRelease(b, loadDir(b, filepath.Join("testdata", "golden", "sticker-anim")), FormatMP4)
}
