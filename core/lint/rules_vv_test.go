package lint

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func occludedDoc(mask *timeline.Mask) *timeline.Document {
	d := newDoc()
	low := clip("itm_low0000001", 0, 90)
	low.Track = trkText
	up := clip("itm_up00000001", 0, 90)
	up.Fit, up.Mask = "cover", mask
	put(d, up)
	put(d, low)
	d.Tracks[trkText] = timeline.Track{ID: trkText, Kind: timeline.TrackVisual, Order: "a2"}
	d.Tracks[trkVideo] = timeline.Track{ID: trkVideo, Kind: timeline.TrackVisual, Order: "a1"}

	return d
}

// A mask leaves the clip below visible (VV-134).
func TestOccludedIgnoresMaskedClip(t *testing.T) {
	t.Parallel()

	if got := has(Lint(occludedDoc(nil), Env{}, Fast), CodeOccluded); len(got) != 1 {
		t.Fatalf("an unmasked covering clip must occlude: %+v", got)
	}

	m := &timeline.Mask{Shape: "rect", Width: 0.5, Height: 1}
	if got := has(Lint(occludedDoc(m), Env{}, Fast), CodeOccluded); len(got) != 0 {
		t.Errorf("a masked clip hides nothing: %+v", got)
	}
}

// A dip passes through a colour, so nothing beneath it is expected (VV-135).
func TestTransitionEmptyIgnoresDip(t *testing.T) {
	t.Parallel()

	for kind, want := range map[string]int{"dip": 0, "fade": 1} {
		d := newDoc()
		a, b := clip("itm_a0000000001", 0, 90), clip("itm_b0000000001", 90, 90)
		b.TransitionIn = &timeline.Transition{Kind: kind, Duration: 6}
		put(d, a)
		put(d, b)

		if got := has(Lint(d, Env{}, Fast), CodeTransitionEmpty); len(got) != want {
			t.Errorf("%s: %d findings, want %d: %+v", kind, len(got), want, got)
		}
	}
}

// A project with no visual track has no black frames (VV-137).
func TestAudioOnlyProjectHasNoBlackFrames(t *testing.T) {
	t.Parallel()

	d := newDoc()
	delete(d.Tracks, trkText)
	delete(d.Tracks, trkVideo)
	it := timeline.NewItem("itm_snd0000001", timeline.ItemAudio, trkAudio, 0, 9000)
	it.Asset = astSound
	put(d, it)

	if got := has(Lint(d, Env{}, Fast), CodeBlackFrames); len(got) != 0 {
		t.Errorf("audio-only: %+v", got)
	}
}

// A text that slides in is judged where it rests, not at its first frame (VV-121).
func TestSafeAreaIgnoresSlideIn(t *testing.T) {
	t.Parallel()

	for _, slide := range []bool{false, true} {
		d := newDoc()
		it := timeline.NewItem("itm_txt0000001", timeline.ItemText, trkText, 0, 90)
		it.Text = "hello"
		x := -900.0
		it.X = &x

		if slide {
			it.TransitionIn = &timeline.Transition{Kind: "slide_left", Duration: 12}
		}

		put(d, it)

		got := has(Lint(d, env(), Fast), CodeSafeArea)
		if want := map[bool]int{false: 1, true: 0}[slide]; len(got) != want {
			t.Errorf("slide %v: %d findings, want %d: %+v", slide, len(got), want, got)
		}
	}
}

// The loudness estimate never promises a gain the export cannot reach: peak headroom plus the
// 12 dB the limiter may take off (VV-074).
func TestLoudnessEstimateIsCappedByPeaks(t *testing.T) {
	t.Parallel()

	d := newDoc()
	d.Settings.LoudnessTarget = -14

	a := clip("itm_a0000000001", 0, 90)
	a.Fit = "cover"
	put(d, a)

	peaks := make([]float64, 400)
	for i := range peaks {
		peaks[i] = 0.5 // -6 dBFS
	}

	arts := fakeArtifacts{
		envSpec{Peaks: map[string][]float64{astVideo: peaks}, Loudness: map[string]float64{astVideo: -50}},
	}

	got := has(Lint(d, Env{Artifacts: arts}, Full), CodeLoudness)
	if len(got) != 1 || !strings.Contains(got[0].Message, "gain of +16.5 dB") {
		t.Fatalf("want a gain capped at +16.5 dB: %+v", got)
	}
}
