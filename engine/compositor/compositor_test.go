package compositor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"math"
	"math/rand/v2"
	"testing"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
)

// pattern is an opaque image whose pixels are all different enough to expose a misplaced one.
func pattern(w, h int, seed uint64) Image {
	rng := rand.New(rand.NewPCG(seed, 7))
	pix := make([]byte, w*h*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = uint8(rng.UintN(256)), uint8(rng.UintN(256)), uint8(rng.UintN(256)), 255
	}
	return Image{Pix: pix, W: w, H: h, Opaque: true}
}

func media(id string, cx, cy, w, h, rot, op float64) plan.Layer {
	return plan.Layer{ItemID: id, Kind: plan.Media, CX: cx, CY: cy, W: w, H: h, Rotation: rot, Opacity: op}
}

func newComp(bands int, imgs map[string]Image) *Compositor {
	return &Compositor{
		Bands: bands,
		Media: func(l *plan.Layer) (Image, error) { return imgs[l.ItemID], nil },
	}
}

func at(img *image.RGBA, x, y int) [4]byte {
	o := (y*img.Rect.Dx() + x) * 4
	return [4]byte(img.Pix[o : o+4])
}

func TestCopyPlacesPixelsOneToOne(t *testing.T) {
	t.Parallel()
	src := pattern(8, 6, 1)
	c := newComp(1, map[string]Image{"a": src})
	dst := image.NewRGBA(image.Rect(0, 0, 20, 20))
	fp := plan.FramePlan{Background: "#102030", Layers: []plan.Layer{media("a", 10, 10, 8, 6, 0, 1)}}
	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}
	// The rectangle's corner is the centre minus half the size: column 6, row 7.
	for y := range 6 {
		for x := range 8 {
			want := [4]byte(src.Pix[(y*8+x)*4 : (y*8+x)*4+4])
			if got := at(dst, 6+x, 7+y); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
	if got := at(dst, 0, 0); got != [4]byte{0x10, 0x20, 0x30, 255} {
		t.Errorf("background = %v", got)
	}
}

func TestOpacityBlendsOverTheBackground(t *testing.T) {
	t.Parallel()
	white := Image{Pix: bytes.Repeat([]byte{255, 255, 255, 255}, 4), W: 2, H: 2, Opaque: true}
	c := newComp(1, map[string]Image{"a": white})
	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	fp := plan.FramePlan{Background: "#000000", Layers: []plan.Layer{media("a", 2, 2, 2, 2, 0, 0.5)}}
	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}
	// An opacity of one half is 128 of 255 after rounding.
	if got := at(dst, 1, 1); got != [4]byte{128, 128, 128, 255} {
		t.Errorf("half white over black = %v", got)
	}
	if got := at(dst, 0, 0); got != [4]byte{0, 0, 0, 255} {
		t.Errorf("outside = %v", got)
	}
}

func TestSamplerScalesAConstantImageExactly(t *testing.T) {
	t.Parallel()
	flat := Image{Pix: bytes.Repeat([]byte{10, 200, 90, 255}, 9), W: 3, H: 3, Opaque: true}
	c := newComp(1, map[string]Image{"a": flat})
	dst := image.NewRGBA(image.Rect(0, 0, 30, 30))
	fp := plan.FramePlan{Background: "#000000", Layers: []plan.Layer{media("a", 15, 15, 20.5, 14.25, 33, 1)}}
	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}
	if got := at(dst, 15, 15); got != [4]byte{10, 200, 90, 255} {
		t.Errorf("centre = %v", got)
	}
	if got := at(dst, 0, 0); got != [4]byte{0, 0, 0, 255} {
		t.Errorf("corner outside the rotated rectangle = %v", got)
	}
}

func TestQuarterTurnMovesTheTopLeftToTheTopRight(t *testing.T) {
	t.Parallel()
	// 2x2: red top-left, the rest black. A clockwise quarter turn puts the top-left at the top-right.
	pix := make([]byte, 16)
	for i := 3; i < 16; i += 4 {
		pix[i] = 255
	}
	pix[0] = 255
	c := newComp(1, map[string]Image{"a": {Pix: pix, W: 2, H: 2, Opaque: true}})
	dst := image.NewRGBA(image.Rect(0, 0, 40, 40))
	fp := plan.FramePlan{Background: "#000000", Layers: []plan.Layer{media("a", 20, 20, 20, 20, 90, 1)}}
	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}
	if r := at(dst, 27, 13)[0]; r < 200 {
		t.Errorf("red at the top-right of the turned square = %d", r)
	}
	if r := at(dst, 13, 13)[0]; r > 50 {
		t.Errorf("red at the old top-left = %d", r)
	}
}

