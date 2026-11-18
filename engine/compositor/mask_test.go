package compositor

import (
	"image"
	"math"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

func newTestMask(spec plan.MaskSpec, p placement) *maskState { return newMask(&spec, nil, p, 1) }

func TestMaskRectHardEdge(t *testing.T) {
	t.Parallel()

	// 8 by 6 at (2, 2); the mask covers the columns 2..5 of the layer's 8 (x 0.25, width 0.5).
	m := newTestMask(
		plan.MaskSpec{Shape: "rect", X: 0.25, Y: 0, Width: 0.5, Height: 1},
		placement{cx: 6, cy: 5, w: 8, h: 6},
	)
	for x := range 8 {
		want := uint32(0)
		if x >= 2 && x <= 5 {
			want = 255
		}
		for y := range 6 {
			if got := m.copyCov(2+x, 2+y); got != want {
				t.Fatalf("column %d row %d: coverage %d, want %d", x, y, got, want)
			}
		}
	}
}

func TestMaskEllipseAndInvert(t *testing.T) {
	t.Parallel()

	p := placement{cx: 20, cy: 20, w: 40, h: 40}
	spec := plan.MaskSpec{Shape: "ellipse", X: 0, Y: 0, Width: 1, Height: 1}
	m := newTestMask(spec, p)
	if got := m.copyCov(20, 20); got != 255 {
		t.Fatalf("centre coverage %d", got)
	}
	if got := m.copyCov(0, 0); got != 0 {
		t.Fatalf("corner coverage %d", got)
	}
	spec.Invert = true
	inv := newTestMask(spec, p)
	for y := range 40 {
		for x := range 40 {
			if m.copyCov(x, y)+inv.copyCov(x, y) != 255 {
				t.Fatalf("pixel (%d,%d): the inverted coverage is not the complement", x, y)
			}
		}
	}
}

func TestMaskFeatherRamp(t *testing.T) {
	t.Parallel()

	m := newTestMask(
		plan.MaskSpec{Shape: "rect", X: 0, Y: 0, Width: 1, Height: 1, Feather: 8},
		placement{cx: 20, cy: 20, w: 40, h: 40},
	)
	// The soft edge is centred on the edge of the shape: half coverage on it, monotonic across 8 px.
	if got := m.coverage(m.l, 20<<maskFix); got != 128 {
		t.Fatalf("coverage on the edge = %d, want 128", got)
	}
	prev := uint32(0)
	for lx := m.l - 5*256; lx <= m.l+5*256; lx += 16 {
		got := m.coverage(lx, 20<<maskFix)
		if got < prev {
			t.Fatalf("coverage falls from %d to %d at %d", prev, got, lx)
		}
		prev = got
	}
	if m.coverage(m.l-4*256, 20<<maskFix) != 0 || m.coverage(m.l+4*256, 20<<maskFix) != 255 {
		t.Fatal("the ramp does not span the feather")
	}
}

// With the layer in place and not rotated, the sampler finds the same position as the copy.
func TestMaskSamplerPositionEqualsCopy(t *testing.T) {
	t.Parallel()

	p := placement{cx: 30, cy: 25, w: 40, h: 20}
	m := newTestMask(plan.MaskSpec{Shape: "ellipse", X: 0.1, Y: 0.2, Width: 0.7, Height: 0.6, Feather: 3}, p)
	for y := 10; y < 40; y++ {
		for x := 5; x < 60; x++ {
			if a, b := m.copyCov(x, y), m.sampleCov(x, y); a != b {
				t.Fatalf("pixel (%d,%d): copy %d, sampler %d", x, y, a, b)
			}
		}
	}
}

// A masked layer never takes the opaque copy, and a pixel outside the mask draws nothing.
func TestMaskedLayerIsNotCopied(t *testing.T) {
	t.Parallel()

	src := pattern(8, 6, 4)
	for _, bands := range []int{1, 8} {
		c := newComp(bands, map[string]Image{"a": src})
		dst := image.NewRGBA(image.Rect(0, 0, 12, 10))
		l := media("a", 6, 5, 8, 6, 0, 1)
		l.Mask = &plan.MaskSpec{Shape: "rect", X: 0, Y: 0, Width: 0.5, Height: 1}
		if err := c.Compose(dst, plan.FramePlan{Background: "#808080", Layers: []plan.Layer{l}}, 1); err != nil {
			t.Fatal(err)
		}
		for y := range 6 {
			for x := range 8 {
				want := [4]byte{128, 128, 128, 255}
				if x < 4 {
					want = [4]byte(src.Pix[(y*8+x)*4 : (y*8+x)*4+4])
				}
				if got := at(dst, 2+x, 2+y); got != want {
					t.Fatalf("bands %d pixel (%d,%d) = %v, want %v", bands, x, y, got, want)
				}
			}
		}
	}
}

func TestMaskedSampledLayerBandsAgree(t *testing.T) {
	t.Parallel()

	src := pattern(30, 20, 9)
	render := func(bands int) []byte {
		c := newComp(bands, map[string]Image{"a": src})
		dst := image.NewRGBA(image.Rect(0, 0, 120, 100))
		l := media("a", 60, 50, 64, 48, 20, 0.7)
		l.Blend = "screen"
		l.Blur = 2
		l.Mask = &plan.MaskSpec{Shape: "ellipse", X: 0.1, Y: 0.1, Width: 0.8, Height: 0.8, Feather: 6, Invert: true}
		if err := c.Compose(dst, plan.FramePlan{Background: "#203040", Layers: []plan.Layer{l}}, 1); err != nil {
			t.Fatal(err)
		}
		return dst.Pix
	}
	one, eight := render(1), render(8)
	for i := range one {
		if one[i] != eight[i] {
			t.Fatalf("byte %d differs between 1 and 8 bands", i)
		}
	}
}

func TestMaskClampsAbsurdNumbers(t *testing.T) {
	t.Parallel()

	m := newTestMask(
		plan.MaskSpec{Shape: "ellipse", X: math.NaN(), Y: -3, Width: 1e300, Height: math.Inf(1), Feather: 1 << 30},
		placement{cx: 1e300, cy: -1e300, w: 1e300, h: 1e-300},
	)
	for _, p := range [][2]int{{0, 0}, {math.MaxInt32, math.MinInt32}, {-5, 1 << 30}} {
		if m.copyCov(p[0], p[1]) > 255 || m.sampleCov(p[0], p[1]) > 255 {
			t.Fatalf("coverage out of range at %v", p)
		}
	}
}
