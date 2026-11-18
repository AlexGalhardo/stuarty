package frame

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/cache"
	"github.com/AlexGalhardo/vukem-video/engine/compositor"
	"github.com/AlexGalhardo/vukem-video/engine/export"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
	"github.com/AlexGalhardo/vukem-video/engine/rasterize"
)

func newFrames(t *testing.T) *Frames {
	t.Helper()
	setup(t)
	c, err := cache.Open(filepath.Join(t.TempDir(), "cache"), 0)
	if err != nil {
		t.Fatal(err)
	}
	return &Frames{Runner: shared.runner, ArtifactsDir: artifactsDir(), Cache: c}
}

func decodeJPEG(t *testing.T, b []byte) *image.RGBA {
	t.Helper()
	im, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	out := image.NewRGBA(im.Bounds())
	for y := range out.Rect.Dy() {
		for x := range out.Rect.Dx() {
			r, g, bl, _ := im.At(x, y).RGBA()
			o := out.PixOffset(x, y)
			out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = uint8(
				r>>8,
			), uint8(
				g>>8,
			), uint8(
				bl>>8,
			), 255
		}
	}
	return out
}

func luma(img *image.RGBA) []byte {
	out := make([]byte, len(img.Pix)/4)
	for i := range out {
		r, g, b := int(img.Pix[i*4]), int(img.Pix[i*4+1]), int(img.Pix[i*4+2])
		out[i] = byte((2126*r + 7152*g + 722*b + 5000) / 10000)
	}
	return out
}

// ssim is the mean structural similarity of two luma planes over 8x8 windows every 4 pixels.
func ssim(a, b []byte, w, h int) float64 {
	const (
		c1 = (0.01 * 255) * (0.01 * 255)
		c2 = (0.03 * 255) * (0.03 * 255)
		n  = 64.0
	)
	var sum float64
	var count int
	for y := 0; y+8 <= h; y += 4 {
		for x := 0; x+8 <= w; x += 4 {
			var sa, sb, saa, sbb, sab float64
			for j := range 8 {
				row := (y+j)*w + x
				for i := range 8 {
					va, vb := float64(a[row+i]), float64(b[row+i])
					sa, sb, saa, sbb, sab = sa+va, sb+vb, saa+va*va, sbb+vb*vb, sab+va*vb
				}
			}
			ma, mb := sa/n, sb/n
			va, vb, cov := saa/n-ma*ma, sbb/n-mb*mb, sab/n-ma*mb
			sum += (2*ma*mb + c1) * (2*cov + c2) / ((ma*ma + mb*mb + c1) * (va + vb + c2))
			count++
		}
	}
	return sum / float64(count)
}

// NFR-022: by the arithmetic of SDD-003 section 3.11 a frame at the width the service derives costs at most
// 800 tokens and a default sheet at most 1,600, computed from the size of the JPEG.
func TestImageTokenBudgets(t *testing.T) {
	f := newFrames(t)
	doc := sceneDoc(t)
	ref := Ref{Project: doc.ID, Revision: 1}
	ctx := context.Background()

	w, _, _ := ImageSize(doc.Settings.Width, doc.Settings.Height, DefaultFrameBudg)
	b, err := f.Frame(ctx, doc, ref, 20, w)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if tok := Tokens(cfg.Width, cfg.Height); tok > 800 {
		t.Errorf("a frame of %dx%d costs %d tokens, want at most 800", cfg.Width, cfg.Height, tok)
	}

	b, err = f.Frame(ctx, doc, ref, 20, DefaultAgentWidth)
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err = jpeg.DecodeConfig(bytes.NewReader(b)); err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 384 || Tokens(cfg.Width, cfg.Height) > 800 {
		t.Errorf("the agent frame is %dx%d", cfg.Width, cfg.Height)
	}

	sh, err := f.Sheet(ctx, doc, ref, SheetRequest{Times: SheetTimes(0, 89, DefaultSheetCells)})
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err = jpeg.DecodeConfig(bytes.NewReader(sh.JPEG)); err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 804 || cfg.Height != 1428 || Tokens(cfg.Width, cfg.Height) != 1479 ||
		Tokens(cfg.Width, cfg.Height) > 1600 {
		t.Errorf("the sheet is %dx%d (%d tokens)", cfg.Width, cfg.Height, Tokens(cfg.Width, cfg.Height))
	}
	if len(sh.Cells) != 9 || sh.Cells[0].Index != 1 || sh.Cells[8].Frame != 89 {
		t.Errorf("cells %+v", sh.Cells)
	}
}

