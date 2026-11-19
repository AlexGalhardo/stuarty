package export

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

func formatDir(name string) string { return filepath.Join("testdata", "formats", name) }

func TestFormatGIFTiming(t *testing.T) {
	t.Parallel()

	cases := []struct {
		fps   timeline.Rational
		n     int
		step  int
		count int
		total int // hundredths of a second
	}{
		{timeline.Rational{Num: 30, Den: 1}, 60, 2, 30, 200},
		{timeline.Rational{Num: 24, Den: 1}, 48, 2, 24, 200},
		{timeline.Rational{Num: 60, Den: 1}, 300, 4, 75, 500},
		{timeline.Rational{Num: 30000, Den: 1001}, 60, 2, 30, 200},
		{timeline.Rational{Num: 30, Den: 1}, 61, 2, 31, 203},
	}
	for _, c := range cases {
		if got := gifStep(c.fps); got != c.step {
			t.Errorf("%v: step %d, want %d", c.fps, got, c.step)
		}

		d := gifDelaysCS(c.fps, c.n)
		if len(d) != c.count {
			t.Errorf("%v: %d frames, want %d", c.fps, len(d), c.count)
		}

		sum := 0
		for _, x := range d {
			if x < 2 {
				t.Errorf("%v: a delay of %d", c.fps, x)
			}

			sum += x
		}

		if sum < c.total-1 || sum > c.total+1 {
			t.Errorf("%v: total %d, want %d", c.fps, sum, c.total)
		}
	}
}

// The frames the writer makes are read back by the standard decoder: size, frame count, delays, loop
// and the colour of a flat frame (the palette is a 6x6x6 cube, so a multiple of 51 is exact).
func TestFormatGIFWriterDecodes(t *testing.T) {
	t.Parallel()

	const w, h = 33, 17

	flat := func(c color.RGBA) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}

		return img
	}
	colours := []color.RGBA{{255, 0, 0, 255}, {0, 102, 204, 255}, {51, 51, 51, 255}}
	write := func() []byte {
		var b bytes.Buffer

		g := newGIFWriter(&b, w, h, 0, false)
		for i, c := range colours {
			if err := g.frame(flat(c), 3+i); err != nil {
				t.Fatal(err)
			}
		}

		if err := g.finish(); err != nil {
			t.Fatal(err)
		}

		return b.Bytes()
	}

	a, b := write(), write()
	if !bytes.Equal(a, b) {
		t.Fatal("two runs gave different bytes")
	}

	dec, err := gif.DecodeAll(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}

	if len(dec.Image) != 3 || !slices.Equal(dec.Delay, []int{3, 4, 5}) || dec.LoopCount != 0 {
		t.Fatalf("%d frames, delays %v, loop %d", len(dec.Image), dec.Delay, dec.LoopCount)
	}

	if dec.Config.Width != w || dec.Config.Height != h {
		t.Errorf("size %dx%d", dec.Config.Width, dec.Config.Height)
	}

	for i, fr := range dec.Image {
		r, g, bl, _ := fr.At(w/2, h/2).RGBA()
		got := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), 255}
		if got != colours[i] {
			t.Errorf("frame %d: colour %v, want %v", i, got, colours[i])
		}
	}
}

