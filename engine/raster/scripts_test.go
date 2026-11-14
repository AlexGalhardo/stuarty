package raster

import (
	"bytes"
	"testing"
)

// The tests below are the measurements of risk R3 (ADR-0012): what the
// pipeline does with emoji and with right-to-left text when the only fonts are
// the six bundled families.

// TestEmojiDrawsTheNotdefGlyph records the emoji result: no bundled family has
// an emoji glyph, so an emoji is drawn with the .notdef glyph of the font and
// the rest of the line is laid out normally (SDD-004 section 5.1 item 9). The
// shaping library can draw colour glyphs (COLR, sbix, CBDT) when a font has
// them, but no open colour emoji font small enough to bundle was approved.
func TestEmojiDrawsTheNotdefGlyph(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	face, err := Bundled().Lookup(st.FontFamily, st.FontWeight)
	if err != nil {
		t.Fatal(err)
	}

	runes := []rune("A\U0001F600B")
	lay := layoutText(runes, nil, face, mustStyle(t, st), 1)

	var notdef []int

	for _, g := range lay.glyphs {
		if g.gid == 0 {
			notdef = append(notdef, g.text)
		}
	}

	// The emoji is one rune in UTF-32 (the cluster index is a rune index).
	if len(notdef) != 1 || notdef[0] != 1 {
		t.Fatalf(".notdef glyphs at runes %v, want only the emoji at 1", notdef)
	}

	// It is drawn, not skipped: the bitmap with the emoji has ink where the
	// same text without it has none, and no glyph of another font replaced it.
	with := render(t, textSpec("A\U0001F600B", st))
	without := render(t, textSpec("AB", st))

	if with.Width <= without.Width {
		t.Errorf("the emoji took no room: %d px against %d", with.Width, without.Width)
	}

	if bytes.Equal(with.Pix, without.Pix) {
		t.Error("the .notdef glyph was not drawn")
	}
}

// TestEmojiSequencesStayTogether covers the variation selector and the zero
// width joiner: a sequence is one cluster, so the layout and the caption word
// boxes do not split it.
func TestEmojiSequencesStayTogether(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	face, _ := Bundled().Lookup(st.FontFamily, st.FontWeight)

	// Family emoji: man, ZWJ, woman, ZWJ, girl.
	text := "\U0001F468\u200d\U0001F469\u200d\U0001F467"
	lay := layoutText([]rune(text), [][2]int{{0, 5}}, face, mustStyle(t, st), 1)

	if len(lay.words) != 1 || lay.words[0].W <= 0 {
		t.Fatalf("word box of the sequence = %+v", lay.words)
	}

	render(t, captionSpec([]string{text, "ok"}, 0, st))
}

// TestRightToLeftScriptsAreOutsideTheBundledFonts records the right-to-left
// result: the bidirectional algorithm and the visual reordering run (see
// TestRightToLeftRunsAreReordered), but none of the six families has Arabic or
// Hebrew letters, so every letter is .notdef and nothing joins. The text
// contract therefore does not promise right-to-left scripts until a font for
// them is bundled (BETA, user fonts, RF-053).
func TestRightToLeftScriptsAreOutsideTheBundledFonts(t *testing.T) {
	t.Parallel()

	st := plainStyle()

	for _, f := range families {
		face, err := Bundled().Lookup(f.name, f.min)
		if err != nil {
			t.Fatal(err)
		}

		for _, text := range []string{"שלום", "مرحبا"} {
			for _, g := range layoutText([]rune(text), nil, face, mustStyle(t, st), 1).glyphs {
				if g.gid != 0 {
					t.Errorf("%s has a glyph for %q: the documented scope is out of date", f.name, text)
				}
			}
		}
	}
}
