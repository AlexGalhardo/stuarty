package compositor

import (
	"bytes"
	"image"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func fx(kind string, params map[string]float64) timeline.Effect {
	e := timeline.Effect{Kind: kind}

	for name, v := range params {
		n := int(v)

		switch name {
		case "amount":
			e.Amount = &v
		case "angle":
			e.Angle = &v
		case "radius":
			e.Radius = &v
		case "threshold":
			e.Threshold = &v
		case "softness":
			e.Softness = &v
		case "size":
			e.Size = &n
		case "speed":
			e.Speed = &n
		case "seed":
			e.Seed = &n
		}
	}

	return e
}

// dot is a transparent w by h layer with one opaque white pixel at its centre.
func dot(w, h int) Image {
	pix := make([]byte, w*h*4)
	o := ((h/2)*w + w/2) * 4
	copy(pix[o:], []byte{255, 255, 255, 255})

	return Image{Pix: pix, W: w, H: h}
}

// composeOne draws one layer over a black frame of fw by fh at scale 1.
func composeOne(t *testing.T, img Image, l plan.Layer, fw, fh, frame, bands int) *image.RGBA {
	t.Helper()

	c := newComp(bands, map[string]Image{l.ItemID: img})
	dst := image.NewRGBA(image.Rect(0, 0, fw, fh))
	fp := plan.FramePlan{Frame: frame, Width: fw, Height: fh, Background: "#000000", Layers: []plan.Layer{l}}

	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}

	return dst
}

func lit(img *image.RGBA) (xs, ys []int) {
	for y := range img.Rect.Dy() {
		for x := range img.Rect.Dx() {
			if p := at(img, x, y); p != [4]byte{0, 0, 0, 255} {
				xs, ys = append(xs, x), append(ys, y)
			}
		}
	}

	return xs, ys
}

func TestAngleTable(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		deg  float64
		c, s int64
	}{
		{0, 1024, 0},
		{90, 0, 1024},
		{-90, 0, -1024},
		{180, -1024, 0},
		{-180, -1024, 0},
		{45, 724, 724},
		{-45, 724, -724},
		{135, -724, 724},
		{-135, -724, -724},
		{30, 887, 512},
		{30.4, 887, 512},
		{30.5, 878, 527},
		{120, -512, 887},
	} {
		c, s := angleCS(tc.deg)
		if c != tc.c || s != tc.s {
			t.Errorf("angleCS(%v) = %d, %d; want %d, %d", tc.deg, c, s, tc.c, tc.s)
		}
	}
}

func TestMotionBlurSpreadsADotAndGrowsTheLayer(t *testing.T) {
	t.Parallel()

	// amount 0.1 is 6 pixels: h = 3, seven taps of (255 + 3) / 7 = 36.
	l := media("a", 15, 15, 9, 9, 0, 1)
	l.Effects = []timeline.Effect{fx(timeline.EffectMotionBlur, map[string]float64{"amount": 0.1, "angle": 0})}
	dst := composeOne(t, dot(9, 9), l, 30, 30, 0, 1)

	xs, ys := lit(dst)
	if len(xs) != 7 {
		t.Fatalf("%d pixels lit, want 7", len(xs))
	}

	for i := range xs {
		if ys[i] != ys[0] || xs[i] != xs[0]+i {
			t.Fatalf("the pixels are not one row of 7: %v %v", xs, ys)
		}

		if got := at(dst, xs[i], ys[i]); got != [4]byte{36, 36, 36, 255} {
			t.Errorf("tap %d = %v, want 36", i, got)
		}
	}
}

func TestMotionBlurAlongAVerticalAngle(t *testing.T) {
	t.Parallel()

	l := media("a", 15, 15, 9, 9, 0, 1)
	l.Effects = []timeline.Effect{fx(timeline.EffectMotionBlur, map[string]float64{"amount": 0.1, "angle": 90})}
	xs, ys := lit(composeOne(t, dot(9, 9), l, 30, 30, 0, 1))

	if len(xs) != 7 || xs[0] != xs[6] || ys[6]-ys[0] != 6 {
		t.Fatalf("not a column of 7: %v %v", xs, ys)
	}
}

