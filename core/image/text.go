package image

import (
	"math"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// textSpec is the bitmap the rasterizer draws for a text layer: the style of a text item of a video over the
// same defaults, wrapped at the width of the box (SDD-021 section 5).
func textSpec(d *Document, l *Layer, pixelScale float64) plan.BitmapSpec {
	var ts *timeline.TextStyle
	if l.Style != nil {
		ts = &l.Style.TextStyle
	}

	style := plan.ResolveTextStyle(d.Width, ts)
	style.MaxWidth = float64(l.W)

	return plan.BitmapSpec{
		Kind: "text", Text: l.Text, ActiveWord: -1, Style: style, PixelScale: pixelScale,
	}
}

// measured is the size of the block of a text layer, with the shrink that keeps a long word inside the box.
type measured struct {
	w, h float64
	fit  float64
}

func measureText(env *Env, d *Document, l *Layer) measured {
	spec := textSpec(d, l, 1)

	var nw, nh float64
	if env != nil && env.Measure != nil {
		nw, nh = env.Measure(spec)
	} else {
		lines := strings.Count(l.Text, "\n") + 1
		nh = math.Ceil(float64(lines) * spec.Style.Size * spec.Style.LineHeight)
	}

	fit := 1.0
	if nw > 0 {
		fit = plan.FitToMaxWidth(spec.Style, nw)
	}

	return measured{w: nw * fit, h: nh * fit, fit: fit}
}

// remeasure sets the height of a text layer to the height of its block.
func remeasure(env *Env, d *Document, l *Layer) {
	m := measureText(env, d, l)
	l.H = min(max(int(math.Ceil(m.h)), 1), MaxBoxSide)
}
