package raster

import (
	"math"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

func layoutOf(t *testing.T, text string, st plan.ResolvedStyle, scale float64) layout {
	t.Helper()

	face, err := Bundled().Lookup(st.FontFamily, st.FontWeight)
	if err != nil {
		t.Fatal(err)
	}

	runes := []rune(text)

	return layoutText(runes, nil, face, mustStyle(t, st), scale)
}

func lineCount(lay layout, st plan.ResolvedStyle) int {
	return int(math.Round(lay.height / (st.Size * st.LineHeight)))
}

func TestLinesWrapAtMaxWidth(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	st.MaxWidth = 400

	lay := layoutOf(t, "The quick brown fox jumps over the lazy dog and keeps running", st, 1)

	if n := lineCount(lay, st); n < 3 {
		t.Fatalf("lines = %d, want at least 3", n)
	}

	if lay.width > st.MaxWidth+0.5 {
		t.Errorf("block width %.1f is wider than max_width %.0f", lay.width, st.MaxWidth)
	}
}

func TestForcedBreaksAlwaysBreak(t *testing.T) {
	t.Parallel()

	st := plainStyle()

	if n := lineCount(layoutOf(t, "one\ntwo\n\nfour", st, 1), st); n != 4 {
		t.Errorf("lines = %d, want 4 (an empty line counts)", n)
	}

	if n := lineCount(layoutOf(t, "one\n", st, 1), st); n != 2 {
		t.Errorf("trailing break: lines = %d, want 2", n)
	}
}

func TestLongWordOverflowsOnItsOwnLine(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	st.MaxWidth = 200

	lay := layoutOf(t, "a Supercalifragilisticexpialidocious b", st, 1)

	if lay.width <= st.MaxWidth {
		t.Errorf("block width %.1f: the long word should overflow max_width %.0f", lay.width, st.MaxWidth)
	}

	if n := lineCount(lay, st); n != 3 {
		t.Errorf("lines = %d, want 3: a, the long word, b", n)
	}
}

func TestLineHeightIsSizeTimesLineHeight(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	st.LineHeight = 1.5

	lay := layoutOf(t, "a\nb\nc", st, 1)
	if want := 3 * 72 * 1.5; math.Abs(lay.height-want) > 1e-9 {
		t.Errorf("height = %v, want %v", lay.height, want)
	}
}

func TestAlignInsideTheBlock(t *testing.T) {
	t.Parallel()

	firstX := func(align string) float64 {
		st := plainStyle()
		st.Align = align
		lay := layoutOf(t, "Wide wide line\ni", st, 1)

		// The glyph of the second line: the last glyph placed.
		return lay.glyphs[len(lay.glyphs)-1].x - lay.width
	}

	left, center, right := firstX(AlignLeft), firstX(AlignCenter), firstX(AlignRight)
	if !(left < center && center < right) {
		t.Errorf("x of the short line: left %.1f, center %.1f, right %.1f; want increasing", left, center, right)
	}

	if math.Abs((center-left)*2-(right-left)) > 1e-6 {
		t.Errorf("center is not halfway: %.1f %.1f %.1f", left, center, right)
	}
}

func TestUppercaseKeepsTheRuneCount(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	st.Uppercase = true

	runes, spans := textRunes(captionSpec([]string{"straße", "ok"}, -1, st))
	if string(runes) != "STRAßE OK" {
		t.Errorf("runes = %q", string(runes))
	}

	if want := [][2]int{
		{0, 6},
		{7, 9},
	}; !slices.Equal(spans[0][:], want[0][:]) ||
		!slices.Equal(spans[1][:], want[1][:]) {
		t.Errorf("spans = %v, want %v", spans, want)
	}
}

func TestCaptionBreaksForceLines(t *testing.T) {
	t.Parallel()

	spec := captionSpec([]string{"a", "b", "c", "d"}, -1, plainStyle())
	spec.Breaks = []int{2}

	runes, _ := textRunes(spec)
	if string(runes) != "a b\nc d" {
		t.Errorf("text = %q", string(runes))
	}
}

// TestRightToLeftRunsAreReordered feeds Hebrew inside English text. None of the
// bundled fonts has Hebrew glyphs, so the letters are .notdef, but the order in
// which the library places the clusters shows whether the Unicode
// bidirectional algorithm ran: the Hebrew run must read right to left.
func TestRightToLeftRunsAreReordered(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	lay := layoutOf(t, "abc אבג def", st, 1)

	glyphs := slices.Clone(lay.glyphs)
	slices.SortStableFunc(glyphs, func(a, b placedGlyph) int {
		switch {
		case a.x < b.x:
			return -1
		case a.x > b.x:
			return 1
		}

		return 0
	})

	got := make([]int, len(glyphs))
	for i, g := range glyphs {
		got[i] = g.text
	}

	if want := []int{0, 1, 2, 3, 6, 5, 4, 7, 8, 9, 10}; !slices.Equal(got, want) {
		t.Errorf("visual order of clusters = %v, want %v", got, want)
	}
}

func TestRightToLeftParagraphStartsOnTheRight(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	lay := layoutOf(t, "אבג abc", st, 1)

	for _, g := range lay.glyphs {
		if g.text == 0 && g.x < lay.width/2 {
			t.Errorf(
				"the first letter of a right-to-left paragraph is at x=%.1f of %.1f: want the right half",
				g.x,
				lay.width,
			)
		}
	}

	if !slices.ContainsFunc(lay.glyphs, func(g placedGlyph) bool { return g.text == 0 }) {
		t.Fatal("no glyph for the first rune")
	}
}

func TestScaleMultipliesTheBlock(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	one := layoutOf(t, "Scale me", st, 1)
	two := layoutOf(t, "Scale me", st, 2)

	if r := two.width / one.width; math.Abs(r-2) > 0.02 {
		t.Errorf("width ratio = %.4f, want 2", r)
	}

	if r := two.height / one.height; math.Abs(r-2) > 1e-9 {
		t.Errorf("height ratio = %.4f, want 2", r)
	}
}
