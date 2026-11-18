package compositor

import (
	"image"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

func redSquare() Image {
	pix := make([]byte, 2*2*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+3] = 255, 255
	}
	return Image{Pix: pix, W: 2, H: 2, Opaque: true}
}

func layerAt(sh *plan.ShadowSpec, ol *plan.OutlineSpec) plan.Layer {
	l := media("a", 4, 4, 2, 2, 0, 1)
	l.Shadow, l.Outline = sh, ol
	return l
}

func TestShadowOffsetPaintsUnderThePicture(t *testing.T) {
	t.Parallel()
	sh := &plan.ShadowSpec{Color: "#00FF00", Opacity: 1, DX: 2}
	one := composeOne(t, redSquare(), layerAt(sh, nil), 10, 10, 0, 1)
	eight := composeOne(t, redSquare(), layerAt(sh, nil), 10, 10, 0, 8)
	if !bytesEqual(one, eight) {
		t.Fatal("1 band and 8 bands differ")
	}
	// Picture at (3,3)-(4,4). Shadow of dx 2, no blur, is the same square two pixels to the right.
	for _, p := range []image.Point{{3, 3}, {4, 3}, {3, 4}, {4, 4}} {
		if got := at(one, p.X, p.Y); got != [4]byte{255, 0, 0, 255} {
			t.Errorf("picture %v: %v", p, got)
		}
	}
	for _, p := range []image.Point{{5, 3}, {6, 3}, {5, 4}, {6, 4}} {
		if got := at(one, p.X, p.Y); got != [4]byte{0, 255, 0, 255} {
			t.Errorf("shadow %v: %v", p, got)
		}
	}
	if got := at(one, 2, 3); got != [4]byte{0, 0, 0, 255} {
		t.Errorf("the pixel left of the picture: %v", got)
	}
}

func TestShadowColourAlphaScalesTheDrop(t *testing.T) {
	t.Parallel()
	sh := &plan.ShadowSpec{Color: "#00FF0080", Opacity: 1, DX: 2}
	dst := composeOne(t, redSquare(), layerAt(sh, nil), 10, 10, 0, 1)
	// factor = div255(255*128) = 128. Over the opaque black frame that is green 128, alpha 255.
	if got := at(dst, 5, 3); got != [4]byte{0, 128, 0, 255} {
		t.Errorf("8-digit shadow: %v", got)
	}
	half := &plan.ShadowSpec{Color: "#00FF00", Opacity: 0.5, DX: 2}
	dst = composeOne(t, redSquare(), layerAt(half, nil), 10, 10, 0, 1)
	if got := at(dst, 5, 3); got != [4]byte{0, 128, 0, 255} {
		t.Errorf("opacity 0.5: %v", got)
	}
}

func TestOutlineDilatesTheSilhouette(t *testing.T) {
	t.Parallel()
	ol := &plan.OutlineSpec{Color: "#FFFFFF", Width: 1}
	one := composeOne(t, redSquare(), layerAt(nil, ol), 10, 10, 0, 1)
	eight := composeOne(t, redSquare(), layerAt(nil, ol), 10, 10, 0, 8)
	if !bytesEqual(one, eight) {
		t.Fatal("1 band and 8 bands differ")
	}
	// Width 1 is a disc of radius 1: the orthogonal neighbours, not the diagonal.
	if got := at(one, 2, 3); got != [4]byte{255, 255, 255, 255} {
		t.Errorf("outline: %v", got)
	}
	if got := at(one, 3, 3); got != [4]byte{255, 0, 0, 255} {
		t.Errorf("picture over the outline: %v", got)
	}
	if got := at(one, 2, 2); got != [4]byte{0, 0, 0, 255} {
		t.Errorf("diagonal outside the disc: %v", got)
	}
}

func TestInvisibleShadowDoesNotChangeThePicture(t *testing.T) {
	t.Parallel()
	plain := composeOne(t, redSquare(), layerAt(nil, nil), 10, 10, 0, 1)
	sh := &plan.ShadowSpec{Color: "#00FF00", Opacity: 0, DX: 2, Blur: 4}
	got := composeOne(t, redSquare(), layerAt(sh, nil), 10, 10, 0, 1)
	if !bytesEqual(plain, got) {
		t.Fatal("a shadow of opacity 0 changed the frame")
	}
}

func bytesEqual(a, b *image.RGBA) bool {
	if len(a.Pix) != len(b.Pix) {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}