func TestOutputScaleMultipliesPlacement(t *testing.T) {
	t.Parallel()
	flat := Image{Pix: bytes.Repeat([]byte{255, 0, 0, 255}, 100), W: 10, H: 10, Opaque: true}
	c := newComp(1, map[string]Image{"a": flat})
	dst := image.NewRGBA(image.Rect(0, 0, 20, 20))
	fp := plan.FramePlan{Background: "#000000", Layers: []plan.Layer{media("a", 20, 20, 20, 20, 0, 1)}}
	if err := c.Compose(dst, fp, 0.5); err != nil {
		t.Fatal(err)
	}
	if got := at(dst, 10, 10); got != [4]byte{255, 0, 0, 255} {
		t.Errorf("centre = %v", got)
	}
	if got := at(dst, 4, 4); got != [4]byte{0, 0, 0, 255} {
		t.Errorf("outside = %v", got)
	}
}

func TestColourMatrix(t *testing.T) {
	t.Parallel()
	src := Image{Pix: []byte{200, 100, 50, 255}, W: 1, H: 1, Opaque: true}
	c := newComp(1, map[string]Image{"a": src})

	run := func(m *[12]float64) [4]byte {
		dst := image.NewRGBA(image.Rect(0, 0, 3, 3))
		l := media("a", 1.5, 1.5, 1, 1, 0, 1)
		l.ColorMatrix = m
		if err := c.Compose(dst, plan.FramePlan{Background: "#000000", Layers: []plan.Layer{l}}, 1); err != nil {
			t.Fatal(err)
		}
		return at(dst, 1, 1)
	}

	identity := &[12]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0}
	if got := run(identity); got != [4]byte{200, 100, 50, 255} {
		t.Errorf("identity = %v", got)
	}
	// Saturation 0: every channel takes the luma 0.2126*200 + 0.7152*100 + 0.0722*50 = 117.65.
	gray := &[12]float64{0.2126, 0.7152, 0.0722, 0, 0.2126, 0.7152, 0.0722, 0, 0.2126, 0.7152, 0.0722, 0}
	if got := run(gray); got[0] != 118 || got[1] != 118 || got[2] != 118 {
		t.Errorf("desaturated = %v", got)
	}
	// An offset past 1 clamps.
	bright := &[12]float64{1, 0, 0, 0.5, 0, 1, 0, 0.5, 0, 0, 1, 0.5}
	if got := run(bright); got != [4]byte{255, 228, 178, 255} {
		t.Errorf("brightened = %v", got)
	}
}

func TestBitmapLayerIsCentredOnTheBlockAndPremultiplied(t *testing.T) {
	t.Parallel()
	spec := plan.BitmapSpec{
		Kind: rasterize.KindShape, ActiveWord: -1, PixelScale: 1,
		Shape: &plan.ShapeSpec{
			Shape: rasterize.ShapeRect, Width: 40, Height: 20, Fill: "#FF0000", StrokeColor: "#000000",
		},
	}
	bm, err := rasterize.Render(spec, rasterize.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	c := &Compositor{
		Bands:   1,
		Bitmaps: func(plan.BitmapSpec) (rasterize.Bitmap, error) { return bm, nil },
	}
	dst := image.NewRGBA(image.Rect(0, 0, 100, 100))
	l := plan.Layer{
		ItemID: "s", Kind: plan.Bitmap, Bitmap: &spec,
		CX: 50, CY: 50, W: bm.NaturalW, H: bm.NaturalH, Opacity: 1,
	}
	if err := c.Compose(dst, plan.FramePlan{Background: "#0000FF", Layers: []plan.Layer{l}}, 1); err != nil {
		t.Fatal(err)
	}
	if got := at(dst, 50, 50); got != [4]byte{255, 0, 0, 255} {
		t.Errorf("shape centre = %v", got)
	}
	if got := at(dst, 5, 5); got != [4]byte{0, 0, 255, 255} {
		t.Errorf("background = %v", got)
	}
}

// busyScene draws every kind of layer so that a band boundary would show.
func busyScene(bands int) (*Compositor, *image.RGBA, plan.FramePlan) {
	imgs := map[string]Image{"a": pattern(97, 131, 2), "b": pattern(61, 43, 3), "c": pattern(300, 200, 4)}
	c := newComp(bands, imgs)
	m := [12]float64{0.9, 0.2, 0.1, 0.05, 0.1, 0.8, 0.1, 0.0, 0.0, 0.3, 0.7, 0.1}
	lc := media("c", 160, 120, 200, 200, 0, 1)
	lc.ColorMatrix = &m
	fp := plan.FramePlan{Background: "#203040", Layers: []plan.Layer{
		media("a", 100, 150, 97, 131, 0, 1),
		media("b", 200, 80, 150.5, 99.25, 23.5, 0.7),
		lc,
		media("a", 40, 40, 333, 111, -71.25, 0.4),
	}}
	return c, image.NewRGBA(image.Rect(0, 0, 320, 240)), fp
}

func TestBandsEqual(t *testing.T) {
	t.Parallel()
	var want []byte
	for _, bands := range []int{1, 2, 3, 8} {
		c, dst, fp := busyScene(bands)
		if err := c.Compose(dst, fp, 1); err != nil {
			t.Fatal(err)
		}
		if want == nil {
			want = dst.Pix
			continue
		}
		if !bytes.Equal(want, dst.Pix) {
			t.Fatalf("%d bands differ from 1 band", bands)
		}
	}
}

// The hash pins the sampler: a change to any rounding shows here, on every architecture (NFR-010).
func TestBusySceneEqualsItsGoldenHash(t *testing.T) {
	t.Parallel()
	c, dst, fp := busyScene(0)
	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(dst.Pix)
	const want = "137f7894c5b604b940498e1d0f97060784e039e2019e89b9687722a62fd8f5bf"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("hash = %s", got)
	}
}

