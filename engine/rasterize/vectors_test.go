package rasterize

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// vectorStyle is the style of a vector with the colours parsed, in the order the file holds it.
type vectorStyle struct {
	fontFamily string
	fontWeight int
	size       float64
	color      colour

	strokeColor colour
	strokeWidth float64

	shadowColor colour
	shadowDX    float64
	shadowDY    float64
	shadowBlur  float64

	background        *colour
	backgroundPadding float64
	backgroundRadius  float64

	align      string
	lineHeight float64
	maxWidth   float64
	uppercase  bool

	highlight           bool
	highlightColor      colour
	highlightBackground *colour
}

func mustColour(t *testing.T, s string) colour {
	t.Helper()

	c, err := parseColour(s)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func optColour(t *testing.T, s string) *colour {
	t.Helper()

	if s == "" {
		return nil
	}

	c := mustColour(t, s)

	return &c
}

func mustStyle(t *testing.T, p plan.ResolvedStyle) vectorStyle {
	t.Helper()

	st := vectorStyle{
		fontFamily: p.FontFamily, fontWeight: p.FontWeight, size: p.Size,
		color: mustColour(t, p.Color), strokeColor: mustColour(t, p.StrokeColor), strokeWidth: p.StrokeWidth,
		shadowColor: mustColour(t, p.ShadowColor), shadowDX: p.ShadowDX, shadowDY: p.ShadowDY, shadowBlur: p.ShadowBlur,
		background: optColour(t, p.BackgroundColor), backgroundPadding: p.BackgroundPadding,
		backgroundRadius: p.BackgroundRadius, align: p.Align, lineHeight: p.LineHeight, maxWidth: p.MaxWidth,
		uppercase: p.Uppercase, highlight: p.Highlight != "none",
		highlightBackground: optColour(t, p.HighlightBackground),
	}

	if p.HighlightColor != "" {
		st.highlightColor = mustColour(t, p.HighlightColor)
	}

	return st
}

// The Rust rasterizer (renderer/stuarty-renderer-text, SDD-013 section 3.11) is the reference. These
// vectors are the specs and the bitmap it draws for each, written by `cargo test` in renderer/
// (VUKEM_WRITE_RASTER_VECTORS=1, a reviewed change of output, rule .agents/skills/renderer-parity).
// This package encodes the same specs, draws them through the renderer module under wazero and must
// get the same bytes: the file is the contract between the native crate and the module.
const rasterVectorsPath = "../../renderer/testdata/raster-vectors.bin"

type vectorCase struct {
	name string
	spec plan.BitmapSpec
}

func shapeVector(name string, scale float64, sh plan.ShapeSpec) vectorCase {
	return vectorCase{name, plan.BitmapSpec{Kind: KindShape, PixelScale: scale, Shape: &sh}}
}

func rasterVectorCases() []vectorCase {
	var out []vectorCase

	golden := goldenCases()
	for i := range golden {
		out = append(out, vectorCase(golden[i]))
	}

	white, black := C(255, 255, 255, 255), C(0, 0, 0, 255)

	out = append(out,
		shapeVector("shape-rect-no-stroke", 1, plan.ShapeSpec{
			Shape: ShapeRect, Width: 100, Height: 40, Fill: C(10, 20, 30, 255),
		}),
		shapeVector("shape-rect-scale-2-fractional", 2, plan.ShapeSpec{
			Shape: ShapeRect, Width: 33.3, Height: 17.7, Fill: C(200, 10, 90, 180),
			StrokeColor: white, StrokeWidth: 3.5,
		}),
		shapeVector("shape-rounded-rect-radius-over-half", 1.5, plan.ShapeSpec{
			Shape: ShapeRoundedRect, Width: 80, Height: 30, Radius: 100, Fill: C(30, 144, 255, 230),
			StrokeColor: black, StrokeWidth: 5,
		}),
		shapeVector("shape-rounded-rect-radius-zero", 1, plan.ShapeSpec{
			Shape: ShapeRoundedRect, Width: 64, Height: 64, Radius: 0, Fill: white,
			StrokeColor: C(255, 0, 0, 128), StrokeWidth: 12,
		}),
		shapeVector("shape-ellipse-circle-small", 1, plan.ShapeSpec{
			Shape: ShapeEllipse, Width: 12, Height: 12, Fill: C(255, 255, 0, 255),
			StrokeColor: black, StrokeWidth: 1,
		}),
		shapeVector("shape-ellipse-wide-scale-3", 3, plan.ShapeSpec{
			Shape: ShapeEllipse, Width: 90, Height: 25, Fill: C(0, 128, 128, 255),
			StrokeColor: C(255, 255, 255, 90), StrokeWidth: 7,
		}),
		shapeVector("shape-rounded-rect-tiny", 1, plan.ShapeSpec{
			Shape: ShapeRoundedRect, Width: 5, Height: 3, Radius: 1, Fill: white,
		}),
	)

	return append(out, textVectorCases()...)
}

type vectorWriter struct{ bytes.Buffer }

func (w *vectorWriter) u32(v int)       { _ = binary.Write(w, binary.LittleEndian, uint32(v)) }
func (w *vectorWriter) f64(v float64)   { _ = binary.Write(w, binary.LittleEndian, math.Float64bits(v)) }
func (w *vectorWriter) str(s string)    { w.u32(len(s)); w.WriteString(s) }
func (w *vectorWriter) colour(c colour) { w.Write(c[:]) }

// pixels writes the bytes as runs: the length of a run of zero bytes, the length of the run of other bytes
// that follows and those bytes, until the whole bitmap is written. A text bitmap is mostly transparent, so
// the file stays small.
func (w *vectorWriter) pixels(pix []byte) {
	for i := 0; i < len(pix); {
		zeros := 0
		for i+zeros < len(pix) && pix[i+zeros] == 0 {
			zeros++
		}

		lit := 0
		for i+zeros+lit < len(pix) && (pix[i+zeros+lit] != 0 || (i+zeros+lit+1 < len(pix) && pix[i+zeros+lit+1] != 0)) {
			lit++
		}

		w.u32(zeros)
		w.u32(lit)
		w.Write(pix[i+zeros : i+zeros+lit])

		i += zeros + lit
	}
}

func (w *vectorWriter) flag(b bool) {
	if b {
		w.WriteByte(1)
	} else {
		w.WriteByte(0)
	}
}

func (w *vectorWriter) optColour(c *colour) {
	w.flag(c != nil)

	if c != nil {
		w.colour(*c)
	}
}

// encodeRasterVectors is the file: "VRAS", the version, the number of cases, then per case its name,
// the kind (0 shape, 1 text, 2 caption), the spec and the expected bitmap.
//
// A shape spec is its kind index, width, height, radius, stroke width, fill, stroke colour and pixel
// scale. A text or caption spec is the style (family, weight, size, colour, stroke colour and width,
// shadow colour, offset and blur, optional box colour, padding and radius, alignment, line height, maximum
// width, upper-case flag, highlight flag and colour, optional highlight box colour), the text (a text) or
// the words, the breaks and the active word plus one (a caption) and the pixel scale. The bitmap is the
// width, height, natural width and height, centre offset, the boxes of the words (a count, then x, y,
// width and height of each) and the pixels as runs (see pixels).
func encodeRasterVectors(t *testing.T) []byte {
	t.Helper()

	cases := rasterVectorCases()

	var w vectorWriter

	w.WriteString("VRAS")
	w.u32(2)
	w.u32(len(cases))

	for i := range cases {
		c := &cases[i]

		w.str(c.name)

		switch c.spec.Kind {
		case KindShape:
			encodeShapeSpec(t, &w, c.spec)
		default:
			encodeTextSpec(t, &w, c.spec)
		}

		bm := render(t, c.spec)
		w.u32(bm.Width)
		w.u32(bm.Height)
		w.f64(bm.NaturalW)
		w.f64(bm.NaturalH)
		w.f64(bm.CenterDX)
		w.f64(bm.CenterDY)

		if c.spec.Kind != KindShape {
			w.u32(len(bm.WordBoxes))

			for _, b := range bm.WordBoxes {
				w.f64(b.X)
				w.f64(b.Y)
				w.f64(b.W)
				w.f64(b.H)
			}
		}

		w.pixels(bm.Pix)
	}

	return w.Bytes()
}

func encodeShapeSpec(t *testing.T, w *vectorWriter, spec plan.BitmapSpec) {
	t.Helper()

	w.u32(0)

	sh := spec.Shape
	kind := map[string]int{ShapeRect: 0, ShapeRoundedRect: 1, ShapeEllipse: 2}[sh.Shape]

	var stroke colour
	if sh.StrokeWidth > 0 {
		stroke = mustColour(t, sh.StrokeColor)
	}

	w.u32(kind)
	w.f64(sh.Width)
	w.f64(sh.Height)
	w.f64(sh.Radius)
	w.f64(sh.StrokeWidth)
	w.colour(mustColour(t, sh.Fill))
	w.colour(stroke)
	w.f64(spec.PixelScale)
}

func encodeTextSpec(t *testing.T, w *vectorWriter, spec plan.BitmapSpec) {
	t.Helper()

	st := mustStyle(t, spec.Style)

	if spec.Kind == KindCaption {
		w.u32(2)
	} else {
		w.u32(1)
	}

	w.str(st.fontFamily)
	w.u32(st.fontWeight)
	w.f64(st.size)
	w.colour(st.color)
	w.colour(st.strokeColor)
	w.f64(st.strokeWidth)
	w.colour(st.shadowColor)
	w.f64(st.shadowDX)
	w.f64(st.shadowDY)
	w.f64(st.shadowBlur)
	w.optColour(st.background)
	w.f64(st.backgroundPadding)
	w.f64(st.backgroundRadius)
	w.str(st.align)
	w.f64(st.lineHeight)
	w.f64(st.maxWidth)
	w.flag(st.uppercase)
	w.flag(st.highlight)
	w.colour(st.highlightColor)
	w.optColour(st.highlightBackground)

	if spec.Kind == KindCaption {
		w.u32(len(spec.Words))

		for _, word := range spec.Words {
			w.str(word)
		}

		w.u32(len(spec.Breaks))

		for _, b := range spec.Breaks {
			w.u32(b)
		}

		w.u32(spec.ActiveWord + 1)
	} else {
		w.str(spec.Text)
	}

	w.f64(spec.PixelScale)
}

// TestRasterVectorsAreCurrent keeps renderer/testdata/raster-vectors.bin equal to what the renderer
// module draws through this package.
func TestRasterVectorsAreCurrent(t *testing.T) {
	got := encodeRasterVectors(t)

	want, err := os.ReadFile(rasterVectorsPath)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, want) {
		t.Fatal("renderer/testdata/raster-vectors.bin is out of date: regenerate it with " +
			"VUKEM_WRITE_RASTER_VECTORS=1 and review the change")
	}
}