// NFR-023: a frame is returned within 2 s cold and within 50 ms from the cache; the file is where SDD-000
// section 9 says.
func TestFrameColdAndCached(t *testing.T) {
	f := newFrames(t)
	doc := sceneDoc(t)
	ref := Ref{Project: doc.ID, Revision: 7}

	start := time.Now()
	cold, err := f.Frame(context.Background(), doc, ref, 40, DefaultAgentWidth)
	coldT := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	warm, err := f.Frame(context.Background(), doc, ref, 40, DefaultAgentWidth)
	warmT := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cold %s, cached %s, %d bytes", coldT, warmT, len(cold))
	if coldT > 2*time.Second {
		t.Errorf("cold frame took %s, want at most 2 s", coldT)
	}
	if warmT > 50*time.Millisecond {
		t.Errorf("cached frame took %s, want at most 50 ms", warmT)
	}
	if !bytes.Equal(cold, warm) {
		t.Error("the cached frame differs")
	}
	if _, ok := f.Cache.Get(cache.FrameKey(doc.ID, 7, 40, DefaultAgentWidth)); !ok {
		t.Error("the frame is not under frames/<project>/<revision>/<frame>-<width>.jpg")
	}
	if _, err := f.Frame(context.Background(), doc, ref, -1, 384); err == nil {
		t.Error("a negative time was accepted")
	}
	if _, err := f.Frame(context.Background(), doc, ref, 1, 5000); err == nil {
		t.Error("a width over the maximum was accepted")
	}
}

