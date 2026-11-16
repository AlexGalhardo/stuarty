package compositor

import (
	"bytes"
	"image"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

func keyed(spec plan.ChromaKeySpec, matrix *[12]float64, pixels ...[4]byte) []byte {
	pix := make([]byte, 0, len(pixels)*4)
	for _, p := range pixels {
		pix = append(pix, p[:]...)
	}

	src := Image{Pix: pix, W: len(pixels), H: 1, Opaque: true}
	c := newComp(1, map[string]Image{"a": src})
	dst := image.NewRGBA(image.Rect(0, 0, len(pixels), 1))
	l := media("a", float64(len(pixels))/2, 0.5, float64(len(pixels)), 1, 0, 1)
	l.ChromaKey = &spec
	l.ColorMatrix = matrix

	if err := c.Compose(dst, plan.FramePlan{Background: "#000000", Layers: []plan.Layer{l}}, 1); err != nil {
		panic(err)
	}

	return dst.Pix
}

func TestIsqrt(t *testing.T) {
	t.Parallel()

	for n := range uint64(70000) {
		r := isqrt(n)
		if r*r > n || (r+1)*(r+1) <= n {
			t.Fatalf("isqrt(%d) = %d", n, r)
		}
	}

	for _, n := range []uint64{1 << 40, 1<<62 - 1, 1<<63 + 12345, ^uint64(0)} {
		r := isqrt(n)
		if r > 1<<32-1 || r*r > n || (r < 1<<32-1 && (r+1)*(r+1) <= n) {
			t.Fatalf("isqrt(%d) = %d", n, r)
		}
	}
}

// The key removes the key colour, keeps the colours far from it, and fades what is between (SDD-004 section 3.7).
func TestChromaKeyRemovesTheKeyColour(t *testing.T) {
	t.Parallel()

	green, red, blue := [4]byte{0, 255, 0, 255}, [4]byte{255, 0, 0, 255}, [4]byte{0, 0, 255, 255}
	nearGreen, farGreen := [4]byte{30, 230, 30, 255}, [4]byte{60, 200, 60, 255}

	got := keyed(plan.ChromaKeySpec{Color: "#00FF00", Similarity: 0.3}, nil, green, red, blue, nearGreen, farGreen)

	// On a black background a removed pixel shows black, a kept one shows itself.
	if !bytes.Equal(got[0:4], []byte{0, 0, 0, 255}) {
		t.Errorf("the key colour = %v, want the background", got[0:4])
	}

	if !bytes.Equal(got[4:8], red[:]) || !bytes.Equal(got[8:12], blue[:]) {
		t.Errorf("colours far from the key changed: %v %v", got[4:8], got[8:12])
	}

	if !bytes.Equal(got[12:16], []byte{0, 0, 0, 255}) {
		t.Errorf("a green close to the key = %v, want it removed", got[12:16])
	}

	// A green further than the similarity is a different colour and stays.
	if !bytes.Equal(got[16:20], farGreen[:]) {
		t.Errorf("a green beyond the similarity = %v, want it kept", got[16:20])
	}
}

func TestChromaKeySmoothnessFadesTheEdge(t *testing.T) {
	t.Parallel()

	// Between similarity and similarity + smoothness the pixel keeps a part of its opacity.
	k := newChromaKey(&plan.ChromaKeySpec{Color: "#00FF00", Similarity: 0.1, Smoothness: 0.4})
	if k.keep(0, 255, 0) != 0 {
		t.Error("the key colour is kept")
	}

	if k.keep(255, 0, 0) != 255 {
		t.Error("red is not kept whole")
	}

	prev := int64(-1)
	ramp := 0

	for g := int64(255); g >= 0; g -= 5 {
		f := k.keep(0, g, 0)
		if f < prev {
			t.Fatalf("the opacity falls from %d to %d as the pixel leaves the key colour", prev, f)
		}

		if f > 0 && f < 255 {
			ramp++
		}

		prev = f
	}

	if ramp < 5 {
		t.Errorf("the soft edge has %d steps", ramp)
	}

	// A hard edge has no ramp.
	hard := newChromaKey(&plan.ChromaKeySpec{Color: "#00FF00", Similarity: 0.1})
	for g := int64(255); g >= 0; g-- {
		if f := hard.keep(0, g, 0); f != 0 && f != 255 {
			t.Fatalf("smoothness 0 gave an opacity of %d", f)
		}
	}
}

func TestChromaKeySpillTakesTheKeyColourOutOfWhatStays(t *testing.T) {
	t.Parallel()

	// A skin tone with a green cast: green is above the mean of red and blue.
	px := [4]byte{200, 190, 120, 255}

	none := keyed(plan.ChromaKeySpec{Color: "#00FF00", Similarity: 0.05}, nil, px)
	full := keyed(plan.ChromaKeySpec{Color: "#00FF00", Similarity: 0.05, Spill: 1}, nil, px)
	half := keyed(plan.ChromaKeySpec{Color: "#00FF00", Similarity: 0.05, Spill: 0.5}, nil, px)

	if !bytes.Equal(none[:4], px[:]) {
		t.Fatalf("no spill changed the pixel: %v", none[:4])
	}

	// The limit is the mean of red and blue, 160; full spill brings green to it, half spill halfway.
	if full[1] != 160 || half[1] != 175 || full[0] != 200 || full[2] != 120 {
		t.Errorf("spill 1 gave %v and spill 0.5 gave %v", full[:4], half[:4])
	}

	// A key with no dominant channel has nothing to despill.
	grey := keyed(plan.ChromaKeySpec{Color: "#808080", Similarity: 0.01, Spill: 1}, nil, px)
	if !bytes.Equal(grey[:4], px[:]) {
		t.Errorf("a grey key despilled: %v", grey[:4])
	}
}

// The key works on the layer's own alpha: a half transparent pixel of the key colour is removed, a half
// transparent pixel of another colour keeps its half.
func TestChromaKeyOnPartiallyTransparentPixels(t *testing.T) {
	t.Parallel()

	// Premultiplied: green at 50% is (0, 128, 0, 128); red at 50% is (128, 0, 0, 128).
	src := Image{Pix: []byte{0, 128, 0, 128, 128, 0, 0, 128}, W: 2, H: 1}
	c := newComp(1, map[string]Image{"a": src})
	dst := image.NewRGBA(image.Rect(0, 0, 2, 1))
	l := media("a", 1, 0.5, 2, 1, 0, 1)
	l.ChromaKey = &plan.ChromaKeySpec{Color: "#00FF00", Similarity: 0.3}

	if err := c.Compose(dst, plan.FramePlan{Background: "#000000", Layers: []plan.Layer{l}}, 1); err != nil {
		t.Fatal(err)
	}

	if got := at(dst, 0, 0); got != [4]byte{0, 0, 0, 255} {
		t.Errorf("transparent green = %v", got)
	}

	if got := at(dst, 1, 0); got != [4]byte{128, 0, 0, 255} {
		t.Errorf("half red over black = %v", got)
	}
}

// The key is applied before the colour matrix: the matrix then acts on what the key left.
func TestChromaKeyComesBeforeTheColourMatrix(t *testing.T) {
	t.Parallel()

	invert := &[12]float64{-1, 0, 0, 1, 0, -1, 0, 1, 0, 0, -1, 1}
	green, red := [4]byte{0, 255, 0, 255}, [4]byte{255, 0, 0, 255}

	got := keyed(plan.ChromaKeySpec{Color: "#00FF00", Similarity: 0.3}, invert, green, red)

	// Green is keyed out first (so the inversion does not turn it into magenta), red is inverted to cyan.
	if !bytes.Equal(got[0:4], []byte{0, 0, 0, 255}) || !bytes.Equal(got[4:8], []byte{0, 255, 255, 255}) {
		t.Errorf("got %v", got)
	}
}

// A keyed layer is never drawn by the opaque copy path: its alpha is no longer 255.
func TestChromaKeyedLayerIsNotOpaque(t *testing.T) {
	t.Parallel()

	got := keyed(
		plan.ChromaKeySpec{Color: "#00FF00", Similarity: 0.5},
		nil,
		[4]byte{0, 255, 0, 255},
		[4]byte{9, 9, 9, 255},
	)
	if !bytes.Equal(got[0:4], []byte{0, 0, 0, 255}) {
		t.Errorf("a keyed pixel was copied: %v", got[0:4])
	}
}

func TestChromaKeyBandsEqual(t *testing.T) {
	t.Parallel()

	src := pattern(64, 48, 11)
	spec := plan.ChromaKeySpec{Color: "#3AC83A", Similarity: 0.35, Smoothness: 0.2, Spill: 0.6}
	render := func(bands int) []byte {
		c := newComp(bands, map[string]Image{"a": src})
		dst := image.NewRGBA(image.Rect(0, 0, 80, 60))
		l := media("a", 40, 30, 64, 48, 12, 0.9)
		l.ChromaKey = &spec

		if err := c.Compose(dst, plan.FramePlan{Background: "#202020", Layers: []plan.Layer{l}}, 1); err != nil {
			t.Fatal(err)
		}

		return dst.Pix
	}

	if !bytes.Equal(render(1), render(8)) {
		t.Fatal("one band and eight bands differ")
	}
}

func TestChromaKeyAbsentChangesNothing(t *testing.T) {
	t.Parallel()

	src := pattern(16, 12, 5)
	c := newComp(1, map[string]Image{"a": src})
	dst := image.NewRGBA(image.Rect(0, 0, 16, 12))

	if err := c.Compose(dst, plan.FramePlan{Layers: []plan.Layer{media("a", 8, 6, 16, 12, 0, 1)}}, 1); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(dst.Pix, src.Pix) {
		t.Fatal("a layer with no key changed")
	}
}
