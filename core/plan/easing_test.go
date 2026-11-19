package plan

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func bezierKeys(prop string, from, to float64, curve ...float64) timeline.Keyframes {
	return timeline.Keyframes{prop: {
		{T: 0, Value: from, Easing: timeline.EasingBezier, Bezier: curve},
		{T: 10, Value: to, Easing: "linear"},
	}}
}

// The expected values were computed with an independent floating-point bisection of 80 steps.
func TestEasingBezierKnownValues(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name  string
		curve []float64
		u     float64
		want  float64
	}{
		{"css ease at 0.5", []float64{0.25, 0.1, 0.25, 1}, 0.5, 0.802403388},
		{"css ease-in-out is symmetric", []float64{0.42, 0, 0.58, 1}, 0.5, 0.5},
		{"the diagonal is linear", []float64{0, 0, 1, 1}, 0.37, 0.37},
		{"ease-in is slow at the start", []float64{0.42, 0, 1, 1}, 0.25, 0.093464651},
		{"overshoot goes past 1", []float64{0.34, 1.56, 0.64, 1}, 0.5, 1.087400670},
		{"back pulls under 0", []float64{0.36, 0, 0.66, -0.56}, 0.3, -0.075776062},
	} {
		got := timeline.BezierEase(c.curve, c.u)
		if math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s: e(%v) = %v, want about %v", c.name, c.u, got, c.want)
		}
	}
}

func TestEasingBezierEndPointsAreExact(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(1, 31))

	for range 500 {
		b := randomCurve(rng, true)
		if got := timeline.BezierEase(b, 0); got != 0 {
			t.Fatalf("e(0) of %v = %v, want exactly 0", b, got)
		}

		if got := timeline.BezierEase(b, 1); got != 1 {
			t.Fatalf("e(1) of %v = %v, want exactly 1", b, got)
		}
	}
}

// A curve whose y values stay from 0 to 1 never goes back in value: time and progress both only move forward.
func TestEasingBezierIsMonotonicInTime(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(2, 31))

	for range 300 {
		b := randomCurve(rng, false)
		prev := -1.0

		for i := range 201 {
			e := timeline.BezierEase(b, float64(i)/200)
			if e < prev-1e-9 {
				t.Fatalf("curve %v: e(%v) = %v after %v, not monotonic", b, float64(i)/200, e, prev)
			}

			prev = e
		}
	}
}

func TestEasingBezierIsDeterministic(t *testing.T) {
	t.Parallel()

	b := []float64{0.34, 1.56, 0.64, 1}
	first := math.Float64bits(timeline.BezierEase(b, 0.4321))

	for range 100 {
		if got := math.Float64bits(timeline.BezierEase(b, 0.4321)); got != first {
			t.Fatalf("the same curve and u gave different bits: %x then %x", first, got)
		}
	}
}

func TestEasingBezierInvalidCurveIsLinear(t *testing.T) {
	t.Parallel()

	for _, b := range [][]float64{nil, {0.1, 0.2}, {1.5, 0, 0, 1}, {0, 5, 1, 1}, {math.NaN(), 0, 1, 1}} {
		if got := timeline.BezierEase(b, 0.3); got != 0.3 {
			t.Errorf("curve %v: e(0.3) = %v, want linear 0.3", b, got)
		}
	}
}

func TestEasingTheSampledValueFollowsTheCurve(t *testing.T) {
	t.Parallel()

	curve := []float64{0.42, 0, 0.58, 1}
	it := clip("itm_a", trkBottom, 0, 10)
	it.Keyframes = bezierKeys("scale", 1, 2, curve...)

	for r := range 11 {
		want := round3(1 + timeline.BezierEase(curve, float64(r)/10))
		if got := round3(property(&it, "scale", 1, float64(r))); got != want {
			t.Errorf("frame %d: scale %v, want %v", r, got, want)
		}
	}
}

// The plan hands every executor the same sampled value: the layer of a frame and of a window agree.
func TestEasingReachesTheLayersAndTheWindow(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_a", trkBottom, 0, 10)
	it.Keyframes = bezierKeys("scale", 1, 2, 0.34, 1.56, 0.64, 1)
	put(d, it)

	w := Window(d, 0, 10).Layers[0]

	for f := range 10 {
		want := Frame(d, f).Layers[0].Scale
		if got := w.Scale[f]; got != want {
			t.Errorf("frame %d: window scale %v, frame scale %v", f, got, want)
		}
	}

	if got := Frame(d, 5).Layers[0].Scale; got <= 1.5 {
		t.Errorf("scale %v at the middle of an overshoot curve, want above the linear 1.5", got)
	}
}

func TestEasingOvershootStaysInsideThePropertyRange(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_a", trkBottom, 0, 10)
	it.Keyframes = bezierKeys("opacity", 0, 1, 0.34, 1.56, 0.64, 1)
	it.Mask = &timeline.Mask{Shape: "rect", X: 0, Y: 0, Width: 0.5, Height: 0.5}
	it.Keyframes["mask_width"] = []timeline.Keyframe{
		{T: 0, Value: 0.2, Easing: timeline.EasingBezier, Bezier: []float64{0.34, 1.56, 0.64, 1}},
		{T: 10, Value: 0.9, Easing: "linear"},
	}
	put(d, it)

	for f := range 10 {
		l := Frame(d, f).Layers[0]
		if l.Opacity < 0 || l.Opacity > 1 {
			t.Errorf("frame %d: opacity %v is out of range", f, l.Opacity)
		}

		if m := l.Mask; m.Width < 0.001 || m.X+m.Width > 1.0001 {
			t.Errorf("frame %d: mask %+v leaves the item", f, m)
		}
	}
}

func randomCurve(rng *rand.Rand, anyY bool) []float64 {
	y := func() float64 {
		if anyY {
			return timeline.BezierMinY + rng.Float64()*(timeline.BezierMaxY-timeline.BezierMinY)
		}

		return rng.Float64()
	}

	return []float64{rng.Float64(), y(), rng.Float64(), y()}
}

// The crop and the mask are sampled by the plan like any property: each frame carries the value on the
// curve, so no executor evaluates one (SDD-013 section 2.3).
func TestEasingCropAndMaskFollowTheCurve(t *testing.T) {
	t.Parallel()

	curve := []float64{0.42, 0, 0.58, 1}
	d := newDoc()
	it := clip("itm_a", trkBottom, 0, 10)
	it.Mask = &timeline.Mask{Shape: "rect", X: 0, Y: 0, Width: 0.2, Height: 1}
	it.Keyframes = timeline.Keyframes{
		"crop_left": {
			{T: 0, Value: 0, Easing: timeline.EasingBezier, Bezier: curve},
			{T: 10, Value: 0.4, Easing: "linear"},
		},
		"mask_width": {
			{T: 0, Value: 0.2, Easing: timeline.EasingBezier, Bezier: curve},
			{T: 10, Value: 0.8, Easing: "linear"},
		},
	}
	put(d, it)

	sw := float64(d.Assets[it.Asset].Width)

	for f := range 10 {
		e := timeline.BezierEase(curve, float64(f)/10)
		l := Frame(d, f).Layers[0]

		if want := round3(0.2 + 0.6*e); l.Mask.Width != want {
			t.Errorf("frame %d: mask width %v, want %v", f, l.Mask.Width, want)
		}

		if want := round3(0.4 * e * sw); l.Crop.X != want {
			t.Errorf("frame %d: crop x %v, want %v", f, l.Crop.X, want)
		}
	}
}