// A transparent GIF reserves index 255, restores to that background before each frame, and does not
// keep a previous colour in a hole. An opaque GIF keeps disposal 1 and background index 0.
func TestFormatGIFTransparentHolesStayClear(t *testing.T) {
	t.Parallel()

	const w, h = 4, 4
	solid := func(c color.RGBA) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
		}
		return img
	}
	write := func(transparent bool, frames ...*image.RGBA) []byte {
		t.Helper()
		var b bytes.Buffer
		g := newGIFWriter(&b, w, h, 0, transparent)
		for _, fr := range frames {
			if err := g.frame(fr, 4); err != nil {
				t.Fatal(err)
			}
		}
		if err := g.finish(); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}

	red := solid(color.RGBA{255, 0, 0, 255})
	hole := image.NewRGBA(image.Rect(0, 0, w, h))
	hole.SetRGBA(1, 1, color.RGBA{0, 255, 0, 255})
	raw := write(true, red, hole)
	if raw[11] != 255 {
		t.Fatalf("background index %d, want 255", raw[11])
	}
	dec, err := gif.DecodeAll(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if dec.Disposal[0] != gif.DisposalBackground || dec.Disposal[1] != gif.DisposalBackground {
		t.Fatalf("disposal %v, want restore to background", dec.Disposal)
	}
	if _, _, _, a := dec.Image[1].At(0, 0).RGBA(); a != 0 {
		t.Fatalf("a hole over the previous red has alpha %d", a)
	}
	if _, _, _, a := dec.Image[1].At(1, 1).RGBA(); a == 0 {
		t.Fatal("the opaque pixel of the second frame was stored as a hole")
	}

	opaque := write(false, red)
	if opaque[11] != 0 {
		t.Fatalf("opaque background index %d, want 0", opaque[11])
	}
	one, err := gif.DecodeAll(bytes.NewReader(opaque))
	if err != nil {
		t.Fatal(err)
	}
	if one.Disposal[0] != gif.DisposalNone {
		t.Fatalf("opaque disposal %d, want do not dispose", one.Disposal[0])
	}
}

func TestFormatGIFWriterSizeLimit(t *testing.T) {
	t.Parallel()

	var b bytes.Buffer

	g := newGIFWriter(&b, 64, 64, 900, false)
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 7)
	}

	var err error
	for range 20 {
		if err = g.frame(img, 4); err != nil {
			break
		}
	}

	if !errors.Is(err, errGIFLimit) || g.n <= 900 {
		t.Fatalf("err = %v after %d bytes, want errGIFLimit past 900", err, g.n)
	}
}

// A GIF export of a 2 s project has one frame per gifStep timeline frames, an infinite loop and the
// planned delays, passes its verification, and two runs give the same bytes (NFR-010).
func TestFormatGIFExport(t *testing.T) {
	doc := loadDir(t, formatDir("gif2s"))

	export := func(id string) (*Result, []byte) {
		job := newJob(t, doc, id, id+".gif")
		job.Format = FormatGIF

		res, err := Run(context.Background(), job)
		if err != nil {
			t.Fatal(err)
		}

		b, err := os.ReadFile(res.Path)
		if err != nil {
			t.Fatal(err)
		}

		return res, b
	}

	res, a := export("format-gif-a")
	_, b := export("format-gif-b")

	if !bytes.Equal(a, b) {
		t.Error("two exports of the same project gave different bytes")
	}

	if res.Format != FormatGIF || res.Encoder != "gif" || res.DurationMS != 2000 {
		t.Errorf("result %+v", res)
	}

	dec, err := gif.DecodeAll(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}

	if len(dec.Image) != 30 || dec.LoopCount != 0 || dec.Config.Width != 180 || dec.Config.Height != 320 {
		t.Fatalf("%d frames, loop %d, %dx%d", len(dec.Image), dec.LoopCount, dec.Config.Width, dec.Config.Height)
	}

	if !slices.Equal(dec.Delay, gifDelaysCS(doc.Settings.FPS, endFrame(doc))) {
		t.Errorf("delays %v", dec.Delay)
	}
}

func TestGIFSizeIsCapped(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		w, h, wantW, wantH int
	}{
		{720, 1280, 480, 853},
		{1920, 1080, 480, 270},
		{480, 480, 480, 480},
		{180, 320, 180, 320},
	} {
		w, h, scale := gifSize(timeline.Settings{Width: c.w, Height: c.h})
		if w != c.wantW || h != c.wantH {
			t.Errorf("%dx%d: %dx%d, want %dx%d", c.w, c.h, w, h, c.wantW, c.wantH)
		}

		if c.w <= gifMaxWidth && scale != 1 {
			t.Errorf("%dx%d: scale %v, want 1", c.w, c.h, scale)
		}
	}
}

