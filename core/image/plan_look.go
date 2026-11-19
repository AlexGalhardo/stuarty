package image

import (
	"github.com/AlexGalhardo/stuarty/core/plan"
)

// look sets the fields of a plan layer that come from the look of a layer (SDD-021 section 5, SDD-013 section
// 3.11). A layer without a look gets none of them.
func (p *planner) look(l *Layer, out *plan.Layer) {
	if a := l.Adjust; a != nil {
		out.ColorMatrix = plan.ColorMatrix(
			float64(a.Brightness)/100, 1+float64(a.Contrast)/100, 1+float64(a.Saturation)/100,
			float64(a.Temperature)/100, float64(a.Tint)/100, float64(a.Hue),
		)

		if t, ok := ToneTable(a); ok {
			out.Tone = &t
		}
	}

	for i := range l.Filters {
		out.Filters = append(out.Filters, filterSpec(&l.Filters[i]))
	}

	if m := l.Mask; m != nil {
		if m.Kind == MaskLayer {
			out.Clip = &plan.ClipSpec{Source: m.Source, Invert: m.Invert}
		} else {
			out.Mask = &plan.MaskSpec{
				Shape: m.Kind, X: float64(m.X) / 1000, Y: float64(m.Y) / 1000, Width: float64(m.W) / 1000,
				Height: float64(m.H) / 1000, Feather: m.Feather, Invert: m.Invert, Radius: m.Radius,
			}
		}
	}

	out.Blend = l.Blend
}

// percent is a whole percent as the fraction of the plan, rounded to three decimals.
func percent(v int) float64 { return round3(float64(v) / 100) }

// filterSpec is the plan filter of a filter of a layer: the percentages become fractions.
func filterSpec(f *Filter) plan.FilterSpec {
	switch f.Kind {
	case FilterGrayscale, FilterSepia, FilterInvert, FilterWarm, FilterCool:
		return plan.FilterSpec{Kind: "matrix", Matrix: plan.FilterMatrix(f.Kind, float64(f.Value("amount"))/100)}
	case FilterBlur:
		return plan.FilterSpec{Kind: "blur", Sigma: float64(f.Value("radius"))}
	case FilterSharpen:
		return plan.FilterSpec{Kind: "sharpen", Amount: percent(f.Value("amount")), Radius: float64(f.Value("radius"))}
	case FilterVignette:
		return plan.FilterSpec{
			Kind:     "vignette",
			Amount:   percent(f.Value("amount")),
			Softness: percent(f.Value("softness")),
		}
	case FilterGrain:
		return plan.FilterSpec{
			Kind: "grain", Amount: percent(f.Value("amount")), Size: f.Value("size"), Seed: f.Value("seed"),
		}
	case FilterShadow:
		return plan.FilterSpec{
			Kind: "shadow", Color: f.Color, Opacity: percent(f.Value("opacity")), DX: float64(f.Value("dx")),
			DY: float64(f.Value("dy")), Blur: float64(f.Value("blur")),
		}
	case FilterGlow:
		return plan.FilterSpec{
			Kind: "outer_glow", Color: f.Color, Radius: float64(f.Value("radius")), Amount: percent(f.Value("amount")),
		}
	default: // FilterStroke
		return plan.FilterSpec{Kind: "stroke", Color: f.Color, Width: float64(f.Value("width"))}
	}
}
