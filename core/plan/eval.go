package plan

import (
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// keyframeValue evaluates keyframes at r, the frame inside the item (SDD-004
// §3.1). r may be fractional, which the audio envelope needs. The keyframes
// are taken in order of t.
func keyframeValue(kfs []timeline.Keyframe, r float64) float64 {
	k := kfs
	if !slices.IsSortedFunc(k, func(a, b timeline.Keyframe) int { return a.T - b.T }) {
		k = slices.Clone(kfs)
		slices.SortStableFunc(k, func(a, b timeline.Keyframe) int { return a.T - b.T })
	}

	return timeline.Interpolate(k, r)
}

// property is the value of an animated property at r: its keyframes, or the
// static value.
func property(it *timeline.Item, name string, static, r float64) float64 {
	if kfs := it.Keyframes[name]; len(kfs) > 0 {
		return keyframeValue(kfs, r)
	}

	return static
}

// animated holds the animated values of a visual item at one frame.
type animated struct {
	x, y, scale, rotation, opacity float64
}

// evaluate gives the position, scale, rotation and opacity of the item at
// frame r inside it, with the transitions applied (SDD-004 §3.1, §3.2). cx and
// cy are the static defaults when the item has no x or y.
func evaluate(it *timeline.Item, r int, width, height, cx, cy float64) animated {
	fr := float64(r)
	a := animated{
		x:        property(it, "x", orDefault(it.X, cx), fr),
		y:        property(it, "y", orDefault(it.Y, cy), fr),
		scale:    property(it, "scale", it.Scale, fr),
		rotation: property(it, "rotation", it.Rotation, fr),
		opacity:  property(it, "opacity", it.Opacity, fr),
	}

	if tr := it.TransitionIn; tr != nil && r < tr.Duration {
		a.transition(tr.Kind, float64(r+1)/float64(tr.Duration+1), width, height)
	}

	if tr := it.TransitionOut; tr != nil && r >= it.Duration-tr.Duration {
		a.transition(tr.Kind, float64(it.Duration-r)/float64(tr.Duration+1), width, height)
	}

	return a
}

func orDefault(p *float64, def float64) float64 {
	if p == nil {
		return def
	}

	return *p
}

// transition applies one transition at progress p (SDD-004 §3.2).
func (a *animated) transition(kind string, p, width, height float64) {
	s := 1 - (1-p)*(1-p)*(1-p)

	switch kind {
	case "fade":
		a.opacity *= p
	case "slide_left":
		a.x -= (1 - s) * width
	case "slide_right":
		a.x += (1 - s) * width
	case "slide_up":
		a.y -= (1 - s) * height
	case "slide_down":
		a.y += (1 - s) * height
	}
}

// sourceFrame is the frame of the normalized media to show at frame r of a
// clip (SDD-004 §3.3), in integer arithmetic; an animated image loops through its
// frames by their delays.
func sourceFrame(it *timeline.Item, a *timeline.Asset, fps timeline.Rational, r int) int {
	if a.IsAnimated() {
		return AnimImageSourceFrame(a.FrameDelays, fps, r)
	}

	if a.Kind == timeline.AssetImage || a.FrameRate.Num == 0 || it.Speed.Den == 0 || fps.Num == 0 {
		return 0
	}

	pos := int64(it.SourceIn) + int64(r)*int64(it.Speed.Num)/int64(it.Speed.Den)
	sf := pos * int64(fps.Den) * int64(a.FrameRate.Num) / (int64(fps.Num) * int64(a.FrameRate.Den))

	if a.FrameCount > 0 {
		sf = min(sf, int64(a.FrameCount)-1)
	}

	return int(max(sf, 0))
}

// colorMatrix is the 3×4 matrix S×C of SDD-004 §3.5, row-major with the
// offset in the fourth column, or nil when brightness, contrast and
// saturation are neutral.
func colorMatrix(it *timeline.Item) *[12]float64 {
	b, k, s := it.Brightness, it.Contrast, it.Saturation
	if b == 0 && k == 1 && s == 1 {
		return nil
	}

	const lr, lg, lb = 0.2126, 0.7152, 0.0722

	// C: v' = k·v + (0.5 - 0.5k + b). S: v'' = l + (v' - l)·s with l the luma of v'.
	off := 0.5 - 0.5*k + b
	rows := [3][3]float64{
		{lr + (1-lr)*s, lg * (1 - s), lb * (1 - s)},
		{lr * (1 - s), lg + (1-lg)*s, lb * (1 - s)},
		{lr * (1 - s), lg * (1 - s), lb + (1-lb)*s},
	}

	var m [12]float64

	for i, row := range rows {
		for j, v := range row {
			m[i*4+j] = round3(v * k)
		}

		// The offset passes through S as well: l(off,off,off) = off.
		m[i*4+3] = round3(off)
	}

	return &m
}
