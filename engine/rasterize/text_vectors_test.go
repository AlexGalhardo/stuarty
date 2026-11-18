package rasterize

import "github.com/AlexGalhardo/vukem-video/core/plan"

// textVectorCases are the cases of the text rasterizer that the goldens do not reach: the corners of the
// layout, the highlight and the effects. renderer/vukem-renderer-text must draw each byte for byte.
//
// Each case is drawn twice: with Inter, a variable font, and with Anton, a static one. The two differ in
// how a port can match go-text: a variable font's points come out of float32 arithmetic and its kerning
// out of GPOS tables with variation deltas, which go-text and harfrust compute differently.
func textVectorCases() []vectorCase {
	var out []vectorCase

	for _, f := range []struct{ prefix, family string }{{"", "Inter"}, {"anton-", "Anton"}} {
		cases := textVectorCasesFor(f.family)
		for i := range cases {
			out = append(out, vectorCase{f.prefix + cases[i].name, cases[i].spec})
		}
	}

	return out
}

func textVectorCasesFor(family string) []vectorCase {
	box := C(20, 24, 32, 220)
	pink := C(255, 45, 110, 255)

	style := func(mod func(*plan.ResolvedStyle)) plan.ResolvedStyle {
		return goldenStyle(func(s *plan.ResolvedStyle) {
			if family != "Inter" {
				s.FontFamily, s.FontWeight = family, 400
			}

			if mod != nil {
				mod(s)
			}

			if family != "Inter" {
				s.FontFamily, s.FontWeight = family, 400
			}
		})
	}

	return []vectorCase{
		{"text-empty-lines", textSpec("one\n\nthree\n", style(func(s *plan.ResolvedStyle) { s.Size = 40 }))},
		{"text-trailing-space", textSpec("trailing space ", style(func(s *plan.ResolvedStyle) { s.Size = 40 }))},
		{"text-unbreakable-word-over-width", textSpec(
			"short Supercalifragilisticexpialidocious end",
			style(func(s *plan.ResolvedStyle) { s.Size, s.MaxWidth = 40, 300 }),
		)},
		{"text-align-left-multiline", textSpec(
			"left aligned text that wraps over several lines of a narrow box",
			style(func(s *plan.ResolvedStyle) { s.Size, s.MaxWidth, s.Align = 32, 260, AlignLeft }),
		)},
		{"text-align-center-multiline", textSpec(
			"centred text that wraps over several lines of a narrow box",
			style(func(s *plan.ResolvedStyle) { s.Size, s.MaxWidth = 32, 260 }),
		)},
		{"text-fractional-size-scale-1.5", func() plan.BitmapSpec {
			sp := textSpec("Fractional 33.3 size", style(func(s *plan.ResolvedStyle) { s.Size = 33.3 }))
			sp.PixelScale = 1.5

			return sp
		}()},
		{"text-shadow-blur-offset", textSpec("Shadow", style(func(s *plan.ResolvedStyle) {
			s.Size, s.ShadowDX, s.ShadowDY, s.ShadowBlur = 64, -10, 14, 5
			s.ShadowColor = C(0, 0, 0, 200)
		}))},
		{"text-box-padding", textSpec("Boxed", style(func(s *plan.ResolvedStyle) {
			s.Size, s.BackgroundColor, s.BackgroundPadding, s.BackgroundRadius = 56, box, 24, 20
		}))},
		{"text-stroke-only-thick", textSpec("Thick", style(func(s *plan.ResolvedStyle) {
			s.Size, s.StrokeWidth, s.StrokeColor = 72, 14, C(200, 0, 0, 255)
		}))},
		{"text-uppercase-accents", textSpec("straße ünï", style(func(s *plan.ResolvedStyle) {
			s.Size, s.Uppercase = 48, true
		}))},
		{"text-ligature-and-kerning", textSpec("Affinity AVATAR To fi fl", style(func(s *plan.ResolvedStyle) {
			s.Size, s.FontWeight = 56, 400
		}))},
		{"caption-break-and-highlight-colour", captionSpec(
			[]string{"one", "two", "three", "four"}, 1,
			style(func(s *plan.ResolvedStyle) { s.Size, s.HighlightColor = 56, C(0, 255, 120, 255) }),
		)},
		{"caption-highlight-none", func() plan.BitmapSpec {
			sp := captionSpec(neverWords, 2, style(func(s *plan.ResolvedStyle) { s.Size, s.Highlight = 56, "none" }))

			return sp
		}()},
		{"caption-forced-break", func() plan.BitmapSpec {
			sp := captionSpec(neverWords, 3, style(func(s *plan.ResolvedStyle) {
				s.Size, s.HighlightBackground = 56, pink
			}))
			sp.Breaks = []int{2}

			return sp
		}()},
		{"caption-wrapped-narrow", captionSpec(
			[]string{"Captions", "wrap", "when", "the", "line", "is", "too", "long"}, 5,
			style(func(s *plan.ResolvedStyle) { s.Size, s.MaxWidth = 48, 300 }),
		)},
	}
}
