package image

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/plan"
)

// The lint codes of an image (SDD-021 section 8). The findings are those of core/lint; the rules are not in its
// catalogue, which is the video's.
const (
	CodeEmptySlot   = "L_IMAGE_EMPTY_SLOT"
	CodeTextOutside = "L_IMAGE_TEXT_OUTSIDE"
	CodeTextSmall   = "L_IMAGE_TEXT_SMALL"
	CodeLowContrast = "L_IMAGE_LOW_CONTRAST"
	CodeUpscaled    = "L_IMAGE_UPSCALED"
)

// MinContrast is the contrast ratio under which a text is hard to read (WCAG, large text).
const MinContrast = 3.0

// MaxUpscale is how many times larger than its source an image layer may be drawn before the lint says so.
const MaxUpscale = 2

// LintRule describes a rule for the surfaces.
type LintRule struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Blocking bool   `json:"blocking"`
}

// LintCatalog lists the rules.
func LintCatalog() []LintRule {
	return []LintRule{
		{CodeEmptySlot, "error", true},
		{CodeTextOutside, "warning", false},
		{CodeTextSmall, "warning", false},
		{CodeLowContrast, "warning", false},
		{CodeUpscaled, "warning", false},
	}
}

// Blocking reports whether a finding refuses an export.
func Blocking(code string) bool { return code == CodeEmptySlot }

// LintEnv is what the rules read besides the document: the text measurer, to know how tall a text is.
type LintEnv struct {
	Measure plan.Measurer
}

// entry is a layer that is drawn: the layer, its box on the canvas and its place in the stacking order.
type entry struct {
	l     *Layer
	box   box
	order int
}

func drawn(d *Document) []entry {
	var out []entry

	var visit func(ls []Layer, dx, dy int)

	visit = func(ls []Layer, dx, dy int) {
		for i := range ls {
			l := &ls[i]
			if !l.Visible {
				continue
			}

			if l.Kind == KindGroup {
				visit(l.Children, dx+l.X, dy+l.Y)

				continue
			}

			out = append(out, entry{l, box{l.X + dx, l.Y + dy, l.W, l.H}, len(out)})
		}
	}

	visit(d.Layers, 0, 0)

	return out
}

// Lint runs the rules on a document. Findings come errors first, then in the stacking order of their layers
// (top of a document's list last), then by code, so the same document always gives the same list.
func Lint(d *Document, _ LintEnv) []lint.Finding {
	type found struct {
		f     lint.Finding
		order int
	}

	var all []found

	add := func(order int, code, severity, layer, message, hint string) {
		all = append(
			all,
			found{lint.Finding{Code: code, Severity: severity, Item: layer, Message: message, Hint: hint}, order},
		)
	}

	r := &linter{d: d, es: drawn(d), floor: minText(d), add: add}

	Walk(d, func(l, _ *Layer, _ int) {
		if l.Slot != "" && empty(l) {
			add(-1, CodeEmptySlot, "error", l.ID, fmt.Sprintf("The slot %q has no content.", l.Slot),
				"Fill it with the template values, or give the layer a text or an image.")
		}
	})

	for _, e := range r.es {
		switch e.l.Kind {
		case KindText:
			r.text(e)
		case KindImage:
			r.image(e)
		}
	}

	slices.SortStableFunc(all, func(a, b found) int {
		if a.f.Severity != b.f.Severity {
			if a.f.Severity == "error" {
				return -1
			}

			return 1
		}

		if a.order != b.order {
			return a.order - b.order
		}

		if a.f.Code < b.f.Code {
			return -1
		} else if a.f.Code > b.f.Code {
			return 1
		}

		return 0
	})

	out := make([]lint.Finding, len(all))
	for i := range all {
		out[i] = all[i].f
	}

	return out
}

