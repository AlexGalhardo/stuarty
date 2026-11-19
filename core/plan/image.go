package plan

import "github.com/AlexGalhardo/stuarty/core/timeline"

// The two functions below let core/image (SDD-021) build the bitmaps of its text and shape layers with the
// very defaults and oversampling of a video text item, instead of a copy of them.

// ResolveTextStyle is the effective style of a text that has the given style over the defaults of a text item
// (SDD-001 section 3.6) at a frame width, with max_width left at the share of the width a text item has: the
// caller sets MaxWidth when the text has a box of its own.
func ResolveTextStyle(width int, s *timeline.TextStyle) ResolvedStyle {
	doc := &timeline.Document{}
	st := baseStyle(width, defaultTextSize, textMaxWidthFraction)

	applyStyle(doc, &st, s)

	if s == nil || s.FontWeight == nil {
		st.FontWeight = timeline.DefaultFontWeight(doc, st.FontFamily)
	}

	return st
}

// PixelScale is the oversampling of a bitmap of an item drawn at itemScale in a frame of output scale (0 means
// 1), as in SDD-004 section 5.3.
func PixelScale(itemScale, output float64) float64 {
	it := &timeline.Item{Scale: itemScale}

	return pixelScale(it, output)
}

// FitToMaxWidth is the factor that brings a bitmap of natural width nw back to the style's max_width when one
// word is wider than it (SDD-004 section 5.1 item 2).
func FitToMaxWidth(st ResolvedStyle, nw float64) float64 { return fitToMaxWidth(st, nw) }
