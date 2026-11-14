package timeline

import (
	"math"
	"testing"
)

func TestEaseTable(t *testing.T) {
	for _, c := range []struct {
		kind    string
		u, want float64
	}{
		{"linear", 0.25, 0.25},
		{"", 0.25, 0.25},
		{"unknown", 0.4, 0.4},
		{"ease_in", 0.5, 0.125},
		{"ease_out", 0.5, 0.875},
		{"ease_in_out", 0.25, 0.0625},
		{"ease_in_out", 0.75, 0.9375},
		{"hold", 0.99, 0},
	} {
		if got := Ease(c.kind, c.u); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("Ease(%q, %v) = %v, want %v", c.kind, c.u, got, c.want)
		}
	}
}

func TestValueAt(t *testing.T) {
	keys := []Keyframe{{T: 10, Value: 1}, {T: 20, Value: 3, Easing: "hold"}, {T: 30, Value: 5}}
	for r, want := range map[int]float64{0: 1, 10: 1, 15: 2, 20: 3, 25: 3, 30: 5, 99: 5} {
		if got := ValueAt(keys, r, 9); got != want {
			t.Errorf("ValueAt(%d) = %v, want %v", r, got, want)
		}
	}
	if got := ValueAt(nil, 4, 7.5); got != 7.5 {
		t.Errorf("static fallback = %v", got)
	}
	if got := ValueAt([]Keyframe{{T: 0, Value: 0}, {T: 3, Value: 1}}, 1, 0); got != 0.333 {
		t.Errorf("rounding: %v", got)
	}
	if got := Interpolate(keys, 12.5); math.Abs(got-1.5) > 1e-12 {
		t.Errorf("fractional r: %v", got)
	}
}
