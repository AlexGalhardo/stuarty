package ingest

import (
	"math"
	"testing"
)

func regular(n int, step float64) []float64 {
	p := make([]float64, n)
	for i := range p {
		p[i] = float64(i) * step
	}
	return p
}

func TestNominalRate(t *testing.T) {
	// Frames kept 11 of every 20 in bursts: an average near 33 and a capture rate of 60.
	var vfr []float64
	for n := 0; len(vfr) < 400; n++ {
		if n%20 < 11 {
			vfr = append(vfr, float64(n)/60)
		}
	}
	// 60 fps with timestamps rounded to the millisecond: 16, 17, 17 ...
	ms := make([]float64, 600)
	for i := range ms {
		ms[i] = math.Round(float64(i)*1000/60) / 1000
	}

	cases := []struct {
		name string
		pts  []float64
		r, a Rational
		want Rational
	}{
		{"1800 packets at 1/30 s", regular(1800, 1.0/30), Rational{30, 1}, Rational{30, 1}, Rational{30, 1}},
		{
			"150 packets at 1001/30000 s",
			regular(150, 1001.0/30000),
			Rational{30000, 1001},
			Rational{30000, 1001},
			Rational{30000, 1001},
		},
		{"23.976 fps", regular(240, 1001.0/24000), Rational{24000, 1001}, Rational{24000, 1001}, Rational{24000, 1001}},
		{"the VFR fixture", vfr, Rational{60, 1}, Rational{3200, 99}, Rational{60, 1}},
		{"60 fps rounded to milliseconds", ms, Rational{60, 1}, Rational{60, 1}, Rational{60, 1}},
		{"120 fps", regular(600, 1.0/120), Rational{120, 1}, Rational{120, 1}, Rational{60, 1}},
		{
			"119.88 fps",
			regular(600, 1001.0/120000),
			Rational{120000, 1001},
			Rational{120000, 1001},
			Rational{60000, 1001},
		},
		{"25 fps", regular(250, 1.0/25), Rational{25, 1}, Rational{25, 1}, Rational{25, 1}},
		{"fewer than 10 packets, r_frame_rate 30/1", regular(5, 0.1), Rational{30, 1}, Rational{0, 0}, Rational{30, 1}},
		{"fewer than 10 packets, cover art", regular(1, 0), Rational{90000, 1}, Rational{24, 1}, Rational{24, 1}},
	}
	for _, c := range cases {
		got, _ := NominalRate(c.pts, c.r, c.a)
		if got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNominalRateWarningsAndOrder(t *testing.T) {
	if _, w := NominalRate(nil, Rational{}, Rational{}); len(w) != 1 || w[0] != "frame_rate_unknown" {
		t.Errorf("no information: %v", w)
	}
	// 13.5 fps is more than 2% from 13/1 and 14/1: irregular.
	if r, w := NominalRate(
		regular(100, 1/13.5),
		Rational{27, 2},
		Rational{27, 2},
	); len(w) != 1 || w[0] != "irregular_frame_rate" ||
		r.Den != 1 {
		t.Errorf("13.5 fps: %v %v", r, w)
	}
	// B-frames make timestamps arrive out of order; they are sorted first.
	pts := regular(300, 1.0/30)
	pts[10], pts[11] = pts[11], pts[10]
	if r, _ := NominalRate(pts, Rational{30, 1}, Rational{30, 1}); r != (Rational{30, 1}) {
		t.Errorf("out of order: %v", r)
	}
	if v, _, _ := MeasureRate(regular(1000, 1.0/240), Rational{240, 1}, Rational{240, 1}); v < 239 || v > 241 {
		t.Errorf("measure of 240 fps = %v", v)
	}
}

func TestParseRationalAndVFR(t *testing.T) {
	if r, ok := ParseRational("30000/1001"); !ok || r != (Rational{30000, 1001}) {
		t.Errorf("%v %v", r, ok)
	}
	for _, bad := range []string{"0/0", "30", "a/b", "30/0", "-1/2", ""} {
		if _, ok := ParseRational(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
	if !IsVFR(Rational{60, 1}, Rational{3200, 99}, 0) || IsVFR(Rational{30, 1}, Rational{30, 1}, 0) ||
		!IsVFR(Rational{30, 1}, Rational{30, 1}, 0.02) {
		t.Error("IsVFR")
	}
}
