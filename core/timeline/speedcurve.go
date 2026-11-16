package timeline

import (
	"math"
	"math/big"
)

// Speed curve limits (SDD-001 §3.5, SDD-004 §3.11).
const (
	MinCurvePoints = 2
	MaxCurvePoints = 8
)

// SpeedPoint is one point of a speed curve: T is the position inside the item as a
// fraction of its duration, with at most 3 decimals, and Speed is the speed there.
type SpeedPoint struct {
	T     float64  `json:"t"     min:"0" max:"1"`
	Speed Rational `json:"speed"`
}

// CurvePermille returns the points as integers: t in thousandths of the duration and the
// speed in thousandths. It reports false when a value is not a multiple of 0.001, which
// is what makes every product of SDD-004 §3.11 an integer.
func CurvePermille(pts []SpeedPoint) (t, s []int, ok bool) {
	for _, p := range pts {
		tp := int(math.Round(p.T * 1000))
		if math.Abs(p.T*1000-float64(tp)) > 1e-6 || p.Speed.Num < 1 || p.Speed.Den < 1 || 1000%p.Speed.Den != 0 {
			return nil, nil, false
		}
		t, s = append(t, tp), append(s, p.Speed.Num*(1000/p.Speed.Den))
	}
	return t, s, true
}

// CurveError describes why a curve is not valid, or is empty when it is.
func CurveError(pts []SpeedPoint) string {
	if len(pts) < MinCurvePoints || len(pts) > MaxCurvePoints {
		return "a speed curve has 2 to 8 points"
	}
	t, s, ok := CurvePermille(pts)
	if !ok {
		return "t and speed must be multiples of 0.001"
	}
	if t[0] != 0 || t[len(t)-1] != 1000 {
		return "the first point is at t 0 and the last at t 1"
	}
	for i := range pts {
		if i > 0 && t[i] <= t[i-1] {
			return "t must increase from one point to the next"
		}
		if s[i] < 250 || s[i] > 4000 {
			return "each speed is from 0.25x to 4x"
		}
	}
	return ""
}

// CurveMean is the mean speed of the curve over the item, in lowest terms: the sum over
// the segments of dt x (s_a + s_b) / 2 with every value in thousandths (SDD-004 §3.11).
// The curve must be valid.
func CurveMean(pts []SpeedPoint) Rational {
	t, s, _ := CurvePermille(pts)
	var sum int64
	for i := 1; i < len(t); i++ {
		sum += int64(t[i]-t[i-1]) * int64(s[i-1]+s[i])
	}
	g, rest := sum, int64(2_000_000)
	for rest != 0 {
		g, rest = rest, g%rest
	}
	return Rational{Num: int(sum / g), Den: int(2_000_000 / g)}
}

// CurveOffset is the number of source frames that precede output frame k of an item of d
// frames, floor(d x integral from 0 to k/d of the speed), by the exact rational formula of
// SDD-004 §3.11. k runs from 0 to d; the curve must be valid.
func CurveOffset(pts []SpeedPoint, d, k int) int {
	t, s, _ := CurvePermille(pts)
	x := int64(k) * 1000
	acc := new(big.Rat)
	for i := 1; i < len(t); i++ {
		lo, hi := int64(t[i-1])*int64(d), int64(t[i])*int64(d)
		if x <= lo {
			break
		}
		v := min(x, hi) - lo
		dt, sa, ds := int64(t[i]-t[i-1]), int64(s[i-1]), int64(s[i]-s[i-1])
		num := new(big.Int).Mul(big.NewInt(2*dt*int64(d)*sa), big.NewInt(v))
		num.Add(num, new(big.Int).Mul(big.NewInt(ds), new(big.Int).Mul(big.NewInt(v), big.NewInt(v))))
		acc.Add(acc, new(big.Rat).SetFrac(num, big.NewInt(2*dt*int64(d)*1_000_000)))
	}
	return int(new(big.Int).Quo(acc.Num(), acc.Denom()).Int64())
}

// CurveSourceFrames is the number of source frames an item of d frames reads under the
// curve, ceil(d x mean speed).
func CurveSourceFrames(pts []SpeedPoint, d int) int {
	m := CurveMean(pts)
	return (d*m.Num + m.Den - 1) / m.Den
}

// SourceOffset is the source frame, counted from source_in, that an item shows at its
// local frame k (SDD-004 §3.3 and §3.11): constant speed floor(k x num / den), a curve
// CurveOffset.
func (it *Item) SourceOffset(k int) int {
	if len(it.SpeedCurve) > 0 {
		return CurveOffset(it.SpeedCurve, it.Duration, k)
	}
	return k * it.Speed.Num / it.Speed.Den
}
