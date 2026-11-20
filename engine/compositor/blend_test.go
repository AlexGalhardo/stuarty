package compositor

import (
	"image"
	"math"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// blendRef is the W3C compositing of SDD-004 section 3.8 in floating point on an opaque destination:
// (1 - a) x Cd + a x B(Cs, Cd) with the straight source Cs = c / a, scaled to 0..255.
func blendRef(mode blendMode, c, d, a uint32) float64 {
	if a == 0 {
		return float64(d)
	}
	cs, cd, al := float64(c)/float64(a), float64(d)/255, float64(a)/255
	var b float64
	switch mode {
	case blendMultiply:
		b = cs * cd
	case blendScreen:
		b = cs + cd - cs*cd
	case blendOverlay:
		if cd <= 0.5 {
			b = 2 * cs * cd
		} else {
			b = cs + (2*cd - 1) - cs*(2*cd-1)
		}
	case blendAdd:
		b = min(cs+cd, 1)
	case blendNormal:
	}
	if mode == blendAdd {
		// The contract adds the premultiplied source: the clamp of B before the alpha is not applied.
		return math.Min(float64(c)+float64(d), 255)
	}
	return 255 * ((1-al)*cd + al*b)
}

func TestBlendChannelFollowsTheCompositingFormulas(t *testing.T) {
	t.Parallel()

	for _, mode := range []blendMode{blendMultiply, blendScreen, blendOverlay, blendAdd} {
		for a := uint32(0); a <= 255; a += 5 {
			for c := uint32(0); c <= a; c += 5 {
				for d := range uint32(256) {
					got := float64(blendChannel(mode, c, d, a))
					if want := blendRef(mode, c, d, a); math.Abs(got-want) > 1.5 {
						t.Fatalf("mode %d c=%d d=%d a=%d: %v, reference %v", mode, c, d, a, got, want)
					}
				}
			}
		}
	}
}

func TestBlendChannelOpaqueSourceValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		mode    blendMode
		c, d    uint32
		want    uint8
		message string
	}{
		{blendMultiply, 255, 200, 200, "multiply by white keeps the destination"},
		{blendMultiply, 0, 200, 0, "multiply by black is black"},
		{blendMultiply, 128, 100, 50, "multiply by half"},
		{blendScreen, 0, 200, 200, "screen with black keeps the destination"},
		{blendScreen, 255, 10, 255, "screen with white is white"},
		{blendOverlay, 128, 100, 100, "overlay of mid grey on a dark value"},
		{blendOverlay, 255, 127, 254, "overlay white on the last dark value"},
		{blendOverlay, 0, 128, 1, "overlay black on the first light value"},
		{blendAdd, 200, 100, 255, "add clamps"},
		{blendAdd, 20, 30, 50, "add"},
	}
	for _, c := range cases {
		if got := blendChannel(c.mode, c.c, c.d, 255); got != c.want {
			t.Errorf("%s: %d, want %d", c.message, got, c.want)
		}
	}
}

// A layer with a blend mode never takes the opaque copy: the mix reads the destination (section 3.8).
func TestBlendLayerDoesNotCopy(t *testing.T) {
	t.Parallel()

	src := pattern(8, 6, 3)
	for _, bands := range []int{1, 8} {
		c := newComp(bands, map[string]Image{"a": src})
		dst := image.NewRGBA(image.Rect(0, 0, 12, 10))
		l := media("a", 6, 5, 8, 6, 0, 1)
		l.Blend = "multiply"
		if err := c.Compose(dst, plan.FramePlan{Background: "#808080", Layers: []plan.Layer{l}}, 1); err != nil {
			t.Fatal(err)
		}
		for y := range 6 {
			for x := range 8 {
				s := src.Pix[(y*8+x)*4:]
				half := func(v byte) byte { return u8(div255(128 * uint32(v))) }
				want := [4]byte{half(s[0]), half(s[1]), half(s[2]), 255}
				if got := at(dst, 2+x, 2+y); got != want {
					t.Fatalf("bands %d pixel (%d,%d) = %v, want %v", bands, x, y, got, want)
				}
			}
		}
		if got := at(dst, 0, 0); got != [4]byte{128, 128, 128, 255} {
			t.Fatalf("outside the layer = %v", got)
		}
	}
}

