package plan

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func TestAudioHoldWithFades(t *testing.T) {
	t.Parallel()

	fps := timeline.Rational{Num: 30, Den: 1}

	t.Run("hold at t=0 preserves FadeIn ramp", func(t *testing.T) {
		t.Parallel()

		it := &timeline.Item{
			ID:       "it_1",
			Start:    0,
			Duration: 30,
			Volume:   1,
			FadeIn:   10,
			Keyframes: map[string][]timeline.Keyframe{
				"volume": {{T: 0, Value: 1, Easing: "hold"}},
			},
		}

		pts := envelope(it, fps, 0, 30)
		// Must not emit a 0.0 plateau right before FadeIn ends.
		for _, pt := range pts {
			if pt.Sample == frameSample(fps, 10)-1 && pt.Gain == 0 {
				t.Fatalf("FadeIn was flattened to 0 by hold easing: %+v", pts)
			}
		}

		if len(pts) < 2 || pts[0].Gain != 0 || pts[1].Gain != 1 {
			t.Fatalf("expected smooth ramp 0 -> 1 across FadeIn, got %+v", pts)
		}
	})

	t.Run("final hold keyframe preserves FadeOut ramp", func(t *testing.T) {
		t.Parallel()

		it := &timeline.Item{
			ID:       "it_2",
			Start:    0,
			Duration: 30,
			Volume:   1,
			FadeOut:  10,
			Keyframes: map[string][]timeline.Keyframe{
				"volume": {{T: 0, Value: 1, Easing: "hold"}},
			},
		}

		pts := envelope(it, fps, 0, 30)
		// Must not hold 1.0 right up to the final sample of FadeOut.
		for _, pt := range pts {
			if pt.Sample == frameSample(fps, 30)-1 && pt.Gain == 1 {
				t.Fatalf("FadeOut was hard-clipped at 1.0 by hold easing: %+v", pts)
			}
		}

		last := pts[len(pts)-1]
		if last.Gain != 0 {
			t.Fatalf("expected final gain 0 at end of FadeOut, got %+v", pts)
		}
	})

	t.Run("hold outside fades keeps step behavior", func(t *testing.T) {
		t.Parallel()

		it := &timeline.Item{
			ID:       "it_3",
			Start:    0,
			Duration: 30,
			Volume:   1,
			Keyframes: map[string][]timeline.Keyframe{
				"volume": {
					{T: 0, Value: 1, Easing: "hold"},
					{T: 10, Value: 0.5, Easing: "linear"},
				},
			},
		}

		pts := envelope(it, fps, 0, 30)
		wantSample := frameSample(fps, 10) - 1
		foundStep := false

		for _, pt := range pts {
			if pt.Sample == wantSample && pt.Gain == 1 {
				foundStep = true
			}
		}

		if !foundStep {
			t.Fatalf("expected hold step at sample %d with gain 1, got %+v", wantSample, pts)
		}
	})
}
