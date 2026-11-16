package timeline

import "testing"

func pt(t float64, n, d int) SpeedPoint { return SpeedPoint{T: t, Speed: Rational{Num: n, Den: d}} }

func TestCurveMeanAndOffsets(t *testing.T) {
	// 1x -> 3x -> 1x is a mean of 2x; 100 frames read 200 source frames.
	c := []SpeedPoint{pt(0, 1, 1), pt(0.5, 3, 1), pt(1, 1, 1)}
	if m := CurveMean(c); m != (Rational{Num: 2, Den: 1}) {
		t.Fatalf("mean %v", m)
	}
	if got := CurveOffset(c, 100, 100); got != 200 {
		t.Errorf("offset at the end %d", got)
	}
	// Constant curve 2x equals floor(k*2).
	flat := []SpeedPoint{pt(0, 2, 1), pt(1, 2, 1)}
	for k := range 38 {
		if got := CurveOffset(flat, 37, k); got != 2*k {
			t.Fatalf("flat k=%d got %d", k, got)
		}
	}
	// A ramp 1x -> 3x over 4 frames: the offset at frame k is floor(k + k^2/4).
	ramp := []SpeedPoint{pt(0, 1, 1), pt(1, 3, 1)}
	for k, want := range []int{0, 1, 3, 5, 8} {
		if got := CurveOffset(ramp, 4, k); got != want {
			t.Errorf("ramp k=%d got %d want %d", k, got, want)
		}
	}
	// Monotonic and never above the whole range for a rough curve.
	rough := []SpeedPoint{pt(0, 4, 1), pt(0.001, 1, 4), pt(0.7, 4, 1), pt(1, 1, 4)}
	prev := 0
	for k := range 978 {
		got := CurveOffset(rough, 977, k)
		if got < prev || k > 0 && got-prev > 4 {
			t.Fatalf("k=%d got %d after %d", k, got, prev)
		}
		prev = got
	}
	if prev > CurveSourceFrames(rough, 977) {
		t.Errorf("offset %d past the source range %d", prev, CurveSourceFrames(rough, 977))
	}
}

func TestCurveError(t *testing.T) {
	good := []SpeedPoint{pt(0, 1, 1), pt(1, 2, 1)}
	if m := CurveError(good); m != "" {
		t.Fatal(m)
	}
	for name, c := range map[string][]SpeedPoint{
		"one point":    {pt(0, 1, 1)},
		"first not 0":  {pt(0.1, 1, 1), pt(1, 1, 1)},
		"last not 1":   {pt(0, 1, 1), pt(0.9, 1, 1)},
		"not rising":   {pt(0, 1, 1), pt(0.5, 1, 1), pt(0.5, 2, 1), pt(1, 1, 1)},
		"too fast":     {pt(0, 1, 1), pt(1, 5, 1)},
		"too slow":     {pt(0, 1, 5), pt(1, 1, 1)},
		"not milli":    {pt(0, 1, 1), pt(1, 1, 3)},
		"t four digit": {pt(0, 1, 1), pt(0.5004, 1, 1), pt(1, 1, 1)},
	} {
		if CurveError(c) == "" {
			t.Errorf("%s accepted", name)
		}
	}
}