// A project wider than the cap is composed at the cap, and the file passes its own verification.
func TestFormatGIFExportOfAWideProject(t *testing.T) {
	doc := loadDir(t, formatDir("gif2s"))
	doc.Settings.Width, doc.Settings.Height = 960, 540

	job := newJob(t, doc, "format-gif-wide", "format-gif-wide.gif")
	job.Format = FormatGIF

	res, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}

	if res.Width != 480 || res.Height != 270 {
		t.Errorf("result size %dx%d, want 480x270", res.Width, res.Height)
	}

	f, err := os.Open(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	cfg, err := gif.DecodeConfig(f)
	if err != nil || cfg.Width != 480 || cfg.Height != 270 {
		t.Errorf("file %dx%d, %v", cfg.Width, cfg.Height, err)
	}
}

func TestFormatGIFSizeLimit(t *testing.T) {
	doc := loadDir(t, formatDir("gif2s"))
	job := newJob(t, doc, "format-gif-limit", filepath.Join("gif-limit", "out.gif"))
	job.Format, job.MaxBytes = FormatGIF, 5000

	_, err := Run(context.Background(), job)
	ee := asFFError(t, err)

	if ee.Code != CodeExportFailed || ee.Details["reason"] != ReasonSizeLimit {
		t.Fatalf("error = %v, want size_limit", err)
	}

	noLeftovers(t, job.JobID, job.OutPath)
}

// A GIF without its trailer fails the block walk of the verification.
func TestFormatGIFVerifyRefusesTruncation(t *testing.T) {
	t.Parallel()

	var b bytes.Buffer

	g := newGIFWriter(&b, 16, 16, 0, false)
	if err := g.frame(image.NewRGBA(image.Rect(0, 0, 16, 16)), 4); err != nil {
		t.Fatal(err)
	}

	if err := g.finish(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "x.gif")
	if err := os.WriteFile(path, b.Bytes()[:b.Len()-1], 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := scanGIF(path); err == nil {
		t.Fatal("a GIF without its trailer was accepted")
	}
}

func probeStreams(t *testing.T, path string) (streams []map[string]any, formatDuration float64) {
	t.Helper()

	dir, err := shared.runner.NewRunDir("format-probe-" + strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = dir.Remove() })

	argv, err := ffargs.ProbeJSON(shared.runner.Policy(dir, filepath.Dir(path)), path)
	if err != nil {
		t.Fatal(err)
	}

	raw, _, err := shared.runner.RunFFprobe(context.Background(), dir, argv, verifyTimeout, filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}

	var pf struct {
		Streams []map[string]any `json:"streams"`
		Format  struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(raw, &pf); err != nil {
		t.Fatal(err)
	}

	d, err := strconv.ParseFloat(pf.Format.Duration, 64)
	if err != nil {
		t.Fatal(err)
	}

	return pf.Streams, d
}

// A 60 fps export of the 5 s project has avg_frame_rate 60/1 and passes the verification, which
// requires 300 packets and both durations within one frame.
func TestFormat60FPS(t *testing.T) {
	doc := loadDir(t, formatDir("fps60"))
	job := newJob(t, doc, "format-60", "format-60.mp4")

	res, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}

	if res.Frames != 300 {
		t.Errorf("%d frames, want 300", res.Frames)
	}

	streams, _ := probeStreams(t, res.Path)

	seen := false

	for _, s := range streams {
		if s["codec_type"] == "video" {
			seen = true

			if s["avg_frame_rate"] != "60/1" {
				t.Errorf("avg_frame_rate %v, want 60/1", s["avg_frame_rate"])
			}
		}
	}

	if !seen {
		t.Error("no video stream")
	}
}

func TestFormatBitrateScalesWithSizeAndRate(t *testing.T) {
	t.Parallel()

	fps30, fps60 := timeline.Rational{Num: 30, Den: 1}, timeline.Rational{Num: 60, Den: 1}

	if got := bitrateK(8, 12, 1080, 1920, fps30); got != 8000 {
		t.Errorf("1080x1920 at 30: %d", got)
	}

	if got := bitrateK(8, 12, 1080, 1920, fps60); got != 12000 {
		t.Errorf("1080x1920 at 60: %d", got)
	}

	if got := bitrateK(8, 12, 3840, 2160, fps60); got != 12000*3840*2160/(1080*1920) {
		t.Errorf("4K at 60: %d", got)
	}

	if got := bitrateK(8, 12, 180, 320, fps30); got != 2000 {
		t.Errorf("180x320: %d, want the 2M floor", got)
	}
}

