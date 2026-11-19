package service

import (
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func TestSpeechFramesRoundOutward(t *testing.T) {
	t.Parallel()

	fps := timeline.Rational{Num: 30, Den: 1}
	// A frame is 1600 samples: [1000, 3300) touches frames 0 to 2; [3500, 3600) is frame 2 only
	// and joins the previous range because they touch.
	raw := []byte(
		`{"sample_rate":48000,"ranges":[{"start":1000,"end":3300},{"start":3500,"end":3600},{"start":96000,"end":96001}]}`,
	)

	got, ok := speechFrames(raw, fps)
	want := []command.SilenceRange{{Start: 0, End: 3}, {Start: 60, End: 61}}

	if !ok || !slices.Equal(got, want) {
		t.Errorf("speechFrames = %v, %v; want %v", got, ok, want)
	}

	if _, ok := speechFrames([]byte(`{"version":1}`), fps); ok {
		t.Error("an artifact without a sample rate must not resolve")
	}
}

func TestBeatFramesIncreasingAndNearest(t *testing.T) {
	t.Parallel()

	fps := timeline.Rational{Num: 30, Den: 1}
	// 800 samples is half a frame (rounds up to 1); 100 is frame 0; 900 also rounds to 1 and is
	// dropped as a repeat.
	raw := []byte(`{"sample_rate":48000,"beats":[100,800,900,24000]}`)

	got, ok := beatFrames(raw, fps)
	want := []int{0, 1, 15}

	if !ok || !slices.Equal(got, want) {
		t.Errorf("beatFrames = %v, %v; want %v", got, ok, want)
	}

	if got, ok := beatFrames([]byte(`{"sample_rate":48000,"beats":[]}`), fps); !ok || len(got) != 0 {
		t.Errorf("an empty grid is valid and empty: %v, %v", got, ok)
	}
}
