package rasterize

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// colour is a straight RGBA colour.
type colour [4]byte

// parseColour reads "#RRGGBB" or "#RRGGBBAA", the forms a document holds once
// the plan has resolved colour references (SDD-001 section 3.1).
func parseColour(s string) (colour, error) {
	if (len(s) != 7 && len(s) != 9) || s[0] != '#' {
		return colour{}, fmt.Errorf("%w: colour %q is not #RRGGBB or #RRGGBBAA", ErrInvalidSpec, s)
	}

	ch := colour{0, 0, 0, 255}

	for i := range (len(s) - 1) / 2 {
		hi, ok1 := hexNibble(s[1+2*i])
		lo, ok2 := hexNibble(s[2+2*i])

		if !ok1 || !ok2 {
			return colour{}, fmt.Errorf("%w: colour %q", ErrInvalidSpec, s)
		}

		ch[i] = hi<<4 | lo
	}

	return ch, nil
}

func hexNibble(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}

	return 0, false
}

// writer builds a request of the module: little-endian numbers, strings with a 32-bit length.
type writer struct{ buf bytes.Buffer }

func (w *writer) Bytes() []byte { return w.buf.Bytes() }

func (w *writer) u8(v int) { w.buf.WriteByte(byte(v)) } //nolint:gosec // G115: a small enumeration

func (w *writer) u32(v int) {
	w.buf.Write(binary.LittleEndian.AppendUint32(nil, uint32(v))) //nolint:gosec // G115: bounded by the callers
}

func (w *writer) f64(v float64) {
	w.buf.Write(binary.LittleEndian.AppendUint64(nil, math.Float64bits(v)))
}

func (w *writer) str(s string) {
	w.u32(len(s))
	w.buf.WriteString(s)
}

func (w *writer) col(c colour) { w.buf.Write(c[:]) }

func (w *writer) flag(b bool) {
	if b {
		w.u8(1)
	} else {
		w.u8(0)
	}
}

func (w *writer) opt(s string) error {
	if s == "" {
		w.u8(0)

		return nil
	}

	c, err := parseColour(s)
	if err != nil {
		return err
	}

	w.u8(1)
	w.col(c)

	return nil
}

func (w *writer) colourOf(s string) error {
	c, err := parseColour(s)
	if err != nil {
		return err
	}

	w.col(c)

	return nil
}

const (
	modeRender  = 0
	modeMeasure = 1
)

// request encodes a spec for the module: the mode, the kind, the pixel scale, then a shape, or the
// document fonts, the style and the text or the words.
func request(spec plan.BitmapSpec, fonts *FontSet, mode int) ([]byte, error) {
	if err := validate(spec); err != nil {
		return nil, err
	}

	var w writer

	w.u8(mode)

	if spec.Kind == KindShape {
		w.u8(0)
		w.f64(spec.PixelScale)

		if err := encodeShape(&w, *spec.Shape); err != nil {
			return nil, err
		}

		return w.Bytes(), nil
	}

	if spec.Kind == KindCaption {
		w.u8(2)
	} else {
		w.u8(1)
	}

	w.f64(spec.PixelScale)
	encodeFonts(&w, fonts)

	if err := encodeStyle(&w, spec.Style); err != nil {
		return nil, err
	}

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

	return w.Bytes(), nil
}

func validate(spec plan.BitmapSpec) error {
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

		if spec.Kind == KindCaption && (spec.ActiveWord >= len(spec.Words) || spec.ActiveWord < -1) {
			return fmt.Errorf("%w: active word %d of %d", ErrInvalidSpec, spec.ActiveWord, len(spec.Words))
		}
	default:
		return fmt.Errorf("%w: kind %q", ErrInvalidSpec, spec.Kind)
	}

	return nil
}

func encodeShape(w *writer, p plan.ShapeSpec) error {
	kind, ok := map[string]int{ShapeRect: 0, ShapeRoundedRect: 1, ShapeEllipse: 2}[p.Shape]
	if !ok {
		return fmt.Errorf("%w: shape %q", ErrInvalidSpec, p.Shape)
	}

	w.u8(kind)
	w.f64(p.Width)
	w.f64(p.Height)
	w.f64(p.Radius)
	w.f64(p.StrokeWidth)

	if err := w.colourOf(p.Fill); err != nil {
		return err
	}

	// The stroke colour is unused, and may be absent, when there is no stroke.
	if p.StrokeWidth > 0 {
		return w.colourOf(p.StrokeColor)
	}

	w.col(colour{})

	return nil
}