func TestBlendOfNames(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]blendMode{
		"": blendNormal, "normal": blendNormal, "multiply": blendMultiply, "screen": blendScreen,
		"overlay": blendOverlay, "add": blendAdd, "bogus": blendNormal,
	} {
		if got := blendOf(name); got != want {
			t.Errorf("blendOf(%q) = %d, want %d", name, got, want)
		}
	}
}

// mixGeneralRef is the W3C compositing of SDD-013 section 3.11.2 in floating point, premultiplied, for
// one channel: c x (1 - da) + dc x (1 - a) + a x da x B(cs, cd), with every value scaled to 0..255.
func mixGeneralRef(mode blendMode, c, a, dc, da uint32) float64 {
	cs, cd := float64(c)/float64(a), float64(dc)/float64(da)
	var b float64
	switch mode {
	case blendMultiply:
		b = cs * cd
	case blendScreen:
		b = cs + cd - cs*cd
	case blendOverlay:
		if cd <= 0.5 {
			b = 2 * cs * cd
		} else {
			b = cs + (2*cd - 1) - cs*(2*cd-1)
		}
	case blendAdd:
		b = min(cs+cd, 1)
	case blendNormal:
	}
	fa, fda := float64(a)/255, float64(da)/255

	return 255 * (float64(c)/255*(1-fda) + float64(dc)/255*(1-fa) + fa*fda*b)
}

// On a destination with alpha below 255 every mode follows the general form of SDD-013 section 3.11.2:
// a destination with alpha 0 takes the source, a partial one agrees with the W3C formula within two
// levels and takes the alpha of a source-over.
func TestBlendOverATransparentDestination(t *testing.T) {
	for _, mode := range []blendMode{blendMultiply, blendScreen, blendOverlay, blendAdd} {
		d := []byte{9, 9, 9, 0}
		mixPixel(d, 40, 80, 120, 200, mode)
		if want := []byte{40, 80, 120, 200}; string(d) != string(want) {
			t.Errorf("mode %d over alpha 0: got %v, want %v", mode, d, want)
		}

		for _, da := range []uint32{1, 64, 128, 200, 254} {
			for _, a := range []uint32{1, 90, 128, 255} {
				for _, sv := range []uint32{0, 60, 130, 255} {
					for _, dv := range []uint32{0, 50, 128, 255} {
						c, dc := min(sv*a/255, a), min(dv*da/255, da)
						d := []byte{byte(dc), byte(dc), byte(dc), byte(da)}
						mixPixel(d, c, c, c, a, mode)

						wantA := a + da - div255(a*da)
						if uint32(d[3]) != wantA {
							t.Fatalf("mode %d: alpha %d, want %d", mode, d[3], wantA)
						}
						if diff := math.Abs(float64(d[0]) - mixGeneralRef(mode, c, a, dc, da)); diff > 2 {
							t.Fatalf("mode %d src %d/%d dst %d/%d: got %d, off the W3C form by %.1f",
								mode, c, a, dc, da, d[0], diff)
						}
					}
				}
			}
		}
	}
}

// An opaque destination keeps the formulas of SDD-004 section 3.8 and its alpha.
func TestBlendOverAnOpaqueDestinationKeepsItsFormulas(t *testing.T) {
	for _, mode := range []blendMode{blendMultiply, blendScreen, blendOverlay, blendAdd} {
		d := []byte{100, 100, 100, 255}
		mixPixel(d, 50, 50, 50, 128, mode)

		want := blendChannel(mode, 50, 100, 128)
		if d[0] != want || d[3] != 255 {
			t.Errorf("mode %d: got %v, want colour %d and alpha 255", mode, d, want)
		}
	}
}
