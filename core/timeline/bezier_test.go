package timeline

import (
	"bytes"
	"math"
	"testing"
)

func TestBezierKeyframeRoundTripsAndValidates(t *testing.T) {
	d := base(t)
	editItem(d, clipID, func(i *Item) {
		k := NewKeyframe(0, 1)
		k.Easing, k.Bezier = EasingBezier, []float64{0.34, 1.56, 0.64, 1}
		i.Keyframes = Keyframes{"scale": {k, NewKeyframe(30, 2)}}
	})

	if vs := Validate(d); len(vs) != 0 {
		t.Fatalf("a valid curve is refused: %v", vs)
	}

	b := Canonical(d)
	if !bytes.Contains(b, []byte(`"bezier":[0.34,1.56,0.64,1]`)) {
		t.Fatalf("the curve is not written: %s", b)
	}

	back, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(Canonical(back), b) {
		t.Errorf("the document does not round-trip")
	}

	if got := back.Items[clipID].Keyframes["scale"][0].Bezier; len(got) != 4 || got[1] != 1.56 {
		t.Errorf("curve after decoding: %v", got)
	}

	cp := CloneItem(back.Items[clipID])
	cp.Keyframes["scale"][0].Bezier[0] = 0.9

	if back.Items[clipID].Keyframes["scale"][0].Bezier[0] != 0.34 {
		t.Errorf("a clone shares the curve with its source")
	}
}

func TestBezierInterpolateUsesTheCurveOfTheSegmentStart(t *testing.T) {
	curve := []float64{0.42, 0, 0.58, 1}
	keys := []Keyframe{
		{T: 0, Value: 0, Easing: EasingBezier, Bezier: curve},
		{T: 10, Value: 10, Easing: "linear"},
	}

	if got, want := Interpolate(keys, 2.5), 10*BezierEase(curve, 0.25); math.Abs(got-want) > 1e-12 {
		t.Errorf("Interpolate = %v, want %v", got, want)
	}

	if got := Interpolate(keys, 5); math.Abs(got-5) > 1e-6 {
		t.Errorf("the symmetric curve at the middle = %v, want 5", got)
	}

	keys[0].Bezier = nil // a bezier with no numbers is linear
	if got := Interpolate(keys, 2.5); got != 2.5 {
		t.Errorf("a bezier without numbers = %v, want linear 2.5", got)
	}
}

func TestValueAtPropKeepsTheValueInsideTheRange(t *testing.T) {
	keys := []Keyframe{
		{T: 0, Value: 0, Easing: EasingBezier, Bezier: []float64{0.34, 1.56, 0.64, 1}},
		{T: 10, Value: 1, Easing: "linear"},
	}

	if v := ValueAt(keys, 5, 0); v <= 1 {
		t.Fatalf("setup: the overshoot at the middle is %v, want above 1", v)
	}

	if v := ValueAtProp("opacity", keys, 5, 0); v != 1 {
		t.Errorf("opacity = %v, want 1", v)
	}

	if v := ValueAtProp("x", keys, 5, 0); v <= 1 {
		t.Errorf("x has no range, got %v", v)
	}
}

// The fixed-point arithmetic of SDD-004 section 3.1 gives the same bits on every system: these were measured
// once and must not change on another operating system, architecture or Go release. A Q30 result divided by
// 2^30 leaves the low bits of the mantissa zero.
func TestBezierEaseBitsArePinned(t *testing.T) {
	for _, c := range []struct {
		curve []float64
		u     float64
		bits  uint64
	}{
		{[]float64{0.34, 1.56, 0.64, 1}, 0.4321, 0x3ff0e0c851000000},
		{[]float64{0.36, 0, 0.66, -0.56}, 0.3, 0xbfb3660f5c000000},
		{[]float64{0.25, 0.1, 0.25, 1}, 0.5, 0x3fe9ad49dd800000},
		{[]float64{0.42, 0, 1, 1}, 1.0 / 3, 0x3fc3fd2dec000000},
		{[]float64{0, 3, 1, -2}, 0.7, 0x3fc0ba34da000000},
	} {
		if got := math.Float64bits(BezierEase(c.curve, c.u)); got != c.bits {
			t.Errorf("e(%v) of %v has bits %#016x, want %#016x", c.u, c.curve, got, c.bits)
		}
	}
}
