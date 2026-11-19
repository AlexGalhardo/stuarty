package compositor

import (
	"bytes"
	"image"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// groupOf is a group layer of a nested frame of w by h pixels placed at (cx, cy) with the same size.
func groupOf(id string, cx, cy, w, h, op float64, layers ...plan.Layer) plan.Layer {
	return plan.Layer{
		ItemID: id, Kind: plan.Group, CX: cx, CY: cy, W: w, H: h, Opacity: op,
		Group: &plan.GroupSpec{Width: int(w), Height: int(h), Layers: layers},
	}
}

// plain is a column of 8 rows, w wide, of one opaque colour.
func plain(w int, r, g, b byte) Image {
	const h = 8

	pix := make([]byte, w*h*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = r, g, b, 255
	}

	return Image{Pix: pix, W: w, H: h, Opaque: true}
}

func TestGroupGeometry(t *testing.T) {
	t.Parallel()

	g := &plan.GroupSpec{Width: 1000, Height: 500}
	cases := []struct {
		name         string
		layerW, out  float64
		wantK        float64
		wantW, wantH int
	}{
		{"drawn at its own size", 1000, 1, 1, 1000, 500},
		{"half the output scale", 1000, 0.5, 0.5, 500, 250},
		{"a cover that is larger than the frame", 3000, 1, 3, 3000, 1500},
		{"a surface over 4096 is lowered", 6000, 1, 4.096, 4096, 2048},
		{"rounds to the nearest pixel", 1001.2, 0.5, 0.5006, 501, 250},
	}

	for _, c := range cases {
		k, w, h := GroupGeometry(g, c.layerW, c.out)
		if w != c.wantW || h != c.wantH || k < c.wantK-1e-9 || k > c.wantK+1e-9 {
			t.Errorf("%s: k=%v %dx%d, want k=%v %dx%d", c.name, k, w, h, c.wantK, c.wantW, c.wantH)
		}
	}

	if k, w, h := GroupGeometry(nil, 100, 1); k != 0 || w != 0 || h != 0 {
		t.Errorf("a nil group has geometry %v %dx%d", k, w, h)
	}
}

// The point of a group: its layers are composed first, so a group at half opacity does not show one layer
// through the other.
func TestGroupIsDrawnAsOneUnit(t *testing.T) {
	t.Parallel()

	imgs := map[string]Image{"a": plain(8, 200, 0, 0), "b": plain(8, 0, 0, 200)}
	// a covers the nested frame; b covers its right half. Both are opaque.
	a, b := media("a", 4, 4, 8, 8, 0, 1), media("b", 6, 4, 4, 8, 0, 1)
	b.W = 4
	imgs["b"] = plain(4, 0, 0, 200)

	c := newComp(1, imgs)
	dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
	fp := plan.FramePlan{
		Width:      8,
		Height:     8,
		Background: "#000000",
		Layers:     []plan.Layer{groupOf("g", 4, 4, 8, 8, 0.5, a, b)},
	}

	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}

	// Left half: a at 50 % over black. Right half: b at 50 % over black, nothing of a (128 is 0.5 of 255).
	if got := at(dst, 1, 4); got[0] != 100 || got[1] != 0 || got[2] != 0 {
		t.Errorf("left half = %v, want a at half opacity over black", got)
	}

	if got := at(dst, 6, 4); got[0] != 0 || got[2] != 100 {
		t.Errorf("right half = %v, want b at half opacity and no a under it", got)
	}
}

// Step 4 of SDD-013 section 3.12: a group is its surface drawn as a media layer with the same placement.
func TestGroupEqualsItsSurfaceDrawnAsMedia(t *testing.T) {
	t.Parallel()

	imgs := map[string]Image{"a": pattern(30, 20, 3), "b": pattern(10, 10, 4)}
	a, b := media("a", 15, 10, 30, 20, 0, 1), media("b", 20, 6, 10, 10, 0, 0.7)

	for _, rot := range []float64{0, 17} {
		g := groupOf("g", 31, 26, 30, 20, 0.8, a, b)
		g.Rotation = rot
		g.Blend = "multiply"

		c := newComp(1, imgs)
		got := image.NewRGBA(image.Rect(0, 0, 64, 48))
		fp := plan.FramePlan{Width: 64, Height: 48, Background: "#304050", Layers: []plan.Layer{g}}

		if err := c.Compose(got, fp, 1); err != nil {
			t.Fatal(err)
		}

		surface := image.NewRGBA(image.Rect(0, 0, 30, 20))
		nested := plan.FramePlan{
			Width:       30,
			Height:      20,
			Background:  "#000000",
			Transparent: true,
			Layers:      []plan.Layer{a, b},
		}

		if err := newComp(1, imgs).Compose(surface, nested, 1); err != nil {
			t.Fatal(err)
		}

		m := media("surface", 31, 26, 30, 20, rot, 0.8)
		m.Blend = "multiply"

		want := image.NewRGBA(image.Rect(0, 0, 64, 48))
		wc := newComp(1, map[string]Image{"surface": {Pix: surface.Pix, W: 30, H: 20}})

		if err := wc.Compose(
			want,
			plan.FramePlan{Width: 64, Height: 48, Background: "#304050", Layers: []plan.Layer{m}},
			1,
		); err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Pix, want.Pix) {
			t.Errorf("rotation %v: the group differs from its surface drawn as a media layer", rot)
		}
	}
}

