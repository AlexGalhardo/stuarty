package raster

import (
	"math"
	"slices"

	"github.com/go-text/typesetting/bidi"
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// placedGlyph is a glyph with its origin on the baseline, in block pixels.
type placedGlyph struct {
	gid  font.GID
	x, y float64
	// text is the index of the glyph's cluster in the whole text, in runes.
	text int
}

// layout is the result of shaping and line breaking: glyphs and the metrics of
// the block they fill. The block starts at (0, 0).
type layout struct {
	glyphs        []placedGlyph
	width, height float64
	// words holds the box of each word, in block pixels; a word with no glyph
	// has an empty box. Empty for a text.
	words []plan.Rect
	// scale is pixels per font unit: size / units per em.
	scale float64
}

// textLine is one wrapped line: its runs in logical order, its width in pixels
// and the offset of its paragraph in the whole text.
type textLine struct {
	runs  []shaping.Output
	width float64
	base  int
}

// layoutText shapes the runes of the text, breaks them into lines no wider than
// the maximum width and places the glyphs (SDD-004 section 5.1). spans are the
// rune ranges of the words of a caption; their boxes come back in layout.words.
func layoutText(runes []rune, spans [][2]int, face *font.Face, st style, scale float64) layout {
	size := mul(st.size, scale)
	lineH := mul(size, st.lineHeight)
	asc, desc := verticalExtents(face, size)
	baseline := (lineH-(asc-desc))/2 + asc

	shaper := &shaping.HarfbuzzShaper{}

	var lines []textLine

	for _, p := range splitParagraphs(runes) {
		lines = append(lines, wrapParagraph(shaper, runes[p[0]:p[1]], p[0], face, size, st.maxWidth*scale)...)
	}

	lay := layout{scale: size / float64(face.Upem()), words: make([]plan.Rect, len(spans))}

	for _, l := range lines {
		lay.width = math.Max(lay.width, l.width)
	}

	lay.height = mul(lineH, float64(len(lines)))

	boxes := make([]wordBox, len(spans))

	for i, l := range lines {
		x := alignOffset(st.align, lay.width-l.width)
		top := mul(float64(i), lineH)
		lay.place(l, x, top, top+baseline, lineH, spans, boxes)
	}

	for i, b := range boxes {
		lay.words[i] = b.rect()
	}

	return lay
}

// verticalExtents returns the ascent (positive) and descent (negative) of the
// face at the given size in pixels.
func verticalExtents(face *font.Face, size float64) (ascent, descent float64) {
	ext, ok := face.FontHExtents()
	k := size / float64(face.Upem())

	if !ok {
		return mul(size, 0.8), -mul(size, 0.2)
	}

	return mul(float64(ext.Ascender), k), mul(float64(ext.Descender), k)
}

func alignOffset(align string, free float64) float64 {
	switch align {
	case AlignLeft:
		return 0
	case AlignRight:
		return free
	default:
		return free / 2
	}
}

// splitParagraphs returns the [start, end) rune ranges between forced breaks.
// A text that ends with a break has a last, empty paragraph, so the break
// counts as a line.
func splitParagraphs(runes []rune) [][2]int {
	var out [][2]int

	start := 0

	for i, r := range runes {
		if r == '\n' {
			out = append(out, [2]int{start, i})
			start = i + 1
		}
	}

	return append(out, [2]int{start, len(runes)})
}

// wrapParagraph shapes one paragraph and breaks it into lines. base is the
// index of the paragraph's first rune in the whole text.
func wrapParagraph(
	shaper *shaping.HarfbuzzShaper, par []rune, base int, face *font.Face, size, maxWidth float64,
) []textLine {
	if len(par) == 0 {
		return []textLine{{base: base}}
	}

	var bp bidi.Paragraph

	runs := bp.Segment(par, bidi.Neutral)
	dir := di.DirectionLTR

	if runs.NumRuns() > 0 && !runs.Run(0).IsLeftToRight() {
		dir = di.DirectionRTL
	}

	var outs []shaping.Output

	for i := range runs.NumRuns() {
		r := runs.Run(i)
		rdir := di.DirectionLTR

		if !r.IsLeftToRight() {
			rdir = di.DirectionRTL
		}

		for _, piece := range splitByScript(par, r.Start, r.End) {
			outs = append(outs, shaper.Shape(shaping.Input{
				Text:      par,
				RunStart:  piece.start,
				RunEnd:    piece.end,
				Direction: rdir,
				Face:      face,
				Size:      fixed.Int26_6(math.Round(size * 64)),
				Script:    piece.script,
				Language:  "en",
			}))
		}
	}

	var wrapper shaping.LineWrapper

	cfg := shaping.WrapConfig{Direction: dir, BreakPolicy: shaping.Never}
	wrapped, _ := wrapper.WrapParagraphF(
		cfg,
		fixed.Int26_6(math.Round(maxWidth*64)),
		par,
		shaping.NewSliceIterator(outs),
	)

	lines := make([]textLine, 0, len(wrapped))

	for _, l := range wrapped {
		tl := textLine{runs: slices.Clone(l), base: base}
		for _, o := range l {
			tl.width += fix(o.Advance)
		}

		lines = append(lines, tl)
	}

	return lines
}

func fix(v fixed.Int26_6) float64 { return float64(v) / 64 }

type scriptPiece struct {
	start, end int
	script     language.Script
}

// splitByScript cuts [start, end) where the script changes. Common and
// Inherited characters (spaces, digits, punctuation, combining marks) join the
// script before them, or the one after them at the start of the run.
func splitByScript(text []rune, start, end int) []scriptPiece {
	var out []scriptPiece

	cur := scriptPiece{start: start, script: language.Common}

	for i := start; i < end; i++ {
		s := language.LookupScript(text[i])
		if !s.Strong() {
			continue
		}

		if !cur.script.Strong() {
			cur.script = s

			continue
		}

		if s != cur.script {
			cur.end = i
			out = append(out, cur)
			cur = scriptPiece{start: i, script: s}
		}
	}

	cur.end = end

	return append(out, cur)
}

// wordBox accumulates the extent of the glyphs of one word.
type wordBox struct {
	x0, x1, y0, y1 float64
	set            bool
}

func (b *wordBox) add(x0, x1, y0, y1 float64) {
	if !b.set {
		*b = wordBox{x0, x1, y0, y1, true}

		return
	}

	b.x0, b.x1 = math.Min(b.x0, x0), math.Max(b.x1, x1)
	b.y0, b.y1 = math.Min(b.y0, y0), math.Max(b.y1, y1)
}

func (b wordBox) rect() plan.Rect {
	if !b.set {
		return plan.Rect{}
	}

	return plan.Rect{X: b.x0, Y: b.y0, W: b.x1 - b.x0, H: b.y1 - b.y0}
}

// place puts the glyphs of a line on the block. x is where the line starts,
// top and baseline are y coordinates of the line box top and of the baseline.
func (lay *layout) place(l textLine, x, top, baseline, lineH float64, spans [][2]int, boxes []wordBox) {
	runs := slices.Clone(l.runs)
	slices.SortStableFunc(runs, func(a, b shaping.Output) int { return int(a.VisualIndex - b.VisualIndex) })

	pen := x

	for _, run := range runs {
		for _, g := range run.Glyphs {
			idx := l.base + g.TextIndex()
			adv := fix(g.Advance)

			lay.glyphs = append(lay.glyphs, placedGlyph{
				gid:  g.GlyphID,
				x:    pen + fix(g.XOffset),
				y:    baseline - fix(g.YOffset),
				text: idx,
			})

			if k := wordAt(spans, idx); k >= 0 {
				boxes[k].add(pen, pen+adv, top, top+lineH)
			}

			pen += adv
		}
	}
}

func wordAt(spans [][2]int, idx int) int {
	for k, s := range spans {
		if idx >= s[0] && idx < s[1] {
			return k
		}
	}

	return -1
}
