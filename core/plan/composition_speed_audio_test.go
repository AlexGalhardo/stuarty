package plan

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Epic 53, SDD-004 section 7 "A composition at a speed": every nested source is moved by the time map, and its
// speed is its own times the speed of the item, so the decoder (atempo) keeps the pitch.

func TestCompositionSpeedAudioMapsTheSources(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name         string
		num, den     int
		dur          int
		s1Start      int64 // output sample of the first clip
		s1Samples    int64
		s2Start      int64
		s2Samples    int64
		speed        timeline.Rational
		fadeInPoints []int64 // the samples of the gain points of itm_s1
	}{
		{"double", 2, 1, 45, 30 * 1600, 30 * 1600, 60 * 1600, 15 * 1600, timeline.Rational{Num: 2, Den: 1}, []int64{0, 8000, 48000}},
		{"half", 1, 2, 180, 30 * 1600, 120 * 1600, 150 * 1600, 60 * 1600, timeline.Rational{Num: 1, Den: 2}, []int64{0, 32000, 192000}},
		{"three halves", 3, 2, 60, 30 * 1600, 40 * 1600, 70 * 1600, 20 * 1600, timeline.Rational{Num: 3, Den: 2}, []int64{0, 10666, 64000}},
	} {
		child := soundChild()
		s1 := child.Items["itm_s1"]
		s1.FadeIn = 10
		child.Items["itm_s1"] = s1

		parent := parentDoc()
		put(parent, composition(compID, trkPar, tlChild, 30, c.dur, speedOf(c.num, c.den)))

		srcs := AudioWith(parent, 0, 400, production{tlChild: child}.opts())
		if len(srcs) != 2 {
			t.Fatalf("%s: %d sources, want 2", c.name, len(srcs))
		}

		a, b := findSource(t, srcs, compID+"/itm_s1"), findSource(t, srcs, compID+"/itm_s2")

		if a.StartSample != c.s1Start || a.Samples != c.s1Samples || a.Speed != c.speed || a.SourceStart != 0 {
			t.Errorf("%s itm_s1 = start %d samples %d speed %v source %d, want %d %d %v 0",
				c.name, a.StartSample, a.Samples, a.Speed, a.SourceStart, c.s1Start, c.s1Samples, c.speed)
		}

		if b.StartSample != c.s2Start || b.Samples != c.s2Samples || b.Speed != c.speed || b.SourceStart != 100*1600 {
			t.Errorf("%s itm_s2 = start %d samples %d speed %v source %d, want %d %d %v %d",
				c.name, b.StartSample, b.Samples, b.Speed, b.SourceStart, c.s2Start, c.s2Samples, c.speed, 100*1600)
		}

		// The sources touch where they touched in the nested timeline: no rounding opens a gap.
		if a.StartSample+a.Samples != b.StartSample {
			t.Errorf("%s: itm_s1 ends at %d and itm_s2 starts at %d", c.name, a.StartSample+a.Samples, b.StartSample)
		}

		for i, want := range c.fadeInPoints {
			if i >= len(a.Gain) || a.Gain[i].Sample != want {
				t.Errorf("%s: gain points %v, want the samples %v", c.name, a.Gain, c.fadeInPoints)

				break
			}
		}
	}
}

// A window over the sped-up item cuts the sources at the window and keeps the speed.
func TestCompositionSpeedAudioWindowAndSourceIn(t *testing.T) {
	t.Parallel()

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 45, speedOf(2, 1)))

	opts := production{tlChild: soundChild()}.opts()

	// Item frames 10 to 20 show nested frames 20 to 40: one clip, from nested frame 20, for 10 frames.
	srcs := AudioWith(parent, 10, 20, opts)
	if len(srcs) != 1 {
		t.Fatalf("window: %d sources, want 1: %+v", len(srcs), srcs)
	}

	if s := srcs[0]; s.StartSample != 10*1600 || s.Samples != 10*1600 || s.SourceStart != 20*1600 || s.Speed.Num != 2 {
		t.Errorf("window source = %+v, want start %d, %d samples, source %d at 2x", s, 10*1600, 10*1600, 20*1600)
	}

	// A source_in of 10 frames of this document shows the nested timeline from its frame 10.
	shifted := parentDoc()
	put(
		shifted,
		composition(compID, trkPar, tlChild, 30, 45, speedOf(2, 1), func(i *timeline.Item) { i.SourceIn = 10 }),
	)

	s1 := findSource(t, AudioWith(shifted, 0, 400, opts), compID+"/itm_s1")
	if s1.StartSample != 30*1600 || s1.SourceStart != 10*1600 || s1.Samples != 25*1600 {
		t.Errorf("source_in: itm_s1 = %+v, want start %d, source %d, %d samples", s1, 30*1600, 10*1600, 25*1600)
	}
}

// A reversed nested source keeps its direction and takes the product of the speeds; a curve carries the scale.
func TestCompositionSpeedAudioReverseAndCurveInside(t *testing.T) {
	t.Parallel()

	child := soundChild()
	rev := child.Items["itm_s1"]
	rev.Reverse = true
	rev.Speed = timeline.Rational{Num: 3, Den: 2}
	rev.Duration = 40
	child.Items["itm_s1"] = rev

	late := child.Items["itm_s2"]
	late.SpeedCurve = []timeline.SpeedPoint{
		{T: 0, Speed: timeline.Rational{Num: 1, Den: 1}}, {T: 1, Speed: timeline.Rational{Num: 3, Den: 1}},
	}
	late.Speed = timeline.CurveMean(late.SpeedCurve)
	child.Items["itm_s2"] = late

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 45, speedOf(2, 1)))

	srcs := AudioWith(parent, 0, 100, production{tlChild: child}.opts())
	a, b := findSource(t, srcs, compID+"/itm_s1"), findSource(t, srcs, compID+"/itm_s2")

	if !a.Reverse || a.Speed != (timeline.Rational{Num: 3, Den: 1}) {
		t.Errorf("reversed source = reverse %v speed %v, want reverse at 3/2 x 2 = 3", a.Reverse, a.Speed)
	}

	if b.Curve == nil || b.Curve.TimeScale == nil || *b.Curve.TimeScale != (timeline.Rational{Num: 2, Den: 1}) ||
		b.Speed != (timeline.Rational{Num: 4, Den: 1}) {
		t.Errorf("curved source = %+v, want the time scale 2 and the mean 2 x 2 = 4", b)
	}

	// The window carries the curved source at its mean rate: the browser does not know the time scale.
	for _, w := range WindowWith(parent, 0, 45, production{tlChild: child}.opts()).Audio {
		if w.Item == compID+"/itm_s2" && (w.Curve != nil || w.Speed != 4) {
			t.Errorf("window entry of the curved source = %+v, want no curve at speed 4", w)
		}
	}
}

// Speed 1/1 is the rule of epic 32, to the sample: the same sources as before the speed existed.
func TestCompositionSpeedAudioAtOneIsUnchanged(t *testing.T) {
	t.Parallel()

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 30, 90, func(i *timeline.Item) { i.SourceIn = 15 }))

	srcs := AudioWith(parent, 0, 200, production{tlChild: soundChild()}.opts())
	s1 := findSource(t, srcs, compID+"/itm_s1")

	if s1.StartSample != 30*1600 || s1.Samples != 45*1600 || s1.SourceStart != 15*1600 || s1.Speed.Num != 1 ||
		s1.Curve != nil {
		t.Errorf("itm_s1 = %+v", s1)
	}
}
