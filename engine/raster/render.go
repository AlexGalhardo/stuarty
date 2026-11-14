package raster

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// ErrInvalidSpec is returned for a spec the rasterizer cannot draw.
var ErrInvalidSpec = errors.New("raster: invalid spec")

// MaxPixels bounds the size of a bitmap: a 4x oversampled 1080 by 1920 frame
// is 33 million pixels, which is also the largest bitmap a text can need.
const MaxPixels = 4096 * 8192

// Render draws the bitmap of the spec. It is deterministic: the same spec gives
// the same bytes on every platform.
func Render(spec BitmapSpec, fonts *FontSet) (Bitmap, error) {
	if err := validate(spec); err != nil {
		return Bitmap{}, err
	}

	switch spec.Kind {
	case KindShape:
		return renderShape(spec)
	default:
		return renderText(spec, fonts)
	}
}

func validate(spec BitmapSpec) error {
	if !(spec.PixelScale > 0 && spec.PixelScale <= 8) {
		return fmt.Errorf("%w: pixel scale %v", ErrInvalidSpec, spec.PixelScale)
	}

	switch spec.Kind {
	case KindShape:
		if spec.Shape == nil || !(spec.Shape.Width > 0 && spec.Shape.Height > 0) {
			return fmt.Errorf("%w: shape needs a positive width and height", ErrInvalidSpec)
		}
	case KindText, KindCaption:
		st := spec.Style
		if !(st.Size > 0 && st.LineHeight > 0 && st.MaxWidth > 0) {
			return fmt.Errorf("%w: size, line height and max width must be positive", ErrInvalidSpec)
		}

		if spec.Kind == KindCaption && len(spec.Words) == 0 {
			return fmt.Errorf("%w: a caption has no words", ErrInvalidSpec)
		}

		if spec.Kind == KindCaption && spec.ActiveWord >= len(spec.Words) {
			return fmt.Errorf("%w: active word %d of %d", ErrInvalidSpec, spec.ActiveWord, len(spec.Words))
		}
	default:
		return fmt.Errorf("%w: kind %q", ErrInvalidSpec, spec.Kind)
	}

	return nil
}

// textRunes returns the runes to shape and the rune range of each word of a
// caption. Upper-casing maps one rune to one rune, so ranges survive it.
func textRunes(spec BitmapSpec) ([]rune, [][2]int) {
	var runes []rune

	var spans [][2]int

	if spec.Kind == KindCaption {
		breaks := map[int]bool{}
		for _, b := range spec.Breaks {
			breaks[b] = true
		}

		for i, w := range spec.Words {
			switch {
			case i == 0:
			case breaks[i]:
				runes = append(runes, '\n')
			default:
				runes = append(runes, ' ')
			}

			start := len(runes)
			runes = append(runes, []rune(w)...)
			spans = append(spans, [2]int{start, len(runes)})
		}
	} else {
		runes = []rune(strings.ReplaceAll(spec.Text, "\r\n", "\n"))
	}

	if spec.Style.Uppercase {
		for i, r := range runes {
			runes[i] = unicodeUpper(r)
		}
	}

	return runes, spans
}

func renderText(spec BitmapSpec, fonts *FontSet) (Bitmap, error) {
	st := spec.Style

	face, err := fonts.Lookup(st.FontFamily, st.FontWeight)
	if err != nil {
		return Bitmap{}, err
	}

	runes, spans := textRunes(spec)
	scale := spec.PixelScale
	lay := layoutText(runes, spans, face, st, scale)

	active := -1
	if spec.Kind == KindCaption && st.HighlightColor.A > 0 {
		active = spec.ActiveWord
	}

	return drawText(spec, face, lay, spans, active)
}

// outlines returns the closed polylines of the glyph outlines, in block pixels.
func outlines(face *font.Face, glyphs []placedGlyph, k float64) []polyline {
	var out []polyline

	for _, g := range glyphs {
		o, ok := face.GlyphDataOutline(g.gid)
		if !ok {
			continue
		}

		out = append(out, glyphPolylines(o, g.x, g.y, k)...)
	}

	return out
}

// glyphPolylines converts an outline in font units (y up) into polylines at the
// glyph origin (ox, oy) in pixels (y down).
func glyphPolylines(o font.GlyphOutline, ox, oy, k float64) []polyline {
	var out []polyline

	var cur []point

	at := func(p font.SegmentPoint) point { return point{ox + mul(float64(p.X), k), oy - mul(float64(p.Y), k)} }

	flush := func() {
		if len(cur) > 1 {
			out = append(out, polyline{pts: cur, closed: true})
		}

		cur = nil
	}

	for _, s := range o.Segments {
		switch s.Op {
		case ot.SegmentOpMoveTo:
			flush()

			cur = []point{at(s.Args[0])}
		case ot.SegmentOpLineTo:
			cur = append(cur, at(s.Args[0]))
		case ot.SegmentOpQuadTo:
			cur = appendQuad(cur, cur[len(cur)-1], at(s.Args[0]), at(s.Args[1]))
		case ot.SegmentOpCubeTo:
			cur = appendCube(cur, cur[len(cur)-1], at(s.Args[0]), at(s.Args[1]), at(s.Args[2]))
		}
	}

	flush()

	return out
}

func polygons(lines []polyline) [][]point {
	out := make([][]point, len(lines))
	for i, l := range lines {
		out[i] = l.pts
	}

	return out
}

func unicodeUpper(r rune) rune { return unicode.ToUpper(r) }
