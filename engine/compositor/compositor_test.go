package compositor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"math/rand/v2"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/engine/raster"
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
		Kind: raster.KindShape, ActiveWord: -1, PixelScale: 1,
		Shape: &plan.ShapeSpec{Shape: raster.ShapeRect, Width: 40, Height: 20, Fill: "#FF0000", StrokeColor: "#000000"},
	}
	bm, err := raster.Render(spec, raster.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	c := &Compositor{
		Bands:   1,
		Bitmaps: func(plan.BitmapSpec) (raster.Bitmap, error) { return bm, nil },
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