func TestGroupBandsEqual(t *testing.T) {
	t.Parallel()

	imgs := map[string]Image{"a": pattern(60, 40, 5), "b": pattern(20, 20, 6)}
	a, b := media("a", 30, 20, 60, 40, 0, 1), media("b", 40, 25, 20, 20, 30, 0.6)
	g := groupOf("g", 50, 40, 60, 40, 0.75, a, b)
	g.Rotation = 9
	fp := plan.FramePlan{Width: 100, Height: 80, Background: "#101010", Layers: []plan.Layer{g}}

	var first []byte

	for _, bands := range []int{1, 3, 8} {
		dst := image.NewRGBA(image.Rect(0, 0, 100, 80))
		if err := newComp(bands, imgs).Compose(dst, fp, 1); err != nil {
			t.Fatal(err)
		}

		if first == nil {
			first = dst.Pix
		} else if !bytes.Equal(first, dst.Pix) {
			t.Fatalf("%d bands draw other bytes than one", bands)
		}
	}
}

func TestGroupInsideAGroup(t *testing.T) {
	t.Parallel()

	imgs := map[string]Image{"a": plain(8, 0, 200, 0)}
	inner := groupOf("inner", 4, 4, 8, 8, 0.5, media("a", 4, 4, 8, 8, 0, 1))
	outer := groupOf("outer", 4, 4, 8, 8, 0.5, inner)

	dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
	fp := plan.FramePlan{Width: 8, Height: 8, Background: "#000000", Layers: []plan.Layer{outer}}

	if err := newComp(1, imgs).Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}

	// 0.5 of 0.5 of 200: 50.
	if got := at(dst, 4, 4); got[1] < 49 || got[1] > 51 || got[0] != 0 {
		t.Errorf("two groups at half opacity give %v, want green near 50", got)
	}
}

func TestGroupScopeIsCalledAtTheNestedScaleAndRestored(t *testing.T) {
	t.Parallel()

	var scales []float64

	restored := 0
	imgs := map[string]Image{"a": plain(8, 9, 9, 9)}
	c := newComp(1, imgs)
	c.Scoped = func(scale float64) func() {
		scales = append(scales, scale)

		return func() { restored++ }
	}

	// A group drawn at 20 pixels wide in a frame composed at scale 0.5: the nested frame of 8 pixels is drawn at
	// 0.5 * 20 / 8 = 1.25 and its surface is 10 pixels.
	g := groupOf("g", 10, 10, 8, 8, 1, media("a", 4, 4, 8, 8, 0, 1))
	g.W, g.H = 20, 20
	g.Group.Width, g.Group.Height = 8, 8

	dst := image.NewRGBA(image.Rect(0, 0, 20, 20))
	if err := c.Compose(
		dst,
		plan.FramePlan{Width: 40, Height: 40, Background: "#000000", Layers: []plan.Layer{g}},
		0.5,
	); err != nil {
		t.Fatal(err)
	}

	if len(scales) != 1 || scales[0] != 1.25 || restored != 1 {
		t.Errorf("scoped %v, restored %d times, want [1.25] and once", scales, restored)
	}
}

func TestGroupWithNoFrameIsAnError(t *testing.T) {
	t.Parallel()

	g := plan.Layer{ItemID: "g", Kind: plan.Group, CX: 4, CY: 4, W: 8, H: 8, Opacity: 1}
	dst := image.NewRGBA(image.Rect(0, 0, 8, 8))

	if err := newComp(1, nil).Compose(dst, plan.FramePlan{Layers: []plan.Layer{g}}, 1); err == nil {
		t.Error("a group with no frame drew without an error")
	}
}
