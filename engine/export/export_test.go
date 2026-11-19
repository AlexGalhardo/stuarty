package export

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
)

func asFFError(t *testing.T, err error) *ffmpeg.Error {
	t.Helper()
	ee, ok := errors.AsType[*ffmpeg.Error](err)
	if !ok {
		t.Fatalf("err = %v, want an *ffmpeg.Error", err)
	}
	return ee
}

func noLeftovers(t *testing.T, jobID, out string) {
	t.Helper()
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("%s exists after a failed export", out)
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(out), "*")); len(m) > 0 {
		t.Errorf("the folder of the output holds %v", m)
	}
	if m, _ := filepath.Glob(filepath.Join(shared.dir, "tmp", jobID+"*")); len(m) > 0 {
		t.Errorf("the work folders %v were not removed", m)
	}
}

// Section 8 on each reference scene: the file passes verification, lands at OutPath, leaves no work
// folder, and its frames decoded look like the composed frames (SSIM of at least 0.95 on luma, section 11).
func TestExportReferenceScenes(t *testing.T) {
	for _, name := range referenceScenes {
		t.Run(name, func(t *testing.T) {
			doc := loadScene(t, name)
			job := newJob(t, doc, "export-"+name, name+".mp4")
			var last Progress
			job.OnProgress = func(p Progress) { last = p }
			res, err := Run(context.Background(), job)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("encoder %s, %d bytes", res.Encoder, res.Size)
			frames := endFrame(doc)
			if res.Frames != frames || res.Path != job.OutPath || res.Size == 0 || res.Encoder == "" {
				t.Errorf("result %+v", res)
			}
			if last.Frame != frames || last.Total != frames {
				t.Errorf("last progress %+v, want %d of %d", last, frames, frames)
			}
			if m, _ := filepath.Glob(filepath.Join(shared.dir, "tmp", job.JobID+"*")); len(m) > 0 {
				t.Errorf("work folders left: %v", m)
			}

			picks := []int{0, frames / 3, frames / 2, frames - 1}
			composed := renderFrames(t, name, 0, picks)
			for i, f := range picks {
				got := decodeFrame(t, res.Path, doc, f)
				s := ssim(luma(composed[i]), luma(got), doc.Settings.Width, doc.Settings.Height)
				if s < 0.95 {
					t.Errorf(
						"frame %d: SSIM %.4f of the exported file against the composed frame, want at least 0.95",
						f,
						s,
					)
				}
			}
		})
	}
}