func TestRGBSplitMovesTheChannels(t *testing.T) {
	t.Parallel()

	// amount 0.1 is 2.4, which is 2 pixels.
	l := media("a", 15, 15, 9, 9, 0, 1)
	l.Effects = []timeline.Effect{fx(timeline.EffectRGBSplit, map[string]float64{"amount": 0.1, "angle": 0})}
	dst := composeOne(t, dot(9, 9), l, 30, 30, 0, 1)

	xs, ys := lit(dst)
	if len(xs) != 3 {
		t.Fatalf("%d pixels lit, want 3", len(xs))
	}

	y := ys[0]
	if got := at(dst, xs[0], y); got != [4]byte{0, 0, 255, 255} {
		t.Errorf("left = %v, want blue", got)
	}

	if got := at(dst, xs[1], y); got != [4]byte{0, 255, 0, 255} {
		t.Errorf("middle = %v, want green", got)
	}

	if got := at(dst, xs[2], y); got != [4]byte{255, 0, 0, 255} {
		t.Errorf("right = %v, want red", got)
	}

	if xs[1]-xs[0] != 2 || xs[2]-xs[1] != 2 {
		t.Errorf("the channels are %v apart", xs)
	}
}

func TestVignetteDarkensTheCornersNotTheMiddle(t *testing.T) {
	t.Parallel()

	white := solid(20, 20, [4]byte{255, 255, 255, 255})
	l := media("a", 20, 20, 20, 20, 0, 1)
	l.Effects = []timeline.Effect{fx(timeline.EffectVignette, map[string]float64{"amount": 1, "softness": 0.5})}
	dst := composeOne(t, white, l, 40, 40, 0, 1)

	if got := at(dst, 20, 20); got != [4]byte{255, 255, 255, 255} {
		t.Errorf("middle = %v, want untouched", got)
	}

	corner := at(dst, 10, 10)
	if corner[0] >= 80 || corner[0] != corner[1] || corner[3] != 255 {
		t.Errorf("corner = %v, want a dark grey", corner)
	}

	for _, p := range [][2]int{{29, 29}, {10, 29}, {29, 10}} {
		if got := at(dst, p[0], p[1]); got != corner {
			t.Errorf("corner %v = %v, want the same as the first, %v", p, got, corner)
		}
	}

	plain := composeOne(t, white, media("a", 20, 20, 20, 20, 0, 1), 40, 40, 0, 1)

	l.Effects = []timeline.Effect{fx(timeline.EffectVignette, map[string]float64{"amount": 0, "softness": 0.5})}
	if got := composeOne(t, white, l, 40, 40, 0, 1); !bytes.Equal(got.Pix, plain.Pix) {
		t.Error("a vignette of amount 0 changed the frame")
	}
}

func TestGrainIsHashedPerFrameAndEqualOnTheChannels(t *testing.T) {
	t.Parallel()

	grey := solid(24, 24, [4]byte{128, 128, 128, 255})
	l := media("a", 24, 24, 24, 24, 0, 1)
	l.Effects = []timeline.Effect{fx(timeline.EffectGrain, map[string]float64{"amount": 1, "size": 1, "seed": 7})}

	a := composeOne(t, grey, l, 48, 48, 3, 1)
	b := composeOne(t, grey, l, 48, 48, 3, 1)
	other := composeOne(t, grey, l, 48, 48, 4, 1)

	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("the same frame gave two grains")
	}

	if bytes.Equal(a.Pix, other.Pix) {
		t.Fatal("two frames have the same grain")
	}

	moved := false

	for i := 0; i < len(a.Pix); i += 4 {
		if a.Pix[i] != a.Pix[i+1] || a.Pix[i] != a.Pix[i+2] {
			t.Fatalf("pixel %d: the channels differ: %v", i/4, a.Pix[i:i+3])
		}

		if a.Pix[i] != 128 && a.Pix[i] != 0 {
			moved = true
		}
	}

	if !moved {
		t.Fatal("the grain changed nothing")
	}

	l.Effects = []timeline.Effect{fx(timeline.EffectGrain, map[string]float64{"amount": 0})}
	zero := composeOne(t, grey, l, 48, 48, 3, 1)

	for y := 12; y < 36; y++ {
		for x := 12; x < 36; x++ {
			if got := at(zero, x, y); got != [4]byte{128, 128, 128, 255} {
				t.Fatalf("a grain of amount 0 changed (%d,%d) to %v", x, y, got)
			}
		}
	}
}