func encodeFonts(w *writer, fonts *FontSet) {
	if fonts == nil || len(fonts.user) == 0 {
		w.u32(0)

		return
	}

	names := make([]string, 0, len(fonts.user))
	for name := range fonts.user {
		names = append(names, name)
	}

	slices.Sort(names)
	w.u32(len(names))

	for _, name := range names {
		w.str(name)
		w.u32(len(fonts.user[name]))

		for _, f := range fonts.user[name] {
			w.u32(f.weight)
			w.str(f.hash)
		}
	}
}

func encodeStyle(w *writer, p plan.ResolvedStyle) error {
	w.str(p.FontFamily)
	w.u32(p.FontWeight)
	w.f64(p.Size)

	if err := w.colourOf(p.Color); err != nil {
		return err
	}

	if err := w.colourOf(p.StrokeColor); err != nil {
		return err
	}

	w.f64(p.StrokeWidth)

	if err := w.colourOf(p.ShadowColor); err != nil {
		return err
	}

	w.f64(p.ShadowDX)
	w.f64(p.ShadowDY)
	w.f64(p.ShadowBlur)

	if err := w.opt(p.BackgroundColor); err != nil {
		return err
	}

	w.f64(p.BackgroundPadding)
	w.f64(p.BackgroundRadius)
	w.u8(map[string]int{AlignLeft: 0, AlignCenter: 1, AlignRight: 2}[p.Align])
	w.f64(p.LineHeight)
	w.f64(p.MaxWidth)
	w.flag(p.Uppercase)
	w.flag(p.Highlight != "none")

	// An unset highlight colour draws nothing different, which is what an alpha of 0 means.
	if p.HighlightColor != "" {
		if err := w.colourOf(p.HighlightColor); err != nil {
			return err
		}
	} else {
		w.col(colour{})
	}

	return w.opt(p.HighlightBackground)
}

// reader decodes a result of the module.
type reader struct {
	b   []byte
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || n < 0 || n > len(r.b) {
		r.err = errors.New("rasterize: the module's result is truncated")

		return nil
	}

	out := r.b[:n]
	r.b = r.b[n:]

	return out
}

func (r *reader) u8() byte {
	b := r.take(1)
	if len(b) < 1 {
		return 0
	}

	return b[0]
}

func (r *reader) u32() int {
	b := r.take(4)
	if len(b) < 4 {
		return 0
	}

	return int(binary.LittleEndian.Uint32(b))
}

func (r *reader) f64() float64 {
	b := r.take(8)
	if len(b) < 8 {
		return 0
	}

	return math.Float64frombits(binary.LittleEndian.Uint64(b))
}

func (r *reader) str() string { return string(r.take(r.u32())) }

// decodeBitmap reads the result of a successful request.
func decodeBitmap(data []byte) (Bitmap, error) {
	r := reader{b: data}

	var bm Bitmap

	bm.Width, bm.Height = r.u32(), r.u32()
	bm.NaturalW, bm.NaturalH, bm.CenterDX, bm.CenterDY = r.f64(), r.f64(), r.f64(), r.f64()

	if n := r.u32(); n > 0 {
		bm.WordBoxes = make([]plan.Rect, 0, min(n, len(r.b)/32))

		for range n {
			bm.WordBoxes = append(bm.WordBoxes, plan.Rect{X: r.f64(), Y: r.f64(), W: r.f64(), H: r.f64()})
		}
	}

	bm.Pix = r.take(bm.Width * bm.Height * 4)
	if r.err == nil && len(r.b) != 0 {
		r.err = fmt.Errorf("rasterize: the module's result has %d extra bytes", len(r.b))
	}

	if bm.Width*bm.Height == 0 {
		bm.Pix = nil
	}

	return bm, r.err
}