func trim(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// minText is the smallest text size the document accepts.
func minText(d *Document) int {
	if p, ok := PresetByName(d.Preset); ok {
		return p.MinText
	}

	return DefaultMinText
}

func textSize(d *Document, l *Layer) float64 {
	return textSpec(d, l, 1).Style.Size
}

// outside reports whether the box of a layer, turned by its rotation around its centre, is not entirely inside
// the canvas.
func outside(d *Document, e entry) bool {
	b := e.box
	hw, hh := float64(b.w)/2, float64(b.h)/2
	rad := float64(e.l.Rotation) * math.Pi / 180
	cos, sin := math.Abs(math.Cos(rad)), math.Abs(math.Sin(rad))
	ex, ey := hw*cos+hh*sin, hw*sin+hh*cos
	cx, cy := float64(b.x)+hw, float64(b.y)+hh

	const slack = 0.5 // the rounding of the rotated corners

	return cx-ex < -slack || cy-ey < -slack || cx+ex > float64(d.Width)+slack || cy+ey > float64(d.Height)+slack
}

// under is the colour under the centre of a text: the topmost fill or shape below it that contains the centre,
// else the background. ok is false when what is under it cannot be known from the document: an image, or a
// transparent background with nothing below.
func under(d *Document, es []entry, text entry) (r, g, b float64, ok bool) {
	cx, cy := float64(text.box.x)+float64(text.box.w)/2, float64(text.box.y)+float64(text.box.h)/2

	for _, e := range slices.Backward(es) {
		if e.order >= text.order {
			continue
		}

		if !contains(e, cx, cy) {
			continue
		}

		switch e.l.Kind {
		case KindImage:
			return 0, 0, 0, false
		case KindFill, KindShape:
			if r, g, b, ok := parseColour(e.l.Fill); ok {
				return r, g, b, true
			}
		}
	}

	if d.Background == Transparent {
		return 0, 0, 0, false
	}

	return parseColour(d.Background)
}

func contains(e entry, x, y float64) bool {
	b := e.box
	if e.l.Kind == KindShape && e.l.Shape == ShapeEllipse {
		rx, ry := float64(b.w)/2, float64(b.h)/2
		dx, dy := (x-float64(b.x)-rx)/rx, (y-float64(b.y)-ry)/ry

		return dx*dx+dy*dy <= 1
	}

	return x >= float64(b.x) && x <= float64(b.x+b.w) && y >= float64(b.y) && y <= float64(b.y+b.h)
}

func parseColour(s string) (r, g, b float64, ok bool) {
	if !ValidColour(s) {
		return 0, 0, 0, false
	}

	v, err := strconv.ParseUint(s[1:7], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}

	return float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff), true
}

func luminance(r, g, b float64) float64 {
	lin := func(c float64) float64 {
		c /= 255
		if c <= 0.03928 {
			return c / 12.92
		}

		return math.Pow((c+0.055)/1.055, 2.4)
	}

	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contrast is the WCAG ratio between the colour of a text and what is under it.
func contrast(d *Document, es []entry, text entry) (float64, bool) {
	br, bg, bb, ok := under(d, es, text)
	if !ok {
		return 0, false
	}

	colour := textSpec(d, text.l, 1).Style.Color

	tr, tg, tb, ok := parseColour(strings.ToUpper(colour))
	if !ok {
		return 0, false
	}

	l1, l2 := luminance(tr, tg, tb), luminance(br, bg, bb)
	if l1 < l2 {
		l1, l2 = l2, l1
	}

	return (l1 + 0.05) / (l2 + 0.05), true
}

// linter holds what the rules of one layer read.
type linter struct {
	d     *Document
	es    []entry
	floor int
	add   func(order int, code, severity, layer, message, hint string)
}

func (r *linter) text(e entry) {
	size := textSize(r.d, e.l)

	if outside(r.d, e) {
		r.add(e.order, CodeTextOutside, "warning", e.l.ID,
			fmt.Sprintf("The text layer %q is not entirely inside the canvas.", e.l.Name),
			"Move it with layer.transform or layer.align so that it sits inside the canvas.")
	}

	if size < float64(r.floor) {
		r.add(
			e.order,
			CodeTextSmall,
			"warning",
			e.l.ID,
			fmt.Sprintf(
				"The text of %q is %s px; the smallest for this image is %d px.",
				e.l.Name,
				trim(size),
				r.floor,
			),
			fmt.Sprintf("Raise the size to at least %d with layer.set_text.", r.floor),
		)
	}

	if ratio, ok := contrast(r.d, r.es, e); ok && ratio < MinContrast {
		r.add(
			e.order,
			CodeLowContrast,
			"warning",
			e.l.ID,
			fmt.Sprintf(
				"The text of %q has a contrast of %.1f against what is under it; 3 is the least.",
				e.l.Name,
				ratio,
			),
			"Change the text colour with layer.set_text, or the colour under it with layer.set_shape.",
		)
	}
}

func (r *linter) image(e entry) {
	src, ok := r.d.Assets[e.l.Asset]
	if !ok {
		return
	}

	c := cropOf(e.l, src)
	if e.l.W > MaxUpscale*c.W || e.l.H > MaxUpscale*c.H {
		r.add(
			e.order,
			CodeUpscaled,
			"warning",
			e.l.ID,
			fmt.Sprintf(
				"The image layer %q is drawn at %dx%d from a source of %dx%d.",
				e.l.Name,
				e.l.W,
				e.l.H,
				c.W,
				c.H,
			),
			"Use a larger image, or make the layer smaller with layer.transform.",
		)
	}
}
