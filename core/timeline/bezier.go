package timeline

import "math"

// The cubic Bezier easing of epic 31 (SDD-004 §3.1). The curve goes from (0, 0) to (1, 1)
// through two control points; the arithmetic is fixed-point (Q30 in 64-bit integers) with a
// fixed number of bisection steps, so every platform, and every executor that is handed the
// result, gets the same number.

const (
	bezierShift = 30
	bezierOne   = int64(1) << bezierShift
)

// BezierMinY and BezierMaxY bound the y of a control point, so a curve may pull back or overshoot
// by a limited amount (SDD-001 §3.8).
const (
	BezierMinY = -2.0
	BezierMaxY = 3.0
)

// BezierValid reports whether b is a usable curve: four numbers, x1 and x2 from 0 to 1, y1 and
// y2 from BezierMinY to BezierMaxY.
func BezierValid(b []float64) bool {
	if len(b) != 4 {
		return false
	}

	for i, v := range b {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}

		lo, hi := 0.0, 1.0
		if i%2 == 1 {
			lo, hi = BezierMinY, BezierMaxY
		}

		if v < lo || v > hi {
			return false
		}
	}

	return true
}

func toQ(v float64) int64 { return int64(math.Round(v * float64(bezierOne))) }

// mulQ is a×b for two Q30 numbers.
func mulQ(a, b int64) int64 { return (a * b) >> bezierShift }

// bezierPoly is 3(1-s)²s·p1 + 3(1-s)s²·p2 + s³ for the Q30 numbers s, p1 and p2.
func bezierPoly(s, p1, p2 int64) int64 {
	inv := bezierOne - s
	s2, inv2 := mulQ(s, s), mulQ(inv, inv)
	a := 3 * mulQ(mulQ(inv2, s), p1)
	b := 3 * mulQ(mulQ(inv, s2), p2)

	return a + b + mulQ(s2, s)
}

// BezierEase is e(u) of the "bezier" easing for the curve b ([x1, y1, x2, y2]) and u from 0 to 1.
// e(0) is exactly 0 and e(1) exactly 1. A curve that is not valid is linear.
func BezierEase(b []float64, u float64) float64 {
	if !BezierValid(b) {
		return u
	}

	switch {
	case u <= 0:
		return 0
	case u >= 1:
		return 1
	}

	x1, y1, x2, y2 := toQ(b[0]), toQ(b[1]), toQ(b[2]), toQ(b[3])
	ux := toQ(u)

	// x(s) does not decrease with s, so 30 halvings find s to the last Q30 digit.
	lo, hi := int64(0), bezierOne
	for range bezierShift {
		mid := (lo + hi) >> 1
		if bezierPoly(mid, x1, x2) < ux {
			lo = mid
		} else {
			hi = mid
		}
	}

	return float64(bezierPoly((lo+hi)>>1, y1, y2)) / float64(bezierOne)
}

// EaseOf is e(u) of a keyframe: its named easing, or its curve when the easing is "bezier".
func (k Keyframe) EaseOf(u float64) float64 {
	if k.Easing == EasingBezier {
		return BezierEase(k.Bezier, u)
	}

	return Ease(k.Easing, u)
}

// MinScale and MaxScale bound the scale of an item (SDD-001 §3.5).
const (
	MinScale = 0.001
	MaxScale = 20.0
)

// KeyframeRange is the range a sampled value of an animated property is kept in, and whether it
// has one: a curve that overshoots must not hand an executor an opacity of 1.1 (SDD-004 §3.1).
// x, y and rotation have none.
func KeyframeRange(prop string) (lo, hi float64, ok bool) {
	switch prop {
	case "scale":
		return MinScale, MaxScale, true
	case "opacity":
		return 0, 1, true
	case "volume":
		return 0, 4, true
	}

	for _, rng := range []func(string) (float64, float64, bool){ColourRange, MaskRange, CropRange} {
		if lo, hi, ok := rng(prop); ok {
			return lo, hi, true
		}
	}

	return 0, 0, false
}
