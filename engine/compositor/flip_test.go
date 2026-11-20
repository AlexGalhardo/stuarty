package compositor

import (
	"bytes"
	"image"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// asymmetric is a premultiplied opaque image whose every pixel differs, so a wrong mirror shows.
func asymmetric(w, h int) Image {
	pix := make([]byte, w*h*4)
	for y := range h {
		for x := range w {
			i := (y*w + x) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = uint8(x*13+1), uint8(y*17+2), uint8(x*y+3), 255
		}
	}
	return Image{Pix: pix, W: w, H: h, Opaque: true}
}

func TestFlippedMirrorsPixels(t *testing.T) {
	t.Parallel()

	src := asymmetric(5, 3)
	orig := bytes.Clone(src.Pix)
	var c Compositor
	for _, tc := range []struct {
		name   string
		fx, fy bool
	}{{"x", true, false}, {"y", false, true}, {"both", true, true}} {
		got := c.flipped(src, tc.fx, tc.fy)
		for y := range 3 {
			for x := range 5 {
				sx, sy := x, y
				if tc.fx {
					sx = 4 - x
				}
				if tc.fy {
					sy = 2 - y
				}
				if !bytes.Equal(got.Pix[(y*5+x)*4:(y*5+x)*4+4], src.Pix[(sy*5+sx)*4:(sy*5+sx)*4+4]) {
					t.Fatalf("%s: pixel (%d,%d) is not the source (%d,%d)", tc.name, x, y, sx, sy)
				}
			}
		}
		if !bytes.Equal(src.Pix, orig) {
			t.Fatalf("%s: the source was written", tc.name)
		}
	}
	if got := c.flipped(src, false, false); &got.Pix[0] != &src.Pix[0] {
		t.Fatal("no flip should return the source itself")
	}
}

// composeFlip draws one 8 by 6 image into a 16 by 12 frame at the given rotation.
func composeFlip(t *testing.T, rot float64, fx, fy bool, bands int) []byte {
	t.Helper()

	src := asymmetric(8, 6)
	c := &Compositor{Bands: bands, Media: func(*plan.Layer) (Image, error) { return src, nil }}
	dst := image.NewRGBA(image.Rect(0, 0, 16, 12))
	fp := plan.FramePlan{Background: "#000000", Layers: []plan.Layer{{
		Kind: plan.Media, ItemID: "a", CX: 8, CY: 6, W: 8, H: 6, Rotation: rot, Opacity: 1, FlipX: fx, FlipY: fy,
	}}}
	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}
	return dst.Pix
}

func TestFlipCopyAndSamplerMirrorInsideTheRectangle(t *testing.T) {
	t.Parallel()

	// A flip of a layer rotated by 180 degrees is the opposite flip of the unrotated layer turned by 180:
	// the mirror is applied before the rotation, so FlipX at 180 equals FlipY at 0.
	if !bytes.Equal(composeFlip(t, 180, true, false, 1), composeFlip(t, 0, false, true, 1)) {
		t.Fatal("flip_x at 180 degrees differs from flip_y at 0")
	}
	if !bytes.Equal(composeFlip(t, 180, true, true, 1), composeFlip(t, 0, false, false, 1)) {
		t.Fatal("both flips at 180 degrees differ from the plain layer")
	}
	if bytes.Equal(composeFlip(t, 0, true, false, 1), composeFlip(t, 0, false, false, 1)) {
		t.Fatal("flip_x changed nothing")
	}
	// Two draws of the same flip, in one and eight bands, give the same bytes.
	for _, rot := range []float64{0, 20} {
		if !bytes.Equal(composeFlip(t, rot, true, true, 1), composeFlip(t, rot, true, true, 8)) {
			t.Fatalf("rotation %v: the bytes depend on the bands", rot)
		}
	}
}
