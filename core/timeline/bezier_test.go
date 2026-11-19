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
