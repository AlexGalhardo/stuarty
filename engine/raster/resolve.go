package raster

import (
	"fmt"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// parseColour reads "#RRGGBB" or "#RRGGBBAA", the forms a document holds once
// the plan has resolved colour references (SDD-001 section 3.1).
func parseColour(s string) (Colour, error) {
	if (len(s) != 7 && len(s) != 9) || s[0] != '#' {
		return Colour{}, fmt.Errorf("%w: colour %q is not #RRGGBB or #RRGGBBAA", ErrInvalidSpec, s)
	}

	var ch [4]byte

	ch[3] = 255

	for i := range (len(s) - 1) / 2 {
		hi, ok1 := hexNibble(s[1+2*i])
		lo, ok2 := hexNibble(s[2+2*i])

		if !ok1 || !ok2 {
			return Colour{}, fmt.Errorf("%w: colour %q", ErrInvalidSpec, s)
		}

		ch[i] = hi<<4 | lo
	}

	return Colour{R: ch[0], G: ch[1], B: ch[2], A: ch[3]}, nil
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

// optionalColour is parseColour, with "" meaning none.
func optionalColour(s string) (*Colour, error) {
	if s == "" {
		return nil, nil //nolint:nilnil // nil is the value "none"
	}

	c, err := parseColour(s)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

// resolveStyle parses the colours of a plan style.
func resolveStyle(p plan.ResolvedStyle) (style, error) {
	var (
		s = style{
			fontFamily: p.FontFamily, fontWeight: p.FontWeight, size: p.Size,
			strokeWidth: p.StrokeWidth, shadowDX: p.ShadowDX, shadowDY: p.ShadowDY, shadowBlur: p.ShadowBlur,
			backgroundPadding: p.BackgroundPadding, backgroundRadius: p.BackgroundRadius,
			align: p.Align, lineHeight: p.LineHeight, maxWidth: p.MaxWidth, uppercase: p.Uppercase,
			highlight: p.Highlight != "none",
		}
		err error
	)

	for _, c := range []struct {
		dst *Colour
		src string
	}{
		{&s.color, p.Color}, {&s.strokeColor, p.StrokeColor}, {&s.shadowColor, p.ShadowColor},
	} {
		if *c.dst, err = parseColour(c.src); err != nil {
			return style{}, err
		}
	}

	if s.background, err = optionalColour(p.BackgroundColor); err != nil {
		return style{}, err
	}

	// An unset highlight colour draws nothing different, which is what an
	// alpha of 0 means.
	if p.HighlightColor != "" {
		if s.highlightColor, err = parseColour(p.HighlightColor); err != nil {
			return style{}, err
		}
	}

	if s.highlightBackground, err = optionalColour(p.HighlightBackground); err != nil {
		return style{}, err
	}

	return s, nil
}

// resolveShape parses the colours of a plan shape.
func resolveShape(p plan.ShapeSpec) (shape, error) {
	s := shape{kind: p.Shape, width: p.Width, height: p.Height, radius: p.Radius, strokeWidth: p.StrokeWidth}

	var err error

	if s.fill, err = parseColour(p.Fill); err != nil {
		return shape{}, err
	}

	// The stroke colour is unused, and may be absent, when there is no stroke.
	if p.StrokeWidth > 0 {
		if s.strokeColor, err = parseColour(p.StrokeColor); err != nil {
			return shape{}, err
		}
	}

	return s, nil
}