func TestComposeRejectsAPaddedFrame(t *testing.T) {
	t.Parallel()
	c := newComp(1, nil)
	dst := &image.RGBA{Pix: make([]byte, 100), Stride: 20, Rect: image.Rect(0, 0, 4, 4)}
	if err := c.Compose(dst, plan.FramePlan{}, 1); err == nil {
		t.Error("a padded frame was accepted")
	}
}

func TestResize(t *testing.T) {
	t.Parallel()
	flat := Image{Pix: bytes.Repeat([]byte{40, 80, 120, 255}, 64*48), W: 64, H: 48, Opaque: true}
	for _, size := range [][2]int{{64, 48}, {32, 24}, {10, 7}, {200, 150}} {
		got := Resize(flat, size[0], size[1])
		if got.W != size[0] || got.H != size[1] || len(got.Pix) != size[0]*size[1]*4 {
			t.Fatalf("size %v gave %dx%d", size, got.W, got.H)
		}
		for i := 0; i < len(got.Pix); i += 4 {
			if got.Pix[i] != 40 || got.Pix[i+1] != 80 || got.Pix[i+2] != 120 || got.Pix[i+3] != 255 {
				t.Fatalf("size %v: pixel %d = %v", size, i/4, got.Pix[i:i+4])
			}
		}
	}

	// Averaging 2x2: black and white in a checker give grey 128 (127.5 rounded up).
	chk := Image{
		Pix:    []byte{0, 0, 0, 255, 255, 255, 255, 255, 255, 255, 255, 255, 0, 0, 0, 255},
		W:      2,
		H:      2,
		Opaque: true,
	}
	if got := Resize(chk, 1, 1); got.Pix[0] != 128 {
		t.Errorf("average = %d", got.Pix[0])
	}
}

// gradient is a smooth opaque image, where two correct bilinear samplers agree to within rounding.
func gradient(w, h int) Image {
	pix := make([]byte, w*h*4)
	for y := range h {
		for x := range w {
			o := (y*w + x) * 4
			pix[o], pix[o+1], pix[o+2], pix[o+3] = byte(x*255/(w-1)), byte(y*255/(h-1)), byte((x+y)*255/(w+h-2)), 255
		}
	}
	return Image{Pix: pix, W: w, H: h, Opaque: true}
}

