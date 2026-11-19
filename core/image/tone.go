package image

import (
	"math"
	"sort"
)

// Tone is the 256-entry table of the exposure, the levels and the curve of an adjustment (SDD-013 section
// 3.11.1): the renderer applies it to every colour channel and has no formula of its own. It is built from
// three tables composed, each entry rounded to an integer in 0 to 255.

// ToneTable returns the table of an adjustment, or false when exposure, levels and curve are all neutral.
func ToneTable(a *Adjust) ([256]uint8, bool) {
	var t [256]uint8

	if a == nil || (a.Exposure == 0 && a.Levels == nil && len(a.Curve) == 0) {
		return t, false
	}

	for v := range t {
		x := v
		x = exposureStep(x, a.Exposure)
		x = levelsStep(x, a.Levels)
		x = curveStep(x, a.Curve)
		t[v] = uint8(x & 0xFF)
	}

	return t, true
}

func clamp255(v float64) int {
	return int(math.Min(math.Max(math.Round(v), 0), 255))
}

func exposureStep(v, e int) int {
	if e == 0 {
		return v
	}

	return clamp255(float64(v) * math.Pow(2, float64(e)/100))
}

func levelsStep(v int, lv *Levels) int {
	if lv == nil {
		return v
	}

	x := (float64(v) - float64(lv.Black)) / float64(lv.White-lv.Black)
	x = math.Min(math.Max(x, 0), 1)

	return clamp255(255 * math.Pow(x, 100/float64(lv.Gamma)))
}

// curveStep is the straight line through the points of the curve, with (0, 0) and (255, 255) as the ends when
// the points do not give them; the delta from the lower point is rounded half away from zero.
func curveStep(v int, curve []CurvePoint) int {
	if len(curve) == 0 {
		return v
	}

	pts := append([]CurvePoint(nil), curve...)
	sort.Slice(pts, func(i, j int) bool { return pts[i].X < pts[j].X })

	if pts[0].X > 0 {
		pts = append([]CurvePoint{{0, 0}}, pts...)
	}

	if pts[len(pts)-1].X < 255 {
		pts = append(pts, CurvePoint{255, 255})
	}

	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		if v > b.X {
			continue
		}

		return a.Y + roundDivAway((v-a.X)*(b.Y-a.Y), b.X-a.X)
	}

	return pts[len(pts)-1].Y
}

// roundDivAway is num / den rounded to nearest, halves away from zero, for den > 0.
func roundDivAway(num, den int) int {
	if num < 0 {
		return -((-num*2 + den) / (2 * den))
	}

	return (num*2 + den) / (2 * den)
}
