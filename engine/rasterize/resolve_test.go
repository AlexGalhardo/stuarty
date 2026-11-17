package rasterize

import (
	"bytes"
	"errors"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

func TestParseColour(t *testing.T) {
	t.Parallel()

	good := map[string]colour{
		"#FFFFFF":   {255, 255, 255, 255},
		"#00000080": {0, 0, 0, 128},
		"#1a2B3c":   {0x1a, 0x2b, 0x3c, 255},
		"#FFE60099": {255, 230, 0, 0x99},
	}

	for in, want := range good {
		if got, err := parseColour(in); err != nil || got != want {
			t.Errorf("parseColour(%q) = %v, %v, want %v", in, got, err, want)
		}
	}

	for _, in := range []string{"", "FFFFFF", "#FFF", "#GGGGGG", "#FFFFFFF", "red", "#+FFFFF"} {
		if _, err := parseColour(in); !errors.Is(err, ErrInvalidSpec) {
			t.Errorf("parseColour(%q) error = %v, want ErrInvalidSpec", in, err)
		}
	}
}

func TestRenderRejectsBadColours(t *testing.T) {
	t.Parallel()

	for _, field := range []func(*plan.ResolvedStyle){
		func(s *plan.ResolvedStyle) { s.Color = "white" },
		func(s *plan.ResolvedStyle) { s.StrokeColor = "" },
		func(s *plan.ResolvedStyle) { s.ShadowColor = "#12" },
		func(s *plan.ResolvedStyle) { s.BackgroundColor = "nope" },
		func(s *plan.ResolvedStyle) { s.HighlightColor = "nope" },
		func(s *plan.ResolvedStyle) { s.HighlightBackground = "nope" },
	} {
		st := plainStyle()
		field(&st)

		if _, err := Render(textSpec("x", st), Bundled()); !errors.Is(err, ErrInvalidSpec) {
			t.Errorf("error = %v, want ErrInvalidSpec", err)
		}
	}
}

// An empty background colour means no box, and the style "none" turns the
// highlight off: the active word is drawn like the others.
func TestPlanStyleConventions(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	st.HighlightColor = C(255, 0, 0, 255)

	words := []string{"one", "two"}

	st.Highlight = "none"
	off := render(t, captionSpec(words, 1, st))
	plain := render(t, captionSpec(words, -1, st))

	if !bytes.Equal(off.Pix, plain.Pix) {
		t.Error("highlight none: the active word changed the bitmap")
	}

	st.Highlight = "word"
	on := render(t, captionSpec(words, 1, st))

	if bytes.Equal(on.Pix, plain.Pix) {
		t.Error("highlight word: the active word did not change the bitmap")
	}
}

// The plan places a bitmap by the size the Measurer gives: it must be exactly
// the natural size Render produces, for every kind and every state.
func TestMeasureEqualsTheRenderedSize(t *testing.T) {
	t.Parallel()

	measure := Measure(Bundled())

	for _, c := range goldenCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			b := render(t, c.spec)

			w, h := measure(c.spec)
			if w != b.NaturalW || h != b.NaturalH {
				t.Errorf("Measure = %v x %v, Render = %v x %v", w, h, b.NaturalW, b.NaturalH)
			}
		})
	}
}

func TestMeasureOfAnInvalidSpecIsZero(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	st.FontFamily = "Papyrus"

	for name, spec := range map[string]plan.BitmapSpec{
		"empty":        {},
		"unknown font": textSpec("x", st),
		"bad colour":   textSpec("x", func() plan.ResolvedStyle { s := plainStyle(); s.Color = "x"; return s }()),
	} {
		if w, h := Measure(Bundled())(spec); w != 0 || h != 0 {
			t.Errorf("%s: Measure = %v x %v, want 0 x 0", name, w, h)
		}
	}
}

func TestCacheIsKeyedByThePlanKey(t *testing.T) {
	t.Parallel()

	c := NewCache(Bundled(), 0)
	spec := textSpec("Keyed", plainStyle())

	if _, err := c.Get(spec); err != nil {
		t.Fatal(err)
	}

	if _, ok := c.items[spec.Key()]; !ok {
		t.Error("the bitmap is not stored under plan.BitmapSpec.Key()")
	}
}
