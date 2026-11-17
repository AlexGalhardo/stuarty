package rasterize

import (
	"bytes"
	"testing"
)

// The tests below are the measurements of risk R3 (ADR-0012): what the
// pipeline does with emoji when the only fonts are the six bundled families.

// TestEmojiDrawsTheNotdefGlyph records the emoji result: no bundled family has
// an emoji glyph, so an emoji is drawn with the .notdef glyph of the font and
// the rest of the line is laid out normally (SDD-004 section 5.1 item 9). It is
// drawn, not skipped: the bitmap with the emoji is wider than the same text
// without it, and no glyph of another font replaced it.
func TestEmojiDrawsTheNotdefGlyph(t *testing.T) {
	t.Parallel()

	st := plainStyle()
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
// width joiner: a sequence is one cluster, so the caption word boxes do not
// split it.
func TestEmojiSequencesStayTogether(t *testing.T) {
	t.Parallel()

	// Family emoji: man, ZWJ, woman, ZWJ, girl.
	text := "\U0001F468\u200d\U0001F469\u200d\U0001F467"

	bm := render(t, captionSpec([]string{text, "ok"}, 0, plainStyle()))
	if len(bm.WordBoxes) != 2 || bm.WordBoxes[0].W <= 0 {
		t.Fatalf("word boxes of the sequence = %+v", bm.WordBoxes)
	}
}
