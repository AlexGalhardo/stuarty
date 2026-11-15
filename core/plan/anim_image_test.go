package plan

import (
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

const astAnim = "ast_anim0000001"

// delays are 2, 1 and 3 frames at 30 fps, in the milliseconds a file records them.
var animDelays = []int{67, 33, 100}

// RF-028: a three-frame image with delays of 2, 1 and 3 frames at 30 fps selects
// frames 0, 0, 1, 2, 2, 2, 0 for item frames 0 to 6 when it loops.
func TestAnimImageSourceFrame(t *testing.T) {
	t.Parallel()

	fps := timeline.Rational{Num: 30, Den: 1}
	want := []int{0, 0, 1, 2, 2, 2, 0}

	for r, w := range want {
		if got := AnimImageSourceFrame(animDelays, fps, r); got != w {
			t.Errorf("item frame %d: frame %d, want %d", r, got, w)
		}
	}
}

func TestAnimImageSourceFrameEdges(t *testing.T) {
	t.Parallel()

	fps := timeline.Rational{Num: 30, Den: 1}

	for _, c := range []struct {
		name   string
		delays []int
		fps    timeline.Rational
		r      int
		want   int
	}{
		{"no delays", nil, fps, 5, 0},
		{"a zero delay", []int{50, 0}, fps, 5, 0},
		{"a zero rate", animDelays, timeline.Rational{}, 5, 0},
		{"negative frame", animDelays, fps, -3, 0},
		{"many loops", animDelays, fps, 30 * 600, 0},
		{"ntsc 29.97", animDelays, timeline.Rational{Num: 30000, Den: 1001}, 3, 2},
		{"60 fps", animDelays, timeline.Rational{Num: 60, Den: 1}, 4, 1},
	} {
		if got := AnimImageSourceFrame(c.delays, c.fps, c.r); got != c.want {
			t.Errorf("%s: frame %d, want %d", c.name, got, c.want)
		}
	}
}

func animDoc() *timeline.Document {
	d := newDoc()
	d.Assets[astAnim] = timeline.Asset{
		ID: astAnim, Kind: timeline.AssetImage, Width: 200, Height: 200, FrameCount: 3, FrameDelays: animDelays,
	}
	it := timeline.NewItem("itm_a1b2c3d4e5", timeline.ItemClip, trkTop, 10, 12)
	it.Asset = astAnim
	put(d, it)

	return d
}

// The layer of an animated image carries its frame, the window carries the series,
// and a still image keeps frame 0 and no series.
func TestAnimImageLayerAndWindow(t *testing.T) {
	t.Parallel()

	d := animDoc()

	var got []int

	for t0 := 10; t0 < 22; t0++ {
		fp := Frame(d, t0)
		if len(fp.Layers) != 1 {
			t.Fatalf("frame %d: %d layers", t0, len(fp.Layers))
		}

		got = append(got, fp.Layers[0].SourceFrame)
	}

	want := []int{0, 0, 1, 2, 2, 2, 0, 0, 1, 2, 2, 2}
	if !slices.Equal(got, want) {
		t.Errorf("layer frames %v, want %v", got, want)
	}

	w := Window(d, 10, 22)
	if len(w.Layers) != 1 || !slices.Equal(w.Layers[0].SourceFrames, want) {
		t.Errorf("window frames %v, want %v", w.Layers[0].SourceFrames, want)
	}

	still := newDoc()
	it := timeline.NewItem("itm_a1b2c3d4e5", timeline.ItemClip, trkTop, 0, 5)
	it.Asset = astImage
	put(still, it)

	if w := Window(still, 0, 5); w.Layers[0].SourceFrames != nil {
		t.Errorf("a still image has frames %v", w.Layers[0].SourceFrames)
	}
}