// The frame at t is frame t of the exported file (SDD-004 section 11: SSIM at least 0.95 on luma).
func TestFrameEqualsTheExportedFrame(t *testing.T) {
	f := newFrames(t)
	doc := sceneDoc(t)
	out := filepath.Join(shared.dir, "exports", "frames-vs-export.mp4")
	res, err := export.Run(context.Background(), export.Job{
		Runner: shared.runner, Caps: shared.caps, Doc: doc, JobID: "frames-export", ArtifactsDir: artifactsDir(),
		OutPath: out, Encoder: ffmpeg.DetectOptions{Forced: "libopenh264"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []int{0, 30, 60, 89} {
		b, err := f.Frame(context.Background(), doc, Ref{Project: doc.ID, Revision: 1}, at, MaxWidth)
		if err != nil {
			t.Fatal(err)
		}
		mine := decodeJPEG(t, b)
		theirs := decodeExported(t, res.Path, doc, at)
		if s := ssim(luma(mine), luma(theirs), mine.Rect.Dx(), mine.Rect.Dy()); s < 0.95 {
			t.Errorf("frame %d: SSIM %.4f against the exported file, want at least 0.95", at, s)
		}
	}
}

func decodeExported(t *testing.T, file string, doc *timeline.Document, at int) *image.RGBA {
	t.Helper()
	r := shared.runner
	dir, err := r.NewRunDir("frames-decode-" + strconv.FormatInt(time.Now().UnixNano(), 36))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Remove() }()
	st := doc.Settings
	inDir := filepath.Dir(file)
	argv, err := ffargs.Cmd{
		Inputs: []ffargs.Input{ffargs.FileInput(file).At(int64(at) * 1000 * int64(st.FPS.Den) / int64(st.FPS.Num))},
		Outputs: []ffargs.Output{{
			Format: "rawvideo", Pipe: true, Maps: []string{"0:v:0"}, NoAudio: true,
			VF:   ffargs.Chain1(ffargs.ScaleFrameToRGBA(st.Width, st.Height), ffargs.Format("rgba")),
			Opts: []ffargs.Opt{ffargs.Int("-frames:v", 1), ffargs.Enum("-pix_fmt", "rgba")},
		}},
	}.Argv(r.Policy(dir, inDir))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := r.RunFFmpeg(context.Background(), ffmpeg.Request{
		Dir: dir, Argv: argv, InputDirs: []string{inDir}, Deadline: time.Minute, Stdout: &buf,
	}); err != nil {
		t.Fatal(err)
	}
	return &image.RGBA{Pix: buf.Bytes(), Stride: st.Width * 4, Rect: image.Rect(0, 0, st.Width, st.Height)}
}

// A small frame has crisp text, not the shimmer of point-sampling a full-size bitmap: it matches the full
// frame box-filtered down to the same size.
func TestSmallFrameMatchesTheDownscaledFullFrame(t *testing.T) {
	f := newFrames(t)
	doc := sceneDoc(t)
	ctx := context.Background()
	at := 40 // inside the caption: 12 to 60

	small, err := f.Frame(ctx, doc, Ref{Project: doc.ID, Revision: 1}, at, DefaultAgentWidth)
	if err != nil {
		t.Fatal(err)
	}
	full, err := f.Frame(ctx, doc, Ref{Project: doc.ID, Revision: 1}, at, MaxWidth)
	if err != nil {
		t.Fatal(err)
	}
	a := decodeJPEG(t, small)
	b := decodeJPEG(t, full)
	ref := compositor.Resize(
		compositor.Image{Pix: b.Pix, W: b.Rect.Dx(), H: b.Rect.Dy(), Opaque: true},
		a.Rect.Dx(),
		a.Rect.Dy(),
	)
	if s := ssim(
		luma(a),
		luma(&image.RGBA{Pix: ref.Pix, Rect: image.Rect(0, 0, ref.W, ref.H)}),
		a.Rect.Dx(),
		a.Rect.Dy(),
	); s < 0.93 {
		t.Errorf("SSIM %.4f of the 384 wide frame against the downscaled full frame, want at least 0.93", s)
	}
}

// TestSheetLabelGolden pins the pixels of the label of a cell (SDD-003 section 3.11 item 3): the same
// bytes on every machine.
func TestSheetLabelGolden(t *testing.T) {
	t.Parallel()
	canvas := image.NewRGBA(image.Rect(0, 0, 268, 476))
	for i := 3; i < len(canvas.Pix); i += 4 {
		canvas.Pix[i] = 255
		canvas.Pix[i-1] = 200
	}
	if err := label(canvas, canvas.Bounds(), rasterize.Bundled(), Cell{Index: 3, Frame: 30, Seconds: 1}); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canvas.Pix)
	const want = "17736f22a789956ffdbf0d1206b8a0992fb126de4f28045ece7fdad39521623a"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("label hash %s", got)
	}
	// The box darkens the blue background to about 30% of it (60), and the text is white.
	dark := 0
	for y := range 40 {
		for x := range 200 {
			if v := canvas.Pix[canvas.PixOffset(x, y)+2]; v > 40 && v < 80 {
				dark++
			}
		}
	}
	if dark < 200 {
		t.Errorf("only %d pixels of the box at 70%% opacity", dark)
	}
	bright := false
	for y := range 20 {
		for x := range 80 {
			if canvas.Pix[canvas.PixOffset(x, y)] > 230 {
				bright = true
			}
		}
	}
	if !bright {
		t.Error("no white text in the label")
	}
}

func TestSecondsText(t *testing.T) {
	t.Parallel()
	for in, want := range map[float64]string{1: "1", 0.5: "0.5", 1.0333333: "1.03", 0: "0", 12.999: "13"} {
		if got := secondsText(in); got != want {
			t.Errorf("%v: %q, want %q", in, got, want)
		}
	}
}