func TestGrainNeverPassesTheAlpha(t *testing.T) {
	t.Parallel()

	// A pixel of alpha 0 stays transparent and one of alpha 100 keeps its channels between 0 and 100.
	e := &effectCtx{
		c: &Compositor{Bands: 1}, outScale: 1, scale: 1, frame: 9,
		geom: effectGeom{sw: 16, sh: 16, wq: 4096, hq: 4096},
	}
	ef := fx(timeline.EffectGrain, map[string]float64{"amount": 1, "size": 1, "seed": 3})

	img := solid(16, 16, [4]byte{100, 100, 100, 100})
	out := e.grain(img, &ef)

	for i := 0; i < len(out.Pix); i += 4 {
		if out.Pix[i+3] != 100 || out.Pix[i] > 100 {
			t.Fatalf("pixel %d = %v", i/4, out.Pix[i:i+4])
		}
	}

	transparent := solid(16, 16, [4]byte{0, 0, 0, 0})
	if out := e.grain(transparent, &ef); !bytes.Equal(out.Pix, transparent.Pix) {
		t.Fatal("the grain drew on transparent pixels")
	}
}

func TestGlowHaloAroundBrightPixelsOnly(t *testing.T) {
	t.Parallel()

	// A bright block on black: the halo reaches the dark pixels around it and the block stays at full value.
	img := solid(30, 30, [4]byte{0, 0, 0, 255})
	for y := 12; y < 18; y++ {
		for x := 12; x < 18; x++ {
			copy(img.Pix[(y*30+x)*4:], []byte{255, 255, 255, 255})
		}
	}

	l := media("a", 30, 30, 30, 30, 0, 1)
	l.Effects = []timeline.Effect{
		fx(timeline.EffectGlow, map[string]float64{"amount": 1, "radius": 3, "threshold": 0.5}),
	}
	dst := composeOne(t, img, l, 60, 60, 0, 1)

	if got := at(dst, 30, 30); got != [4]byte{255, 255, 255, 255} {
		t.Errorf("block = %v", got)
	}

	if got := at(dst, 30+3+2, 30); got[0] == 0 {
		t.Errorf("next to the block = %v, want a halo", got)
	}

	if got := at(dst, 16, 16); got[0] != 0 {
		t.Errorf("far from the block = %v, want black", got)
	}

	// Nothing above the threshold, nothing added.
	dark := solid(30, 30, [4]byte{40, 40, 40, 255})
	dst = composeOne(t, dark, l, 60, 60, 0, 1)

	if got := at(dst, 30, 30); got != [4]byte{40, 40, 40, 255} {
		t.Errorf("a dark layer = %v, want untouched", got)
	}
}

func TestDipMixEndsAtTheLayerAndAtTheColour(t *testing.T) {
	t.Parallel()

	img := Image{Pix: []byte{200, 100, 50, 255, 100, 50, 25, 128, 0, 0, 0, 0}, W: 3, H: 1}
	col := [4]uint8{10, 20, 30, 255}

	if got := dipMix(img, 65535, col); !bytes.Equal(got.Pix, img.Pix) {
		t.Errorf("progress near 1 = %v, want the layer %v", got.Pix, img.Pix)
	}

	got := dipMix(img, 1, col)
	if !bytes.Equal(got.Pix[:4], []byte{10, 20, 30, 255}) {
		t.Errorf("opaque pixel at progress near 0 = %v, want the colour", got.Pix[:4])
	}

	// Inside the shape of a half transparent pixel the colour is premultiplied by its alpha.
	if want := []byte{5, 10, 15, 128}; !bytes.Equal(got.Pix[4:8], want) {
		t.Errorf("half transparent pixel = %v, want %v", got.Pix[4:8], want)
	}

	if !bytes.Equal(got.Pix[8:], []byte{0, 0, 0, 0}) {
		t.Errorf("transparent pixel = %v, want transparent", got.Pix[8:])
	}
}

func TestWipeCoversFromTheEdge(t *testing.T) {
	t.Parallel()

	white := solid(20, 10, [4]byte{255, 255, 255, 255})
	cases := []struct {
		dir     string
		covered func(x, y int) bool
	}{
		{"left", func(x, _ int) bool { return x < 10 }},
		{"right", func(x, _ int) bool { return x >= 10 }},
		{"up", func(_, y int) bool { return y < 5 }},
		{"down", func(_, y int) bool { return y >= 5 }},
	}

	for _, tc := range cases {
		l := media("a", 10, 5, 20, 10, 0, 1)
		l.Transition = &plan.TransitionSpec{Kind: "wipe", Edge: "in", P: 32768, Direction: tc.dir}
		dst := composeOne(t, white, l, 20, 10, 0, 1)

		for y := range 10 {
			for x := range 20 {
				want := [4]byte{0, 0, 0, 255}
				if tc.covered(x, y) {
					want = [4]byte{255, 255, 255, 255}
				}

				if got := at(dst, x, y); got != want {
					t.Fatalf("%s: pixel (%d,%d) = %v, want %v", tc.dir, x, y, got, want)
				}
			}
		}
	}
}

