package rasterize

import (
	"errors"
	"fmt"
	"slices"
	"unicode"

	"golang.org/x/image/font/sfnt"
)

// exempt is not a glyph the font is expected to have: control and format characters (a joiner, a
// direction mark), whose absence draws nothing, and the variation selectors that follow an emoji.
func exempt(r rune) bool {
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Variation_Selector)
}

// Missing returns the runes of text that the family has no glyph for, in order of first appearance, each
// once, so that the text is drawn with the `.notdef` glyph for them (SDD-004 section 5.1 item 9). Control and
// format characters are not reported. A family or weight the set does not have, and a user font whose file
// cannot be read, are reported as an error: the lint stays silent for them, the render says why.
func (s *FontSet) Missing(family string, weight int, text string) ([]rune, error) {
	has, err := s.coverage(family, weight)
	if err != nil {
		return nil, err
	}

	var out []rune

	for _, r := range text {
		if exempt(r) || has(r) || slices.Contains(out, r) {
			continue
		}

		out = append(out, r)
	}

	return out, nil
}

func (s *FontSet) coverage(family string, weight int) (func(rune) bool, error) {
	if s != nil {
		for _, f := range s.user[family] {
			if f.weight != weight {
				continue
			}

			if s.src == nil {
				return nil, fmt.Errorf("%w: %s", ErrFontNotStored, family)
			}

			data, err := s.src(f.hash)
			if err != nil {
				return nil, err
			}

			return sfntCoverage(data)
		}
	}

	ranges, ok := bundledCoverage[family]
	if !ok {
		return nil, fmt.Errorf("the font family %q is not in the set", family)
	}

	return func(r rune) bool {
		i, _ := slices.BinarySearchFunc(ranges, r, func(e [2]rune, r rune) int {
			if r > e[1] {
				return -1
			}

			if r < e[0] {
				return 1
			}

			return 0
		})

		return i < len(ranges) && ranges[i][0] <= r && r <= ranges[i][1]
	}, nil
}

func sfntCoverage(data []byte) (func(rune) bool, error) {
	f, err := sfnt.Parse(data)
	if err != nil {
		return nil, err
	}

	var b sfnt.Buffer

	return func(r rune) bool {
		g, err := f.GlyphIndex(&b, r)

		return err == nil && g != 0
	}, nil
}

// coverageRanges lists the runes a font file has a glyph for as sorted, closed ranges. It is how the table in
// coverage_gen.go is made (missing_gen_test.go).
func coverageRanges(data []byte) ([][2]rune, error) {
	has, err := sfntCoverage(data)
	if err != nil {
		return nil, err
	}

	var out [][2]rune

	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF || !has(r) {
			continue
		}

		if n := len(out); n > 0 && out[n-1][1] == r-1 {
			out[n-1][1] = r
		} else {
			out = append(out, [2]rune{r, r})
		}
	}

	if len(out) == 0 {
		return nil, errors.New("the font has no glyph")
	}

	return out, nil
}