// A project over the ceilings is refused before anything is encoded.
func TestFormatLimits(t *testing.T) {
	t.Parallel()

	for _, st := range []timeline.Settings{
		{Width: 4098, Height: 2160, FPS: timeline.Rational{Num: 30, Den: 1}},
		{Width: 1080, Height: 1920, FPS: timeline.Rational{Num: 121, Den: 1}},
	} {
		if checkLimits(st) == nil {
			t.Errorf("%dx%d at %v passed", st.Width, st.Height, st.FPS)
		}
	}

	if checkLimits(timeline.Settings{Width: 4096, Height: 4096, FPS: timeline.Rational{Num: 120, Den: 1}}) != nil {
		t.Error("the ceilings themselves were refused")
	}
}

// 4K on the encoder of the bundled build either passes the verification, or fails with the documented
// error that names the encoder, the size and what to do. Both are right; anything else is not.
func TestFormat4K(t *testing.T) {
	doc := loadDir(t, formatDir("fps60"))
	doc.Settings.Width, doc.Settings.Height = 3840, 2160
	doc.Settings.FPS = timeline.Rational{Num: 30, Den: 1}

	it := doc.Items["itm_m1n2p3q4r5"]
	it.Duration = 15
	doc.Items["itm_m1n2p3q4r5"] = it

	job := newJob(t, doc, "format-4k", "format-4k.mp4")

	res, err := Run(context.Background(), job)
	if err != nil {
		ee := asFFError(t, err)
		if ee.Details["reason"] != ReasonUnsupportedSize || !strings.Contains(ee.Message, "3840x2160") ||
			ee.Hint == "" {
			t.Fatalf("error = %v, want the unsupported_size error", err)
		}

		t.Skipf("the encoder cannot do 4K here: %v", err)
	}

	streams, _ := probeStreams(t, res.Path)

	for _, s := range streams {
		if s["codec_type"] == "video" && (s["width"] != float64(3840) || s["height"] != float64(2160)) {
			t.Errorf("size %vx%v", s["width"], s["height"])
		}
	}
}

// An encoder that is not on the machine cannot encode any size: the capability test says so with the
// message and the hint of the export.
func TestFormatCapabilityFailure(t *testing.T) {
	doc := loadDir(t, formatDir("fps60"))
	job := newJob(t, doc, "format-cap", "format-cap.mp4")

	s := testState(t, job, job.JobID)
	s.job.Doc.Settings.Width, s.job.Doc.Settings.Height = 3840, 2160

	for _, name := range []string{"h264_videotoolbox", "h264_vaapi", "h264_qsv", "h264_amf", "h264_nvenc"} {
		cand := ffmpeg.Candidate{
			Name:   name,
			Status: ffmpeg.EncoderOK,
			Input:  ffargs.DetectInput{Filter: "format=yuv420p"},
		}

		det := &ffmpeg.EncoderDetection{Chosen: name, Candidates: []ffmpeg.Candidate{cand}}
		_, err := s.checkCapability(context.Background(), det)
		if err == nil {
			continue
		}

		ee := asFFError(t, err)
		if ee.Code != CodeExportFailed || ee.Details["reason"] != ReasonUnsupportedSize ||
			!strings.Contains(ee.Message, name) || !strings.Contains(ee.Message, "3840x2160 at 60 fps") ||
			!strings.Contains(ee.Hint, "Lower the project size") || !errors.Is(err, ffmpeg.ErrEncoderSize) {
			t.Fatalf("error = %#v", ee)
		}

		return
	}

	t.Skip("every hardware encoder works here")
}

