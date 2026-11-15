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
