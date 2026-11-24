package command

import (
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func luminance(t *testing.T, c timeline.Color) float64 {
	t.Helper()
	ch := func(i int) float64 {
		v, err := strconv.ParseUint(string(c)[1+2*i:3+2*i], 16, 8)
		if err != nil {
			t.Fatalf("colour %q: %v", c, err)
		}
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(0) + 0.7152*ch(1) + 0.0722*ch(2)
}

func contrast(t *testing.T, a, b timeline.Color) float64 {
	t.Helper()
	la, lb := luminance(t, a), luminance(t, b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

// SDD-002 C.88: 14 styles, each with a bundled font and weight, valid at every frame size, and a text
// colour that reads against its box or its edge (4.5:1).
func TestCaptionStylesAreValidAndReadable(t *testing.T) {
	t.Parallel()

	if got := len(captionStyles); got != 14 {
		t.Fatalf("%d caption styles, want 14", got)
	}

	seen := map[string]bool{}

	for _, spec := range captionStyles {
		if seen[spec.ID] {
			t.Errorf("duplicate style id %s", spec.ID)
		}
		seen[spec.ID] = true

		s := spec.Style
		weights, bundled := timeline.BundledWeights[*s.FontFamily]
		if !bundled || !slices.Contains(timeline.BundledFamilies, *s.FontFamily) {
			t.Errorf("%s: font %q is not a bundled family", spec.ID, *s.FontFamily)
		} else if !slices.Contains(weights, *s.FontWeight) {
			t.Errorf("%s: %s has no weight %d", spec.ID, *s.FontFamily, *s.FontWeight)
		}

		backing := s.BackgroundColor
		if backing == nil {
			backing = s.StrokeColor
		}

		if backing == nil {
			t.Errorf("%s: no box and no stroke to read against", spec.ID)
		} else if c := contrast(t, *s.Color, *backing); c < 4.5 {
			t.Errorf("%s: text %s on %s has contrast %.2f, want 4.5", spec.ID, *s.Color, *backing, c)
		}

		for _, size := range [][2]int{{1080, 1920}, {1920, 1080}, {720, 1280}, {3840, 2160}, {240, 240}} {
			r := ResolveCaptionStyle(spec, size[0], size[1])
			if v := timeline.CaptionStyleViolations(r); len(v) != 0 {
				t.Errorf("%s at %dx%d: %v", spec.ID, size[0], size[1], v)
			}
		}
	}
}

// The lengths follow the shorter side of the project (C.88), and the table is not changed by a result.
func TestResolveCaptionStyleScalesAndDoesNotShareTheTable(t *testing.T) {
	t.Parallel()

	spec, _ := CaptionStyleByID("thick_outline")
	ref := ResolveCaptionStyle(spec, 1080, 1920)
	half := ResolveCaptionStyle(spec, 540, 960)
	wide := ResolveCaptionStyle(spec, 1920, 1080)

	if *ref.Size != 80 || *half.Size != 40 || *wide.Size != 80 {
		t.Errorf("sizes %v, %v, %v, want 80, 40, 80", *ref.Size, *half.Size, *wide.Size)
	}

	if *half.StrokeWidth != 7 {
		t.Errorf("half-size stroke %v, want 7", *half.StrokeWidth)
	}

	*ref.Size = 1
	if again, _ := CaptionStyleByID("thick_outline"); *again.Style.Size != 80 {
		t.Error("changing a resolved style changed the table")
	}

	bar, _ := CaptionStyleByID("lower_bar")
	if got := *ResolveCaptionStyle(bar, 1080, 1920).MaxWidth; got != 929 {
		t.Errorf("lower_bar max_width %v, want 929 (0.86 of 1080)", got)
	}
}
