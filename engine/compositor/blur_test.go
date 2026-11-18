package compositor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// blurSceneHash pins the blur on every architecture (NFR-010).
const blurSceneHash = "32eeae97b495eb5ff12fd37205061e12008aaa3b2a64ecfe94fc26d7ba937c3a"

func TestBlurRadius(t *testing.T) {
	t.Parallel()

	for sigma, want := range map[float64]int{
		-1: 0, 0: 0, 0.5: 0, 0.8: 0, 0.9: 1, 1: 1, 2: 2, 4: 4, 10: 10, 100: 100, 1000: maxBlurRadius,
	} {
		if got := blurRadius(sigma); got != want {
			t.Errorf("blurRadius(%v) = %d, want %d", sigma, got, want)
		}
	}
}

func solid(w, h int, c [4]byte) Image {
	pix := make([]byte, w*h*4)
	for i := 0; i < len(pix); i += 4 {
		copy(pix[i:], c[:])
	}

	return Image{Pix: pix, W: w, H: h, Opaque: c[3] == 255}
}

func blurLayer(id string, cx, cy, w, h, blur float64) plan.Layer {
	l := media(id, cx, cy, w, h, 0, 1)
	l.Blur = blur

	return l
}

// A solid layer keeps its colour inside, fades to the background past its edge by a few radii, and
// the background is untouched beyond that: the blur reads the outside as transparent and the layer
// grows softly (SDD-004 section 3.6).
func TestBlurSoftensTheEdgeOfASolidLayer(t *testing.T) {
	t.Parallel()

	c := newComp(1, map[string]Image{"a": solid(40, 40, [4]byte{200, 100, 50, 255})})
	dst := image.NewRGBA(image.Rect(0, 0, 100, 100))
	fp := plan.FramePlan{Background: "#000000", Layers: []plan.Layer{blurLayer("a", 50, 50, 40, 40, 3)}}

	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}

	if got := at(dst, 50, 50); got != [4]byte{200, 100, 50, 255} {
		t.Errorf("centre = %v, want the solid colour", got)
	}

	// The edge is at x = 30; the blur has radius 3 (variance 12 per axis), three passes reach 9.
	row := func(x int) byte { return at(dst, x, 50)[0] }

	if row(30) <= row(27) || row(30) >= row(33) || row(27) == 0 || row(33) == 200 {
		t.Errorf("the edge is not soft: %d %d %d %d", row(27), row(30), row(33), row(36))
	}

	if row(30) < 90 || row(30) > 125 {
		t.Errorf("the edge pixel is %d, want about half of 200", row(30))
	}

	if row(15) != 0 || row(85) != 0 {
		t.Errorf("the background past the reach changed: %d %d", row(15), row(85))
	}

	if dst.Pix[3] != 255 {
		t.Errorf("the frame is not opaque: %d", dst.Pix[3])
	}
}

// The blurred layer is mirror symmetric around a centred square and keeps its light: the sum of the
// bytes changes by rounding only.
func TestBlurIsSymmetricAndKeepsTheLight(t *testing.T) {
	t.Parallel()

	src := solid(21, 21, [4]byte{})

	for y := 8; y < 13; y++ {
		for x := 8; x < 13; x++ {
			copy(src.Pix[(y*21+x)*4:], []byte{255, 255, 255, 255})
		}
	}

	c := newComp(1, nil)
	out, p := c.blurred(src, placement{cx: 10, cy: 10, w: 21, h: 21}, 2)

	if out.W != 21+2*6 || out.H != out.W || p.w != float64(out.W) || p.cx != 10 {
		t.Fatalf("grown to %dx%d, placement %+v", out.W, out.H, p)
	}

	px := func(x, y int) byte { return out.Pix[(y*out.W+x)*4] }

	for y := range out.H {
		for x := range out.W {
			if px(x, y) != px(out.W-1-x, y) || px(x, y) != px(x, out.H-1-y) {
				t.Fatalf("not symmetric at %d,%d", x, y)
			}
		}
	}

	var before, after int

	for i := 0; i < len(src.Pix); i += 4 {
		before += int(src.Pix[i])
	}

	for i := 0; i < len(out.Pix); i += 4 {
		after += int(out.Pix[i])
	}

	if d := float64(after-before) / float64(before); d < -0.02 || d > 0.02 {
		t.Errorf("light %d before, %d after (%.3f)", before, after, d)
	}
}

// The reach follows the scale at which the source is drawn: the same sigma on the frame is twice
// the radius in the pixels of a source that is drawn at half its size.
func TestBlurReachFollowsTheDrawnScale(t *testing.T) {
	t.Parallel()

	c := newComp(1, nil)
	big := solid(40, 40, [4]byte{255, 255, 255, 255})

	same, _ := c.blurred(big, placement{w: 40, h: 40}, 4)
	half, _ := c.blurred(big, placement{w: 20, h: 20}, 4)

	if same.W != 40+2*3*4 || half.W != 40+2*3*8 {
		t.Errorf("grown to %d and %d", same.W, half.W)
	}
}