// An audio only export has one audio stream, no video stream and a duration within one frame of the
// timeline; WAV and AAC both pass their verification.
func TestFormatAudioOnly(t *testing.T) {
	doc := loadDir(t, formatDir("audio"))
	want := float64(endFrame(doc)) / 30

	for _, c := range []struct {
		format Format
		codec  string
	}{{FormatWAV, "pcm_s16le"}, {FormatAAC, "aac"}} {
		t.Run(string(c.format), func(t *testing.T) {
			id := "format-audio-" + string(c.format)
			job := newJob(t, doc, id, id+c.format.Ext())
			job.Format = c.format

			var last Progress

			job.OnProgress = func(p Progress) { last = p }

			res, err := Run(context.Background(), job)
			if err != nil {
				t.Fatal(err)
			}

			if res.Format != c.format || res.Encoder != c.codec || last.Frame != last.Total || last.Total == 0 {
				t.Errorf("result %+v, progress %+v", res, last)
			}

			streams, dur := probeStreams(t, res.Path)
			if len(streams) != 1 || streams[0]["codec_type"] != "audio" || streams[0]["codec_name"] != c.codec {
				t.Fatalf("streams %v", streams)
			}

			if dur < want-1.0/30 || dur > want+1.0/30 {
				t.Errorf("duration %v, want %.3f within one frame", dur, want)
			}
		})
	}
}

// A WAV that lost its tail fails verification.
func TestFormatWAVVerifyRefusesTruncation(t *testing.T) {
	doc := loadDir(t, formatDir("audio"))
	job := newJob(t, doc, "format-wav-trunc", "unused.wav")
	s := testState(t, job, job.JobID)

	mix, _, err := s.mixAudio(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := checkWAVComplete(mix); err != nil {
		t.Fatalf("a whole WAV: %v", err)
	}

	b, err := os.ReadFile(mix)
	if err != nil {
		t.Fatal(err)
	}

	cut := filepath.Join(filepath.Dir(mix), "cut.wav")
	if err := os.WriteFile(cut, b[:len(b)-100], 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.verifyAudio(context.Background(), cut, FormatWAV); err == nil {
		t.Fatal("a truncated WAV passed")
	}
}

func TestFormatUnknownIsRefused(t *testing.T) {
	doc := loadDir(t, formatDir("gif2s"))
	job := newJob(t, doc, "format-bad", "format-bad.webm")
	job.Format = "webm"

	_, err := Run(context.Background(), job)
	if ee := asFFError(t, err); ee.Details["reason"] != ReasonBadFormat {
		t.Fatalf("error = %v", err)
	}
}

// The estimate follows the size and frame rate of the export and brackets what the writer makes.
func TestEstimateGIF(t *testing.T) {
	st := timeline.Settings{Width: 1920, Height: 1080, FPS: timeline.Rational{Num: 30, Den: 1}}

	e := EstimateGIF(st, 300)
	if e.Width != 480 || e.Height != 270 || e.Frames != 150 {
		t.Fatalf("size and frames = %dx%d, %d", e.Width, e.Height, e.Frames)
	}

	if e.TypicalBytes != int64(480*270*150*0.5) || e.WorstBytes <= e.TypicalBytes {
		t.Errorf("bytes = %d typical, %d worst", e.TypicalBytes, e.WorstBytes)
	}

	if z := EstimateGIF(st, 0); z.Frames != 0 || z.TypicalBytes != 0 {
		t.Errorf("no frames: %+v", z)
	}

	// Noise is the worst a picture gets: the bound holds for what the writer really produces.
	img := image.NewRGBA(image.Rect(0, 0, 480, 270))
	r := rand.New(rand.NewSource(1))

	for i := range img.Pix {
		img.Pix[i] = byte(r.Intn(256))
	}

	var b bytes.Buffer

	g := newGIFWriter(&b, 480, 270, 0, false)

	for range 3 {
		if err := g.frame(img, 6); err != nil {
			t.Fatal(err)
		}
	}

	if err := g.finish(); err != nil {
		t.Fatal(err)
	}

	if got, bound := int64(b.Len()), EstimateGIF(st, 6).WorstBytes; got > bound {
		t.Errorf("noise GIF is %d bytes, over the worst bound %d", got, bound)
	}
}
