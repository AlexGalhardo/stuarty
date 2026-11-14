package raster

import "math"

// The bitmaps must be byte-equal on every architecture (NFR-010). Two things in
// the standard library break that: math functions written in Go (Sin, Cos,
// Atan2, Hypot, Acos on arm64) are compiled with fused multiply-add, which
// rounds differently from the unfused code of amd64, and some of them have an
// assembly version on one architecture only (Hypot on amd64). The functions
// below use only the operations IEEE 754 rounds exactly (+, -, *, /, sqrt,
// floor) and put an explicit conversion around every product, which the Go
// specification says forbids fusing it. They are as accurate as the geometry
// needs (about 1e-15) and give the same bits everywhere.

// hypot is the length of (x, y).
func hypot(x, y float64) float64 { return math.Sqrt(float64(x*x) + float64(y*y)) }

const (
	twoPi  = 2 * math.Pi
	halfPi = math.Pi / 2
)

// sincos returns the sine and cosine of x radians.
func sincos(x float64) (sin, cos float64) {
	// Reduce to [-pi, pi]; the angles of this package stay within a few turns.
	x -= mul(twoPi, math.Floor(x/twoPi+0.5))

	x2 := mul(x, x)
	sin, cos = x, 1.0
	ts, tc := x, 1.0

	for k := 1; k <= 24; k++ {
		tc = -mul(tc, x2) / float64((2*k-1)*(2*k))
		ts = -mul(ts, x2) / float64((2*k)*(2*k+1))
		cos += tc
		sin += ts
	}

	return sin, cos
}

// atan returns the arc tangent of t, in [-pi/2, pi/2].
func atan(t float64) float64 {
	if t < 0 {
		return -atan(-t)
	}

	if t > 1 {
		return halfPi - atan(1/t)
	}

	// Halve the angle twice, atan(t) = 2 atan(t / (1 + sqrt(1 + t^2))), so that
	// the series converges fast.
	for range 2 {
		t /= 1 + math.Sqrt(1+mul(t, t))
	}

	t2 := mul(t, t)
	sum, term := t, t

	for k := 1; k <= 20; k++ {
		term = -mul(term, t2)
		sum += term / float64(2*k+1)
	}

	return 4 * sum
}

// atan2 returns the angle of the vector (x, y), in (-pi, pi].
func atan2(y, x float64) float64 {
	switch {
	case x > 0:
		return atan(y / x)
	case x < 0 && y >= 0:
		return atan(y/x) + math.Pi
	case x < 0:
		return atan(y/x) - math.Pi
	case y > 0:
		return halfPi
	case y < 0:
		return -halfPi
	}

	return 0
}