// BenchmarkViewSheet9 is the measure of NFR-023 for a nine-cell sheet of the reference scene (the 200-item
// project of SDD-003 section 3.11 is the service's benchmark; this one covers the engine alone).
func BenchmarkViewSheet9(b *testing.B) {
	setup(b)
	doc := sceneDoc(b)
	f := &Frames{Runner: shared.runner, ArtifactsDir: artifactsDir()}
	for range b.N {
		if _, err := f.Sheet(
			context.Background(),
			doc,
			Ref{Project: doc.ID, Revision: 1},
			SheetRequest{Times: SheetTimes(0, 89, 9)},
		); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkViewFrame is NFR-023 for a frame: one frame at the agent width, cold (no cache).
func BenchmarkViewFrame(b *testing.B) {
	setup(b)
	doc := sceneDoc(b)
	f := &Frames{Runner: shared.runner, ArtifactsDir: artifactsDir()}
	for range b.N {
		if _, err := f.Frame(
			context.Background(),
			doc,
			Ref{Project: doc.ID, Revision: 1},
			40,
			DefaultAgentWidth,
		); err != nil {
			b.Fatal(err)
		}
	}
}

// A still is a PNG of the project's own size whose pixels are those of the compositor.
func TestStillIsTheProjectSizePNG(t *testing.T) {
	f := newFrames(t)
	doc := sceneDoc(t)

	b, err := f.Still(context.Background(), doc, 40)
	if err != nil {
		t.Fatal(err)
	}

	im, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}

	st := doc.Settings
	if got := im.Bounds().Size(); got.X != st.Width || got.Y != st.Height {
		t.Errorf("still is %v, want %dx%d", got, st.Width, st.Height)
	}

	r, err := f.renderer(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	want, err := r.Render(context.Background(), 40, 1)
	if err != nil {
		t.Fatal(err)
	}

	got := image.NewRGBA(im.Bounds())
	for y := range got.Rect.Dy() {
		for x := range got.Rect.Dx() {
			c := im.At(x, y)
			rr, g, bl, a := c.RGBA()
			o := got.PixOffset(x, y)
			got.Pix[o], got.Pix[o+1], got.Pix[o+2], got.Pix[o+3] = uint8(rr>>8), uint8(g>>8), uint8(bl>>8), uint8(a>>8)
		}
	}

	if !bytes.Equal(got.Pix, want.Pix) {
		t.Error("the still differs from the composed frame")
	}

	if _, err := f.Still(context.Background(), doc, -1); err == nil {
		t.Error("a negative time was accepted")
	}

	if _, err := f.Still(context.Background(), doc, 1<<20); err != nil {
		t.Errorf("a time past the end: %v", err)
	}
}

// Still encodes the compositor's pixels. A transparent project keeps an uncovered pixel at alpha 0;
// an opaque project keeps it opaque.
func TestStillKeepsATransparentBackground(t *testing.T) {
	f := newFrames(t)
	st := timeline.DefaultSettings()
	st.Width, st.Height = 32, 32
	st.Background = timeline.BackgroundTransparent
	doc := &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion,
		ID:            "prj_gg00000000",
		Name:          "transparent still",
		Settings:      st,
		Assets:        map[string]timeline.Asset{},
		Tracks:        map[string]timeline.Track{},
		Items:         map[string]timeline.Item{},
	}
	corner := func(background timeline.Color) []byte {
		t.Helper()
		doc.Settings.Background = background
		b, err := f.Still(context.Background(), doc, 0)
		if err != nil {
			t.Fatal(err)
		}
		im, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		r, g, bl, a := im.At(0, 0).RGBA()
		return []byte{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)}
	}
	if got := corner(timeline.BackgroundTransparent); !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Fatalf("transparent still corner %v", got)
	}
	if got := corner("#202020"); !bytes.Equal(got, []byte{0x20, 0x20, 0x20, 255}) {
		t.Fatalf("opaque still corner %v", got)
	}
}