// SDD-004 section 12 rule 2: the fixed-point sampler agrees with draw.BiLinear of x/image, which is the
// reference for what a bilinear transform gives, to within rounding.
func TestSamplerAgreesWithXImageBiLinear(t *testing.T) {
	t.Parallel()
	const (
		sw, sh   = 120, 200
		fw, fh   = 160, 240
		cx, cy   = 80.0, 120.0
		w, h     = 100.0, 150.0
		rotation = 17.0
	)
	src := gradient(sw, sh)
	c := newComp(1, map[string]Image{"a": src})
	ours := image.NewRGBA(image.Rect(0, 0, fw, fh))
	if err := c.Compose(
		ours,
		plan.FramePlan{Background: "#000000", Layers: []plan.Layer{media("a", cx, cy, w, h, rotation, 1)}},
		1,
	); err != nil {
		t.Fatal(err)
	}

	sin, cos := math.Sincos(rotation * math.Pi / 180)
	kx, ky := w/sw, h/sh
	m := f64.Aff3{
		cos * kx, -sin * ky, cx - cos*w/2 + sin*h/2,
		sin * kx, cos * ky, cy - sin*w/2 - cos*h/2,
	}
	ref := image.NewRGBA(image.Rect(0, 0, fw, fh))
	for i := 3; i < len(ref.Pix); i += 4 {
		ref.Pix[i] = 255
	}
	srcImg := &image.RGBA{Pix: src.Pix, Stride: sw * 4, Rect: image.Rect(0, 0, sw, sh)}
	xdraw.BiLinear.Transform(ref, m, srcImg, srcImg.Bounds(), xdraw.Over, nil)

	var off, big int
	var sum [3]int
	for i := 0; i < len(ours.Pix); i += 4 {
		d := 0
		for k := range 3 {
			e := int(math.Abs(float64(int(ours.Pix[i+k]) - int(ref.Pix[i+k]))))
			sum[k] += e
			d = max(d, e)
		}
		if d > 2 {
			off++
		}
		if d > 40 {
			big++
		}
	}
	// Only the pixels on the border of the rotated rectangle may differ (each library decides its own
	// edge pixels); the interior agrees to within one rounding step.
	if share := float64(off) / float64(fw*fh); share > 0.01 {
		t.Errorf("%.2f%% of the pixels differ by more than 2 from x/image's BiLinear", share*100)
	}
	t.Logf("%d pixels differ by more than 2, %d by more than 40 (border of the rectangle)", off, big)

	// The mean absolute error per channel over the whole frame, recorded in SDD-004 section 12.
	for k, name := range []string{"R", "G", "B"} {
		mae := float64(sum[k]) / float64(fw*fh)
		t.Logf("mean absolute error, channel %s: %.4f levels", name, mae)
		if mae > 0.5 {
			t.Errorf("mean absolute error of %s is %.4f levels, want at most 0.5", name, mae)
		}
	}
}

func benchSample(b *testing.B, bands int, rot float64) {
	b.Helper()
	src := pattern(1080, 1920, 9)
	c := newComp(bands, map[string]Image{"a": src})
	dst := image.NewRGBA(image.Rect(0, 0, 1080, 1920))
	fp := plan.FramePlan{Background: "#000000", Layers: []plan.Layer{media("a", 540, 960, 1080, 1920, rot, 1)}}
	b.SetBytes(1080 * 1920 * 4)
	b.ResetTimer()
	for range b.N {
		if err := c.Compose(dst, fp, 1); err != nil {
			b.Fatal(err)
		}
	}
}

// A full-frame layer with a rotation, the slow path of SDD-004 section 12 (target: a frame in under 33 ms with 8 bands).
func BenchmarkSampleRotatedFullFrame1Band(b *testing.B)  { benchSample(b, 1, 3) }
func BenchmarkSampleRotatedFullFrame8Bands(b *testing.B) { benchSample(b, 8, 3) }

// The copy path: a full-frame layer at the same size.
func BenchmarkCopyFullFrame(b *testing.B) { benchSample(b, 1, 0) }

// A window of a layer that reaches outside the frame draws the same pixels as the whole layer cut by the
// frame (VV-162), and a window of a layer that is turned is refused.
func TestWindowedSourceEqualsTheWholeLayer(t *testing.T) {
	t.Parallel()

	full := benchTestImage(300, 200)
	const fw, fh = 100, 200

	// The layer is 300 by 200, centred in a 100 by 200 frame: columns 100 to 199 are visible.
	whole := image.NewRGBA(image.Rect(0, 0, fw, fh))
	layer := plan.Layer{ItemID: "a", Kind: plan.Media, CX: 50, CY: 100, W: 300, H: 200, Opacity: 1}
	fp := plan.FramePlan{Background: "#000000", Layers: []plan.Layer{layer}}

	c := &Compositor{Media: func(*plan.Layer) (Image, error) { return full, nil }}
	if err := c.Compose(whole, fp, 1); err != nil {
		t.Fatal(err)
	}

	win := Image{W: fw, H: fh, Opaque: true, FullW: 300, FullH: 200, OffX: 100, OffY: 0, Pix: make([]byte, fw*fh*4)}
	for y := range fh {
		copy(win.Pix[y*fw*4:(y+1)*fw*4], full.Pix[(y*300+100)*4:(y*300+200)*4])
	}

	got := image.NewRGBA(image.Rect(0, 0, fw, fh))
	c = &Compositor{Media: func(*plan.Layer) (Image, error) { return win, nil }}

	if err := c.Compose(got, fp, 1); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got.Pix, whole.Pix) {
		t.Error("the windowed layer differs from the whole layer")
	}

	fp.Layers[0].FlipX = true
	if err := c.Compose(got, fp, 1); err == nil {
		t.Error("a flipped layer drew a window")
	}
}

func benchTestImage(w, h int) Image {
	pix := make([]byte, w*h*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = byte(i/4%251), byte(i/4/7%241), byte(i/4/13%239), 255
	}

	return Image{Pix: pix, W: w, H: h, Opaque: true}
}
