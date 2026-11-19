package export

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// The comparison tool of SDD-004 section 11 and SDD-008 section 4: exact comparison by hash, PSNR for a
// different FFmpeg build, and, when a hash changes, the images as artifacts of the job.

const (
	// envArtifacts is the folder where a failed comparison writes its images (CI uploads it).
	envArtifacts = "STUARTY_TEST_ARTIFACTS_DIR"
	// envBefore is a folder of the frames as they were before the change, <scene>-<frame>.png, from which a
	// difference image is made.
	envBefore = "STUARTY_TEST_GOLDEN_BEFORE_DIR"
	// envOtherFFmpeg is the bin folder of an FFmpeg other than the pinned one.
	envOtherFFmpeg = "STUARTY_TEST_OTHER_FFMPEG_DIR"
	// minPSNR is the tolerance for a different FFmpeg build (SDD-004 section 11), in decibels.
	minPSNR = 50.0
)

// psnr is the peak signal to noise ratio of two RGBA frames over the colour channels, in decibels; +Inf
// when they are equal.
func psnr(a, b []byte) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var sum float64
	var n int
	for i := 0; i+3 < len(a); i += 4 {
		for k := range 3 {
			d := float64(a[i+k]) - float64(b[i+k])
			sum += d * d
			n++
		}
	}
	if sum == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/(sum/float64(n)))
}

func writePNG(path string, w, h int, pix []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, &image.RGBA{Pix: pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// diffImage marks the pixels that differ: grey where equal, red where not, brighter for a larger
// difference.
func diffImage(a, b []byte) []byte {
	out := make([]byte, len(a))
	for i := 0; i+3 < len(a); i += 4 {
		d := 0
		for k := range 3 {
			d = max(d, int(math.Abs(float64(int(a[i+k])-int(b[i+k])))))
		}
		c := color.RGBA{R: 64, G: 64, B: 64, A: 255}
		if d > 0 {
			c = color.RGBA{R: uint8(min(128+d, 255)), A: 255}
		}
		out[i], out[i+1], out[i+2], out[i+3] = c.R, c.G, c.B, c.A
	}
	return out
}

// saveMismatch writes the frame that no longer matches its golden hash and, when the frame as it was is
// given, that frame and the difference (the artifacts of a failed job).
func saveMismatch(t *testing.T, scene string, frame, w, h int, got []byte) {
	t.Helper()
	dir := os.Getenv(envArtifacts)
	if dir == "" {
		return
	}
	name := scene + "-" + strconv.Itoa(frame)
	if err := writePNG(filepath.Join(dir, name+"-after.png"), w, h, got); err != nil {
		t.Logf("artifact: %v", err)
		return
	}
	before := os.Getenv(envBefore)
	if before == "" {
		return
	}
	f, err := os.Open(filepath.Join(before, name+".png"))
	if err != nil {
		t.Logf("no frame before: %v", err)
		return
	}
	defer func() { _ = f.Close() }()
	im, err := png.Decode(f)
	if err != nil || im.Bounds().Dx() != w || im.Bounds().Dy() != h {
		return
	}
	prev := image.NewRGBA(im.Bounds())
	for y := range h {
		for x := range w {
			r, g, b, a := im.At(x, y).RGBA()
			o := prev.PixOffset(x, y)
			prev.Pix[o], prev.Pix[o+1], prev.Pix[o+2], prev.Pix[o+3] = uint8(
				r>>8,
			), uint8(
				g>>8,
			), uint8(
				b>>8,
			), uint8(
				a>>8,
			)
		}
	}
	_ = writePNG(filepath.Join(dir, name+"-before.png"), w, h, prev.Pix)
	_ = writePNG(filepath.Join(dir, name+"-diff.png"), w, h, diffImage(prev.Pix, got))
}

func TestPSNR(t *testing.T) {
	t.Parallel()
	a := make([]byte, 400)
	b := make([]byte, 400)
	for i := range a {
		a[i], b[i] = 100, 100
	}
	if !math.IsInf(psnr(a, b), 1) {
		t.Error("equal frames are not infinite")
	}
	for i := 0; i < len(b); i += 4 {
		b[i] = 101
	}
	// One step in one of three channels: mean squared error 1/3, 10 log10(255^2 x 3) = 52.9 dB.
	if got := psnr(a, b); got < 52.8 || got > 53.0 {
		t.Errorf("psnr %.2f", got)
	}
	if psnr(a, b[:8]) != 0 {
		t.Error("frames of different sizes compare")
	}
}

func TestSaveMismatchWritesTheImages(t *testing.T) {
	dir, before := t.TempDir(), t.TempDir()
	t.Setenv(envArtifacts, dir)
	t.Setenv(envBefore, before)
	old := make([]byte, 4*4*4)
	now := make([]byte, 4*4*4)
	for i := 3; i < len(old); i += 4 {
		old[i], now[i] = 255, 255
	}
	now[0] = 90
	if err := writePNG(filepath.Join(before, "s-3.png"), 4, 4, old); err != nil {
		t.Fatal(err)
	}
	saveMismatch(t, "s", 3, 4, 4, now)
	for _, n := range []string{"s-3-after.png", "s-3-before.png", "s-3-diff.png"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
}

// SDD-004 section 11, a different FFmpeg build: the frames composed with the FFmpeg in
// STUARTY_TEST_OTHER_FFMPEG_DIR are within 50 dB PSNR of those composed with the pinned one. The result
// is reported, not blocking (the decoders of two builds may round differently).
func TestGoldenWithAnotherFFmpegIsWithin50dB(t *testing.T) {
	setup(t)
	dir := os.Getenv(envOtherFFmpeg)
	if dir == "" {
		t.Skip("set " + envOtherFFmpeg + " to the bin folder of another FFmpeg to compare builds")
	}
	exeName := "ffmpeg"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}
	in, err := ffmpeg.Discover(
		context.Background(),
		ffmpeg.Config{FFmpegPath: filepath.Join(dir, exeName), CPUCount: runtime.NumCPU()},
	)
	if err != nil {
		t.Skipf("the FFmpeg in %s is not usable: %v", envOtherFFmpeg, err)
	}
	other, err := ffmpeg.NewRunner(in, shared.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range referenceScenes {
		doc := loadScene(t, name)
		frames := sceneFrames(endFrame(doc))
		pinned := renderFrames(t, name, 0, frames)

		job := newJob(t, doc, "other-"+name, "unused.mp4")
		job.Runner = other
		s := testState(t, job, job.JobID)
		rd := s.newRenderer(0)
		dst := image.NewRGBA(image.Rect(0, 0, doc.Settings.Width, doc.Settings.Height))
		lowest := math.Inf(1)
		for i, f := range frames {
			if err := rd.render(dst, f); err != nil {
				rd.close()
				t.Fatal(err)
			}
			lowest = min(lowest, psnr(pinned[i], dst.Pix))
		}
		rd.close()
		verdict := "within tolerance"
		if lowest < minPSNR {
			verdict = "BELOW the tolerance (reported, not blocking)"
		}
		t.Logf("%s: lowest PSNR %.1f dB against the pinned build, %s", name, lowest, verdict)
	}
}

// NFR-011 and SDD-004 section 11: the browser preview during playback matches the export (SSIM of at
// least 0.97 on luma, no layer displaced by more than 1 pixel) on the three reference scenes. The test
// is the Playwright one, preview-vs-export (SDD-012), which needs the preview executor.
func TestPreviewMatchesTheExportOnTheReferenceScenes(t *testing.T) {
	t.Skip("blocked by P1-UI-003: the preview executor and the Playwright test preview-vs-export are not built yet")
}