func TestBlurBandsEqual(t *testing.T) {
	t.Parallel()

	var want []byte

	for _, bands := range []int{1, 2, 3, 8} {
		c := newComp(bands, map[string]Image{"a": pattern(260, 180, 5)})
		dst := image.NewRGBA(image.Rect(0, 0, 320, 240))
		fp := plan.FramePlan{Background: "#102030", Layers: []plan.Layer{blurLayer("a", 160, 120, 260, 180, 6)}}

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

func TestBlurSceneEqualsItsGoldenHash(t *testing.T) {
	t.Parallel()

	c := newComp(0, map[string]Image{"a": pattern(260, 180, 5), "b": pattern(120, 120, 6)})
	dst := image.NewRGBA(image.Rect(0, 0, 320, 240))
	top := blurLayer("b", 200, 90, 120, 120, 2.5)
	top.Rotation, top.Opacity = 17, 0.8
	fp := plan.FramePlan{
		Background: "#102030",
		Layers:     []plan.Layer{blurLayer("a", 160, 120, 260, 180, 6), top},
	}

	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(dst.Pix)
	if got := hex.EncodeToString(sum[:]); got != blurSceneHash {
		t.Errorf("hash = %s", got)
	}
}

// The output scale multiplies the sigma like any size.
func TestBlurScalesWithTheOutputScale(t *testing.T) {
	t.Parallel()

	src := solid(40, 40, [4]byte{255, 255, 255, 255})
	c := newComp(1, map[string]Image{"a": src})
	l := blurLayer("a", 40, 40, 40, 40, 2)
	fp := plan.FramePlan{Background: "#000000", Layers: []plan.Layer{l}}

	one := image.NewRGBA(image.Rect(0, 0, 80, 80))
	if err := c.Compose(one, fp, 1); err != nil {
		t.Fatal(err)
	}

	two := image.NewRGBA(image.Rect(0, 0, 160, 160))
	if err := c.Compose(two, fp, 2); err != nil {
		t.Fatal(err)
	}

	// Sigma 2 at scale 1 reaches 6 px past the edge of the square; sigma 4 at scale 2 reaches 12.
	reach := func(img *image.RGBA, edge int) int {
		n := 0

		for x := edge - 16; x < edge; x++ {
			if at(img, x, img.Rect.Dy()/2)[0] > 0 {
				n++
			}
		}

		return n
	}

	if a, b := reach(one, 20), reach(two, 40); a == 0 || b < 2*a-3 || b > 2*a+3 {
		t.Errorf("reach %d at scale 1 and %d at scale 2", a, b)
	}
}

func TestFilterOnALayer(t *testing.T) {
	t.Parallel()

	src := Image{Pix: []byte{200, 100, 50, 255}, W: 1, H: 1, Opaque: true}
	c := newComp(1, map[string]Image{"a": src})

	run := func(name string, amount float64) [4]byte {
		dst := image.NewRGBA(image.Rect(0, 0, 3, 3))
		l := media("a", 1.5, 1.5, 1, 1, 0, 1)
		l.Filter = plan.FilterMatrix(name, amount)

		if err := c.Compose(dst, plan.FramePlan{Background: "#000000", Layers: []plan.Layer{l}}, 1); err != nil {
			t.Fatal(err)
		}

		return at(dst, 1, 1)
	}

	for name, want := range map[string][4]byte{
		"invert":    {55, 155, 205, 255},
		"grayscale": {118, 118, 118, 255},
		"sepia":     {165, 147, 114, 255},
		"warm":      {220, 100, 45, 255},
		"cool":      {180, 100, 55, 255},
		"none":      {200, 100, 50, 255},
	} {
		if got := run(name, 1); got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}

	if got := run("invert", 0.5); got != [4]byte{128, 128, 128, 255} {
		t.Errorf("invert at 0.5 = %v", got)
	}
}

// The filter comes after the colour matrix and leaves the alpha alone.
func TestFilterAfterColourKeepsAlpha(t *testing.T) {
	t.Parallel()

	// Premultiplied (100, 50, 25) at alpha 128 is straight (199, 99, 50).
	src := Image{Pix: []byte{100, 50, 25, 128}, W: 1, H: 1}
	c := newComp(1, map[string]Image{"a": src})
	dst := image.NewRGBA(image.Rect(0, 0, 3, 3))
	l := media("a", 1.5, 1.5, 1, 1, 0, 1)
	l.ColorMatrix = &[12]float64{1, 0, 0, 0.1, 0, 1, 0, 0.1, 0, 0, 1, 0.1}
	l.Filter = plan.FilterMatrix("invert", 1)

	if err := c.Compose(dst, plan.FramePlan{Background: "#000000", Layers: []plan.Layer{l}}, 1); err != nil {
		t.Fatal(err)
	}

	if got := at(dst, 1, 1); got != [4]byte{15, 65, 90, 255} {
		t.Errorf("composed = %v", got)
	}
}

func benchBlur(b *testing.B, bands int, sigma float64) {
	b.Helper()

	src := pattern(1080, 1920, 9)
	c := newComp(bands, map[string]Image{"a": src})
	dst := image.NewRGBA(image.Rect(0, 0, 1080, 1920))
	fp := plan.FramePlan{Background: "#000000", Layers: []plan.Layer{blurLayer("a", 540, 960, 1080, 1920, sigma)}}

	b.SetBytes(1080 * 1920 * 4)
	b.ResetTimer()

	for range b.N {
		if err := c.Compose(dst, fp, 1); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportMetric(float64(b.N)/30/b.Elapsed().Seconds(), "x_realtime")
}

func BenchmarkComposeBlur8Bands(b *testing.B)      { benchBlur(b, 8, 12) }
func BenchmarkComposeBlur1Band(b *testing.B)       { benchBlur(b, 1, 12) }
func BenchmarkComposeBlurLarge8Bands(b *testing.B) { benchBlur(b, 8, 60) }