func TestWipeMultipliesTheMask(t *testing.T) {
	t.Parallel()

	white := solid(20, 10, [4]byte{255, 255, 255, 255})
	l := media("a", 10, 5, 20, 10, 0, 1)
	l.Mask = &plan.MaskSpec{Shape: "rect", X: 0.25, Y: 0, Width: 0.5, Height: 1}
	l.Transition = &plan.TransitionSpec{Kind: "wipe", Edge: "in", P: 32768, Direction: "left"}
	dst := composeOne(t, white, l, 20, 10, 0, 1)

	for x := range 20 {
		want := [4]byte{0, 0, 0, 255}
		if x >= 5 && x < 10 {
			want = [4]byte{255, 255, 255, 255}
		}

		if got := at(dst, x, 4); got != want {
			t.Fatalf("pixel %d = %v, want %v", x, got, want)
		}
	}
}

func TestWhipBlursAlongItsDirection(t *testing.T) {
	t.Parallel()

	l := media("a", 40, 40, 21, 21, 0, 1)
	l.Transition = &plan.TransitionSpec{Kind: "whip", Edge: "in", P: 4096, Direction: "left"}
	// Width 80: len = ((65536-4096) x 80 / 16 + 32768) >> 16 = 5, so h = 2 and five taps.
	xs, ys := lit(composeOne(t, dot(21, 21), l, 80, 80, 0, 1))

	if len(xs) != 5 || ys[0] != ys[4] {
		t.Fatalf("not a row of 5: %v %v", xs, ys)
	}

	l.Transition.P = 65000
	if xs, _ := lit(composeOne(t, dot(21, 21), l, 80, 80, 0, 1)); len(xs) != 1 {
		t.Fatalf("a whip near its end blurred the layer: %d pixels", len(xs))
	}

	l.Transition.Direction = "down"
	l.Transition.P = 4096
	// Height 160: len = 9, so h = 4 and nine taps in a column.
	xs, ys = lit(composeOne(t, dot(21, 21), l, 80, 160, 0, 1))
	if len(ys) != 9 || xs[0] != xs[8] {
		t.Fatalf("not a column of 9: %v %v", xs, ys)
	}
}

// Every effect and transition gives the same bytes in one and eight bands, on a layer that is rotated and
// scaled, so the sampler, the padding and the masks are all in play.
func TestEffectsBandsEqual(t *testing.T) {
	t.Parallel()

	all := []timeline.Effect{
		fx(timeline.EffectMotionBlur, map[string]float64{"amount": 0.4, "angle": 33}),
		fx(timeline.EffectGlow, map[string]float64{"amount": 0.8, "radius": 4, "threshold": 0.3}),
		fx(timeline.EffectVignette, map[string]float64{"amount": 0.9, "softness": 0.7}),
		fx(timeline.EffectGrain, map[string]float64{"amount": 0.6, "size": 2, "seed": 11}),
		fx(timeline.EffectRGBSplit, map[string]float64{"amount": 0.5, "angle": -50}),
	}
	transitions := []*plan.TransitionSpec{
		nil,
		{Kind: "whip", Edge: "out", P: 9000, Direction: "up"},
		{Kind: "dip", Edge: "in", P: 30000, Color: "#ff8000"},
		{Kind: "wipe", Edge: "out", P: 40000, Direction: "right"},
	}

	for _, tr := range transitions {
		l := media("a", 50, 40, 60, 44, 17, 0.9)
		l.Effects, l.Transition = all, tr
		l.Mask = &plan.MaskSpec{Shape: "ellipse", X: 0.1, Y: 0.1, Width: 0.8, Height: 0.8, Feather: 5}
		l.FlipX = true

		one := composeOne(t, pattern(40, 30, 5), l, 100, 80, 6, 1)
		eight := composeOne(t, pattern(40, 30, 5), l, 100, 80, 6, 8)

		if !bytes.Equal(one.Pix, eight.Pix) {
			t.Errorf("transition %+v: the bytes depend on the bands", tr)
		}
	}
}
