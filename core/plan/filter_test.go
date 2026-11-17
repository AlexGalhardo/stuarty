package plan

import (
	"math"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func apply(m *[12]float64, in [3]float64) [3]float64 {
	var out [3]float64
	for i := range out {
		out[i] = m[i*4]*in[0] + m[i*4+1]*in[1] + m[i*4+2]*in[2] + m[i*4+3]
	}

	return out
}

func TestFilterMatrix(t *testing.T) {
	t.Parallel()

	in := [3]float64{0.8, 0.4, 0.2}
	luma := 0.2126*in[0] + 0.7152*in[1] + 0.0722*in[2]

	for _, c := range []struct {
		name   string
		amount float64
		want   [3]float64
	}{
		{"grayscale", 1, [3]float64{luma, luma, luma}},
		{"invert", 1, [3]float64{0.2, 0.6, 0.8}},
		{"invert", 0.5, [3]float64{0.5, 0.5, 0.5}},
		{"sepia", 1, [3]float64{0.393*0.8 + 0.769*0.4 + 0.189*0.2, 0.349*0.8 + 0.686*0.4 + 0.168*0.2, 0.272*0.8 + 0.534*0.4 + 0.131*0.2}},
		{"warm", 1, [3]float64{0.88, 0.4, 0.18}},
		{"cool", 1, [3]float64{0.72, 0.4, 0.22}},
		{"cool", 0.5, [3]float64{0.76, 0.4, 0.21}},
	} {
		m := FilterMatrix(c.name, c.amount)
		if m == nil {
			t.Fatalf("%s %v: nil matrix", c.name, c.amount)
		}

		got := apply(m, in)
		for i := range got {
			if math.Abs(got[i]-c.want[i]) > 0.003 {
				t.Errorf("%s %v channel %d: %v, want %v", c.name, c.amount, i, got[i], c.want[i])
			}
		}
	}

	for _, c := range []struct {
		name   string
		amount float64
	}{{"none", 1}, {"", 1}, {"sharpen", 1}, {"sepia", 0}, {"sepia", -1}} {
		if FilterMatrix(c.name, c.amount) != nil {
			t.Errorf("FilterMatrix(%q, %v) must be nil", c.name, c.amount)
		}
	}

	if over := FilterMatrix("invert", 7); over == nil || *over != *FilterMatrix("invert", 1) {
		t.Error("an amount above 1 must act as 1")
	}
}

func TestLayerCarriesFilterAndBlur(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_f", trkBottom, 0, 10)
	it.Filter, it.FilterAmount, it.Blur = "sepia", 0.5, 12.3456
	put(d, it)

	l := Frame(d, 0).Layers[0]
	if l.Filter == nil || *l.Filter != *FilterMatrix("sepia", 0.5) {
		t.Errorf("layer filter %v", l.Filter)
	}

	near(t, "blur", l.Blur, 12.346)

	w := Window(d, 0, 10)
	if len(w.Layers) != 1 || w.Layers[0].Filter == nil || w.Layers[0].Blur != 12.346 {
		t.Errorf("window layer lost the filter or the blur: %+v", w.Layers)
	}

	plain := newDoc()
	put(plain, clip("itm_p", trkBottom, 0, 10))

	if pl := Frame(plain, 0).Layers[0]; pl.Filter != nil || pl.Blur != 0 {
		t.Errorf("an item with no filter must give none: %v %v", pl.Filter, pl.Blur)
	}

	sh := newDoc()
	shape := timeline.NewItem("itm_s", timeline.ItemShape, trkBottom, 0, 10)
	shape.Shape, shape.Width, shape.Height, shape.Blur = "rect", 10, 10, 4
	put(sh, shape)

	if Frame(sh, 0).Layers[0].Blur != 4 {
		t.Error("a bitmap layer carries the blur too")
	}
}

func TestLayerCarriesBlend(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_b", trkBottom, 0, 10)
	it.Blend = "screen"
	put(d, it)

	if l := Frame(d, 0).Layers[0]; l.Blend != "screen" {
		t.Errorf("layer blend %q, want screen", l.Blend)
	}

	if w := Window(d, 0, 10); len(w.Layers) != 1 || w.Layers[0].Blend != "screen" {
		t.Errorf("window layer lost the blend: %+v", w.Layers)
	}

	plain := newDoc()
	put(plain, clip("itm_p", trkBottom, 0, 10))

	if Frame(plain, 0).Layers[0].Blend != "" {
		t.Error("an item with no blend must give none")
	}
}

func TestLayerCarriesMask(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_m", trkBottom, 0, 10)
	it.Mask = &timeline.Mask{Shape: "ellipse", X: 0.1, Y: 0.2, Width: 0.5, Height: 0.60004, Feather: 12, Invert: true}
	put(d, it)

	want := MaskSpec{Shape: "ellipse", X: 0.1, Y: 0.2, Width: 0.5, Height: 0.6, Feather: 12, Invert: true}
	if l := Frame(d, 0).Layers[0]; l.Mask == nil || *l.Mask != want {
		t.Errorf("layer mask %+v, want %+v", l.Mask, want)
	}

	if w := Window(d, 0, 10); len(w.Layers) != 1 || w.Layers[0].Mask == nil || *w.Layers[0].Mask != want {
		t.Errorf("window layer lost the mask: %+v", w.Layers)
	}

	plain := newDoc()
	put(plain, clip("itm_p", trkBottom, 0, 10))

	if Frame(plain, 0).Layers[0].Mask != nil {
		t.Error("an item with no mask must give none")
	}
}

func TestLayerCarriesFlip(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_f", trkBottom, 0, 10)
	it.FlipX, it.FlipY = true, true
	put(d, it)

	if l := Frame(d, 0).Layers[0]; !l.FlipX || !l.FlipY {
		t.Errorf("layer flip %v %v, want both", l.FlipX, l.FlipY)
	}

	if w := Window(d, 0, 10); len(w.Layers) != 1 || !w.Layers[0].FlipX || !w.Layers[0].FlipY {
		t.Errorf("window layer lost the flip: %+v", w.Layers)
	}

	plain := newDoc()
	put(plain, clip("itm_p", trkBottom, 0, 10))

	if l := Frame(plain, 0).Layers[0]; l.FlipX || l.FlipY {
		t.Error("an item with no flip must give none")
	}
}

func TestLayerCarriesChromaKey(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_k", trkBottom, 0, 10)
	it.ChromaKey = &timeline.ChromaKey{Color: "#00FF00", Similarity: 0.30004, Smoothness: 0.1, Spill: 0.25}
	put(d, it)

	want := ChromaKeySpec{Color: "#00FF00", Similarity: 0.3, Smoothness: 0.1, Spill: 0.25}
	if l := Frame(d, 0).Layers[0]; l.ChromaKey == nil || *l.ChromaKey != want {
		t.Errorf("layer chroma key %+v, want %+v", l.ChromaKey, want)
	}

	if w := Window(d, 0, 10); len(w.Layers) != 1 || w.Layers[0].ChromaKey == nil || *w.Layers[0].ChromaKey != want {
		t.Errorf("window layer lost the chroma key: %+v", w.Layers)
	}

	plain := newDoc()
	put(plain, clip("itm_p", trkBottom, 0, 10))

	if Frame(plain, 0).Layers[0].ChromaKey != nil {
		t.Error("an item with no key must give none")
	}
}

func TestLayerCarriesTheGrade(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_g", trkBottom, 0, 10)
	it.Hsl = []timeline.HslBand{{Range: "red", Hue: 0.2}}
	it.Curves = &timeline.Curves{Master: []timeline.CurvePoint{{X: 0, Y: 0}, {X: 0.5, Y: 0.4}, {X: 1, Y: 1}}}
	put(d, it)

	if l := Frame(d, 0).Layers[0]; l.Grade == nil || len(l.Grade.Hsl) != 1 || l.Grade.Curves == nil ||
		l.Grade.Wheels != nil {
		t.Errorf("layer grade %+v", l.Grade)
	}

	if w := Window(d, 0, 10); len(w.Layers) != 1 || w.Layers[0].Grade == nil || len(w.Layers[0].Grade.Hsl) != 1 {
		t.Errorf("window layer lost the grade: %+v", w.Layers)
	}

	plain := newDoc()
	put(plain, clip("itm_p", trkBottom, 0, 10))

	if Frame(plain, 0).Layers[0].Grade != nil {
		t.Error("an item with no grade must give none")
	}
}

func TestLayerCarriesTheLut(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_l", trkBottom, 0, 10)
	it.Lut = &timeline.Lut{Asset: "ast_l000000001", Intensity: 0.6}
	put(d, it)

	want := LutSpec{Asset: "ast_l000000001", Intensity: 0.6}

	if l := Frame(d, 3).Layers[0]; l.Lut == nil || *l.Lut != want {
		t.Errorf("layer lut %+v, want %+v", l.Lut, want)
	}

	if w := Window(d, 0, 10); len(w.Layers) != 1 || w.Layers[0].Lut == nil || *w.Layers[0].Lut != want {
		t.Errorf("window layer lost the lut: %+v", w.Layers)
	}

	plain := newDoc()
	put(plain, clip("itm_p", trkBottom, 0, 10))

	if Frame(plain, 0).Layers[0].Lut != nil {
		t.Error("an item with no lut must give none")
	}
}

func TestAnimatedColourGivesEachFrameItsMatrix(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_a", trkBottom, 0, 10)
	it.Keyframes = timeline.Keyframes{"brightness": {
		{T: 0, Value: 0, Easing: "linear"}, {T: 10, Value: 0.5, Easing: "linear"},
	}}
	put(d, it)

	if m := Frame(d, 0).Layers[0].ColorMatrix; m != nil {
		t.Errorf("frame 0 has matrix %v, want none while brightness is 0", m)
	}

	if m := Frame(d, 5).Layers[0].ColorMatrix; m == nil || m[3] != 0.25 {
		t.Errorf("frame 5 matrix %v, want the offset of brightness 0.25", m)
	}

	w := Window(d, 0, 10).Layers[0]
	if len(w.ColorMatrices) != 10 || string(w.ColorMatrices[0]) != "null" || string(w.ColorMatrices[9]) == "null" {
		t.Errorf("window series %s, want 10 entries from null to a matrix", w.ColorMatrices)
	}

	still := newDoc()
	put(still, clip("itm_s", trkBottom, 0, 10))

	if w := Window(still, 0, 10).Layers[0]; w.ColorMatrices != nil {
		t.Error("a clip with no animated colour must carry no series")
	}
}

func TestSpinShrinkAndGrowTransitionsMoveTheLayer(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"spin", "shrink", "grow"} {
		d := newDoc()
		it := clip("itm_t", trkBottom, 0, 30)
		it.TransitionIn = &timeline.Transition{Kind: kind, Duration: 10}
		put(d, it)

		first, settled := Frame(d, 0).Layers[0], Frame(d, 15).Layers[0]
		if first.Scale >= settled.Scale && first.Rotation <= settled.Rotation {
			t.Errorf(
				"%s: frame 0 is scale %v rotation %v, the settled frame %v and %v",
				kind,
				first.Scale,
				first.Rotation,
				settled.Scale,
				settled.Rotation,
			)
		}

		if settled.Scale != 1 || settled.Rotation != 0 {
			t.Errorf("%s: the item must settle, got scale %v rotation %v", kind, settled.Scale, settled.Rotation)
		}
	}
}