// The audio of the export is at the project's loudness target, or at the peak ceiling when that comes first.
func TestExportLoudness(t *testing.T) {
	doc := loadScene(t, "clip-caption")
	job := newJob(t, doc, "export-loudness", "loudness.mp4")
	res, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	l := res.Loudness
	if l.IntegratedLUFS == nil || l.TruePeakDBTP == nil {
		t.Fatalf("loudness %+v: the mix was measured as silent", l)
	}

	dir, err := shared.runner.NewRunDir("loudness-check")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Remove() }()
	in := filepath.Dir(res.Path)
	argv, err := ffargs.Cmd{
		Inputs:  []ffargs.Input{ffargs.FileInput(res.Path)},
		Outputs: []ffargs.Output{{Format: "null", Maps: []string{"0:a:0"}, AF: ffargs.Chain1(ffargs.Ebur128())}},
	}.Argv(shared.runner.Policy(dir, in))
	if err != nil {
		t.Fatal(err)
	}
	out, err := shared.runner.RunFFmpeg(
		context.Background(), ffmpeg.Request{Dir: dir, Argv: argv, InputDirs: []string{in}, Deadline: time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ingest.ParseLoudness(out.Stderr, 0)
	if err != nil || got.IntegratedLUFS == nil || got.TruePeakDBTP == nil {
		t.Fatalf("loudness of the file: %v %+v", err, got)
	}
	gi, gtp := *got.IntegratedLUFS, *got.TruePeakDBTP
	t.Logf("measured %.1f LUFS, %.1f dBTP; gain %.1f dB; target %.1f", gi, gtp, l.GainDB, l.TargetLUFS)
	if gtp > -1.0 {
		t.Errorf("true peak of the file %.1f dBTP, want at most -1.0", gtp)
	}
	if lim := *l.TruePeakDBTP + l.GainDB; lim < -1.9 {
		// Not limited by the peak: the file is at the target.
		if d := gi - l.TargetLUFS; d < -1 || d > 1 {
			t.Errorf("integrated loudness %.1f LUFS, target %.1f", gi, l.TargetLUFS)
		}
	}
}

// An output that is not whole fails verification, leaves nothing under exports/ and no work folder.
func TestVerifyTruncatedOutputFails(t *testing.T) {
	doc := loadScene(t, "contain-text")
	job := newJob(t, doc, "export-truncated", filepath.Join("trunc", "out.mp4"))
	job.afterEncode = func(partial string) error {
		fi, err := os.Stat(partial)
		if err != nil {
			return err
		}
		return os.Truncate(partial, fi.Size()*6/10)
	}
	_, err := Run(context.Background(), job)
	if err == nil {
		t.Fatal("a truncated file was published")
	}
	ee := asFFError(t, err)
	if ee.Code != CodeVerifyFailed {
		t.Fatalf("code %s, want %s: %v", ee.Code, CodeVerifyFailed, err)
	}
	checks, ok := ee.Details["checks"].([]Check)
	if !ok || len(checks) == 0 {
		t.Fatalf("details %v carry no failed checks", ee.Details)
	}
	t.Logf("failed checks: %+v", checks)
	if _, statErr := os.Stat(job.OutPath); !os.IsNotExist(statErr) {
		t.Error("the truncated file was published")
	}
	if m, _ := filepath.Glob(filepath.Join(shared.dir, "tmp", job.JobID+"*")); len(m) > 0 {
		t.Errorf("the work folders %v were not removed", m)
	}
}

// Cancelling ends the export at once: no FFmpeg process remains, no partial file, no work folder
// (SDD-004 section 13 item 8).
func TestCancelLeavesNothing(t *testing.T) {
	doc := loadScene(t, "two-clips-cut")
	job := newJob(t, doc, "export-cancel", filepath.Join("cancel", "out.mp4"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job.OnProgress = func(Progress) { cancel() }

	start := time.Now()
	_, err := Run(ctx, job)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want a cancellation", err)
	}
	t.Logf("cancelled export returned after %s", time.Since(start))
	noLeftovers(t, job.JobID, job.OutPath)

	marker := filepath.ToSlash(shared.dir)
	n, ok := ffmpegProcesses(marker)
	if !ok {
		t.Skip("this machine cannot list processes")
	}
	if n != 0 {
		t.Errorf("%d FFmpeg processes of the export are still running", n)
	}
}

func TestLintGateRefusesAnEmptyTimeline(t *testing.T) {
	setup(t)
	doc := loadScene(t, "contain-text")
	doc.Items = map[string]timeline.Item{}
	job := newJob(t, doc, "export-empty", filepath.Join("empty", "out.mp4"))
	_, err := Run(context.Background(), job)
	ee := asFFError(t, err)
	if ee.Code != CodeLintBlocking {
		t.Fatalf("code %s: %v", ee.Code, err)
	}
	if f, ok := ee.Details["findings"]; !ok || f == nil {
		t.Errorf("details %v carry no findings", ee.Details)
	}
	noLeftovers(t, job.JobID, job.OutPath)
}

func TestLintGateNoEncoderIsRefused(t *testing.T) {
	setup(t)
	doc := loadScene(t, "contain-text")
	r, err := ffmpeg.NewRunner(shared.runner.In, shared.dir)
	if err != nil {
		t.Fatal(err)
	}
	job := newJob(t, doc, "export-noenc", "noenc.mp4")
	job.Runner = r
	job.Encoder = ffmpeg.DetectOptions{Candidates: []string{}}
	_, err = Run(context.Background(), job)
	if ee := asFFError(t, err); ee.Code != ffmpeg.CodeNoEncoder {
		t.Fatalf("code %s: %v", ee.Code, err)
	}
}

func TestExportMissingMediaFailsCleanly(t *testing.T) {
	doc := loadScene(t, "contain-text")
	for id, a := range doc.Assets {
		a.Hash = "sha256:" + strings.Repeat("ab", 32)
		doc.Assets[id] = a
	}
	job := newJob(t, doc, "export-nomedia", filepath.Join("nomedia", "out.mp4"))
	_, err := Run(context.Background(), job)
	if ee := asFFError(t, err); ee.Code != CodeExportFailed {
		t.Fatalf("code %s: %v", ee.Code, err)
	}
	noLeftovers(t, job.JobID, job.OutPath)
}

// A clip decoded in several runs gives the frames the export composes: the pre-pass decides pre-scaling per
// item, and an animated layer is not scaled by its decoder.
func TestScanLayersFindsConstantAndAnimatedLayers(t *testing.T) {
	doc := loadScene(t, "contain-text")
	job := newJob(t, doc, "scan", "unused.mp4")
	s := testState(t, job, "scan")
	infos := s.scanLayers()
	in, ok := infos["itm_k1k1k1k1k1"]
	if !ok {
		t.Fatalf("infos %v", infos)
	}
	if in.constant {
		t.Error("the keyframed scale of the contain clip was taken as constant")
	}

	doc = loadScene(t, "clip-caption")
	job = newJob(t, doc, "scan2", "unused.mp4")
	s = testState(t, job, "scan2")
	if in := s.scanLayers()["itm_m1n2p3q4r5"]; in == nil || !in.constant {
		t.Errorf("the plain clip was not constant: %+v", in)
	}
}

// A changing crop is recorded like a changing size, so nothing pre-scales one rectangle for the item. A
// static crop stays constant. A sliding mask does not change the crop.
func TestScanLayersTreatsAChangingCropLikeAChangingSize(t *testing.T) {
	check := func(name string, vary, constant bool) {
		t.Helper()
		c := alphaCases[name]()
		s := &state{job: &Job{Doc: c.doc}, frames: endFrame(c.doc)}
		infos := s.scanLayers()
		if len(infos) != 1 {
			t.Fatalf("%s: %d media items", name, len(infos))
		}
		for id, in := range infos {
			if in.cropVaries != vary || in.constant != constant {
				t.Errorf("%s %s: cropVaries %v constant %v, want %v and %v",
					name, id, in.cropVaries, in.constant, vary, constant)
			}
		}
	}
	check("crop", false, true)
	check("crop-animated", true, false)
	check("crop-still", true, false)
	check("mask-animated", false, true)
}

// Frame 0 and frame 10 of a frozen source differ only when the crop or the mask of that frame is the one
// that is drawn. The same bytes at 1 and 8 bands are the rule of SDD-004 section 12.
func TestAnimatedCropAndMaskFramesDiffer(t *testing.T) {
	setup(t)
	for _, name := range []string{"crop-animated", "crop-still", "mask-animated"} {
		t.Run(name, func(t *testing.T) {
			c := alphaCases[name]()
			for id, spec := range c.fixtures {
				useAsset(c.doc, id, ingestClip(t, spec))
			}
			one := renderDoc(t, c.doc, name, 1, c.frames)
			eight := renderDoc(t, c.doc, name, 8, c.frames)
			if len(one) != 2 || len(eight) != 2 {
				t.Fatalf("frames %d and %d", len(one), len(eight))
			}
			for i := range one {
				if !bytes.Equal(one[i], eight[i]) {
					t.Fatalf("frame %d differs between 1 band and 8 bands", c.frames[i])
				}
			}
			if bytes.Equal(one[0], one[1]) {
				t.Fatal("the two frames are the same picture")
			}
		})
	}
}

// A shadow and an outline change pixels of the export compositor, and the bands stay equal. The corner of
// the frame stays the opaque background, so the pad is the layer's and not a full-frame fill.
func TestShadowAndOutlineChangeThePicture(t *testing.T) {
	setup(t)
	for _, name := range []string{"shadow", "outline"} {
		t.Run(name, func(t *testing.T) {
			c := alphaCases[name]()
			for id, spec := range c.fixtures {
				useAsset(c.doc, id, ingestClip(t, spec))
			}
			one := renderDoc(t, c.doc, name, 1, c.frames)
			eight := renderDoc(t, c.doc, name, 8, c.frames)
			if len(one) != 1 || len(eight) != 1 || !bytes.Equal(one[0], eight[0]) {
				t.Fatal("1 band and 8 bands differ")
			}
			if !bytes.Equal(one[0][:4], []byte{0x20, 0x20, 0x20, 255}) {
				t.Fatalf("corner %v, want the opaque background", one[0][:4])
			}
			for id, it := range c.doc.Items {
				it.Shadow, it.Outline = nil, nil
				c.doc.Items[id] = it
			}
			plain := renderDoc(t, c.doc, name+"-plain", 1, c.frames)
			if bytes.Equal(one[0], plain[0]) {
				t.Fatal("removing the shadow and the outline changed no pixel")
			}
		})
	}
}

// A transparent project leaves the uncovered corner at alpha 0 in the composed frame, in a PNG of
// those pixels and in the GIF export. The opaque control stays opaque, including its GIF.
func TestTransparentExportLeavesUncoveredPixels(t *testing.T) {
	setup(t)
	render := func(name string) (pix []byte, doc *timeline.Document) {
		t.Helper()
		c := alphaCases[name]()
		for id, spec := range c.fixtures {
			useAsset(c.doc, id, ingestClip(t, spec))
		}
		one := renderDoc(t, c.doc, name, 1, c.frames)
		eight := renderDoc(t, c.doc, name, 8, c.frames)
		if len(one) != 1 || !bytes.Equal(one[0], eight[0]) {
			t.Fatalf("%s: 1 band and 8 bands differ", name)
		}
		return one[0], c.doc
	}
	pngCorner := func(pix []byte, doc *timeline.Document) []byte {
		t.Helper()
		w, h := doc.Settings.Width, doc.Settings.Height
		var buf bytes.Buffer
		if err := png.Encode(&buf, &image.RGBA{
			Pix: pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h),
		}); err != nil {
			t.Fatal(err)
		}
		im, err := png.Decode(&buf)
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, a := im.At(0, 0).RGBA()
		return []byte{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
	}
	gifCorner := func(name string, doc *timeline.Document) (alpha uint32, disposal byte) {
		t.Helper()
		job := newJob(t, doc, "gif-"+name, name+".gif")
		job.Format = FormatGIF
		res, err := Run(context.Background(), job)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Remove(res.Path) }()
		raw, err := os.ReadFile(res.Path)
		if err != nil {
			t.Fatal(err)
		}
		dec, err := gif.DecodeAll(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, a := dec.Image[0].At(0, 0).RGBA()
		return a, dec.Disposal[0]
	}

	pix, doc := render("transparent")
	if !bytes.Equal(pix[:4], []byte{0, 0, 0, 0}) {
		t.Fatalf("composed corner %v, want alpha 0", pix[:4])
	}
	if got := pngCorner(pix, doc); !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Fatalf("png corner %v, want alpha 0", got)
	}
	if a, disposal := gifCorner("transparent", doc); a != 0 || disposal != gif.DisposalBackground {
		t.Fatalf("gif alpha %d disposal %d, want 0 and restore to background", a, disposal)
	}

	pix, doc = render("opaque-control")
	if !bytes.Equal(pix[:4], []byte{0x20, 0x20, 0x20, 255}) {
		t.Fatalf("opaque composed corner %v", pix[:4])
	}
	if got := pngCorner(pix, doc); !bytes.Equal(got, []byte{0x20, 0x20, 0x20, 255}) {
		t.Fatalf("opaque png corner %v", got)
	}
	if a, disposal := gifCorner("opaque-control", doc); a>>8 != 255 || disposal != gif.DisposalNone {
		t.Fatalf("opaque gif alpha %d disposal %d, want opaque and do not dispose", a, disposal)
	}
}

// The encoder that detection picks on this machine (a hardware one where there is one) also produces a
// file that passes verification.
func TestExportWithTheDetectedEncoder(t *testing.T) {
	setup(t)
	doc := loadScene(t, "contain-text")
	r, err := ffmpeg.NewRunner(shared.runner.In, shared.dir)
	if err != nil {
		t.Fatal(err)
	}
	job := newJob(t, doc, "export-detected", filepath.Join("detected", "out.mp4"))
	job.Runner, job.Encoder = r, ffmpeg.DetectOptions{}
	res, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("detected encoder: %s", res.Encoder)
}
