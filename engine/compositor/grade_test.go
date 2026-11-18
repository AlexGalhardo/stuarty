package compositor

import (
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func testGrade() *grade {
	return newGrade(&plan.GradeSpec{
		Hsl: []timeline.HslBand{
			{Range: "red", Hue: 0.5, Saturation: 0.4, Lightness: -0.2},
			{Range: "green", Saturation: -0.7},
		},
		Wheels: &timeline.Wheels{
			Shadows:    &timeline.Wheel{Hue: 220, Amount: 0.6, Luma: -0.2},
			Highlights: &timeline.Wheel{Hue: 359.9, Amount: 0.8, Luma: 0.3},
		},
		Curves: &timeline.Curves{
			Master: []timeline.CurvePoint{{X: 0, Y: 0.1}, {X: 0.3, Y: 0.2}, {X: 0.7, Y: 0.9}, {X: 1, Y: 1}},
			Blue:   []timeline.CurvePoint{{X: 0, Y: 1}, {X: 1, Y: 0}},
		},
	})
}

// The same pixels are pinned in renderer/vukem-renderer/src/grade.rs: an implementation that disagrees on one of
// them is not the grade of SDD-004 section 3.14.
var gradePinned = []struct{ in, want [3]int64 }{
	{[3]int64{200, 60, 40}, [3]int64{230, 33, 221}},
	{[3]int64{30, 220, 80}, [3]int64{102, 206, 184}},
	{[3]int64{128, 128, 128}, [3]int64{181, 100, 101}},
	{[3]int64{0, 0, 0}, [3]int64{26, 26, 208}},
	{[3]int64{255, 255, 255}, [3]int64{255, 246, 9}},
	{[3]int64{250, 250, 20}, [3]int64{255, 247, 229}},
	{[3]int64{10, 20, 240}, [3]int64{26, 26, 0}},
}

func TestGradePixelsArePinned(t *testing.T) {
	g := testGrade()
	for _, c := range gradePinned {
		r, gr, b := g.pixel(c.in[0], c.in[1], c.in[2])
		if got := [3]int64{r, gr, b}; got != c.want {
			t.Errorf("grade of %v = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestGradeThatChangesNothingIsNil(t *testing.T) {
	for name, spec := range map[string]*plan.GradeSpec{
		"nil":        nil,
		"empty":      {},
		"zero band":  {Hsl: []timeline.HslBand{{Range: "red"}}},
		"zero wheel": {Wheels: &timeline.Wheels{Shadows: &timeline.Wheel{Hue: 120}}},
		"no points":  {Curves: &timeline.Curves{}},
	} {
		if g := newGrade(spec); g != nil {
			t.Errorf("%s: a grade that changes no pixel is not nil", name)
		}
	}
}

// A grey pixel has no hue, so the bands leave it alone, and a straight curve through the corners is the identity.
func TestGradeGreysAndIdentityCurve(t *testing.T) {
	g := newGrade(&plan.GradeSpec{Hsl: []timeline.HslBand{{Range: "red", Hue: 1, Saturation: 1, Lightness: 1}}})
	for v := int64(0); v < 256; v += 5 {
		if r, gr, b := g.pixel(v, v, v); r != v || gr != v || b != v {
			t.Fatalf("the grey %d became %d %d %d", v, r, gr, b)
		}
	}

	id := newGrade(&plan.GradeSpec{Curves: &timeline.Curves{Master: []timeline.CurvePoint{{X: 0, Y: 0}, {X: 1, Y: 1}}}})
	for v := range 256 {
		if got := id.curves[0][v]; int(got) != v {
			t.Fatalf("the identity curve maps %d to %d", v, got)
		}
	}
}

// A curve through increasing points never goes down (the monotone cubic of SDD-004 section 3.14).
func TestGradeCurveIsMonotone(t *testing.T) {
	g := newGrade(&plan.GradeSpec{Curves: &timeline.Curves{Master: []timeline.CurvePoint{
		{X: 0, Y: 0}, {X: 0.1, Y: 0.05}, {X: 0.2, Y: 0.9}, {X: 0.5, Y: 0.9}, {X: 0.6, Y: 0.95}, {X: 1, Y: 1},
	}}})
	for v := 1; v < 256; v++ {
		if g.curves[0][v] < g.curves[0][v-1] {
			t.Fatalf("the curve goes down at %d: %d after %d", v, g.curves[0][v], g.curves[0][v-1])
		}
	}
}
