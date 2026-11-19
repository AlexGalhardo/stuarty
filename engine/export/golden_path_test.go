//go:build pathffmpeg

package export

import (
	"context"
	"image"
	"math"
	"os/exec"
	"runtime"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// TestGoldenPathFFmpeg composes the golden cases with the FFmpeg of the PATH when it is not the pinned
// one and reports the lowest PSNR of each case against the references (SDD-004 section 11: at least
// 50 dB per frame). Run it with go test -tags pathffmpeg -run Golden ./engine/export; the CI job that
// calls it does not block a merge.
func TestGoldenPathFFmpeg(t *testing.T) {
	setup(t)
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on the PATH")
	}
	in, err := ffmpeg.Discover(
		context.Background(),
		ffmpeg.Config{FFmpegPath: path, CPUCount: runtime.NumCPU(), ToolsDir: os.Getenv("STUARTY_TEST_TOOLS_DIR")},
	)
	if err != nil {
		t.Skipf("the FFmpeg on the PATH is not usable: %v", err)
	}
	if in.Info.Version == shared.runner.In.Info.Version && in.Info.License.Name == shared.runner.In.Info.License.Name {
		t.Skipf("the FFmpeg on the PATH is the pinned build (%s)", in.Info.Version)
	}
	other, err := ffmpeg.NewRunner(in, shared.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range requiredCases {
		t.Run(name, func(t *testing.T) {
			dir := goldenDir(name)
			doc := loadDir(t, dir)
			frames := readFrames(t, dir)
			job := newJob(t, doc, "path-"+name, "unused.mp4")
			job.Runner = other
			s := testState(t, job, job.JobID)
			rd := s.newRenderer(0)
			defer rd.close()
			dst := image.NewRGBA(image.Rect(0, 0, doc.Settings.Width, doc.Settings.Height))
			lowest := math.Inf(1)
			for _, f := range frames {
				if err := rd.render(dst, f); err != nil {
					t.Fatal(err)
				}
				lowest = min(lowest, psnr(readPNG(t, framePNG(dir, f)), dst.Pix))
			}
			t.Logf("%s: lowest PSNR %.1f dB with %s", name, lowest, in.Info.Version)
			if lowest < minPSNR {
				t.Errorf("lowest PSNR %.1f dB, want at least %.0f", lowest, minPSNR)
			}
		})
	}
}
