package plan

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func TestTransitionFactors(t *testing.T) {
	t.Parallel()

	in := timeline.NewItem("itm_t", timeline.ItemText, trkTop, 0, 100)
	in.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 10}
	in.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 10}

	for r, want := range map[int]float64{0: 0.091, 9: 0.909, 50: 1, 90: 0.909, 99: 0.091} {
		near(t, "opacity", round3(evaluate(&in, r, 1080, 1920, 540, 960).opacity), want)
	}
}

func TestSlideLeftOffset(t *testing.T) {
	t.Parallel()

	it := timeline.NewItem("itm_t", timeline.ItemText, trkTop, 0, 100)
	it.TransitionIn = &timeline.Transition{Kind: "slide_left", Duration: 10}
	p := 1.0 / 11
	s := 1 - (1-p)*(1-p)*(1-p)
	near(t, "x at r=0", evaluate(&it, 0, 1080, 1920, 540, 960).x-540, -(1-s)*1080)
	near(t, "x after the transition", evaluate(&it, 10, 1080, 1920, 540, 960).x, 540)
}

func TestWindowDeterministic(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_c0000000001", trkTop, 0, 60)
	c.Keyframes = timeline.Keyframes{"scale": {{T: 0, Value: 1}, {T: 40, Value: 1.5, Easing: "ease_in_out"}}}
	put(d, c)

	var first []byte

	for range 2 {
		b, err := json.Marshal(WindowWith(d, 0, 60, Options{Measure: fakeMeasure}))
		if err != nil {
			t.Fatal(err)
		}

		if first == nil {
			first = b
		} else if !bytes.Equal(first, b) {
			t.Fatal("two runs of Window gave different bytes")
		}
	}
}

func TestPixelScaleFollowsTheOutputScale(t *testing.T) {
	t.Parallel()

	it := timeline.NewItem("itm_t", timeline.ItemText, trkTop, 0, 30)
	it.Keyframes = timeline.Keyframes{"scale": {{T: 0, Value: 1}, {T: 20, Value: 1.2}}}

	for out, want := range map[float64]float64{0: 1.25, 1: 1.25, 0.5: 0.75, 10: 4} {
		if got := pixelScale(&it, out); got != want {
			t.Errorf("pixelScale at output scale %v = %v, want %v", out, got, want)
		}
	}

	it.Keyframes, it.Scale = nil, 20
	if got := pixelScale(&it, 1); got != 4 {
		t.Errorf("scale 20 = %v, want 4", got)
	}

	// The scale reaches the bitmap of a frame.
	d := newDoc()
	tx := timeline.NewItem("itm_x0000000001", timeline.ItemText, trkTop, 0, 30)
	tx.Text, tx.Scale = "hi", 1.2
	put(d, tx)

	if got := FrameWith(d, 0, Options{OutputScale: 0.5}).Layers[0].Bitmap.PixelScale; got != 0.75 {
		t.Errorf("frame pixel scale = %v, want 0.75", got)
	}
}

func TestColorMatrixMidGrey(t *testing.T) {
	t.Parallel()

	it := timeline.NewItem("itm_c", timeline.ItemClip, trkTop, 0, 30)
	it.Brightness, it.Contrast, it.Saturation = 0.1, 1.2, 0.5

	m := colorMatrix(&it)
	for row := range 3 {
		got := m[row*4]*0.5 + m[row*4+1]*0.5 + m[row*4+2]*0.5 + m[row*4+3]
		near(t, "channel", got, 0.6)
	}
}

func TestSafeAreaFourFive(t *testing.T) {
	t.Parallel()

	if got, want := SafeArea(1080, 1080), (Rect{54, 54, 972, 972}); got != want {
		t.Errorf("SafeArea(1080, 1080) = %+v, want %+v", got, want)
	}

	// 4:5 is above the 0.6 ratio: 5% on every side, rounded to whole pixels.
	got := SafeArea(1080, 1350)
	if got.X != 54 || got.W != 972 || math.Abs(got.Y-67.5) > 0.5 || math.Abs(got.Y+got.H-1282.5) > 0.5 {
		t.Errorf("SafeArea(1080, 1350) = %+v, want 5%% on every side", got)
	}
}

func TestActiveWord(t *testing.T) {
	t.Parallel()

	words := []timeline.Word{
		{Text: "a", Start: 0, Duration: 9},
		{Text: "b", Start: 9, Duration: 6},
		{Text: "c", Start: 20, Duration: 5},
	}

	for r, want := range map[int]int{0: 0, 8: 0, 9: 1, 14: 1, 16: -1, 20: 2, 24: 2, 25: -1} {
		if got := activeWord(words, r); got != want {
			t.Errorf("activeWord(%d) = %d, want %d", r, got, want)
		}
	}
}

func TestSampleConversion(t *testing.T) {
	t.Parallel()

	r30 := timeline.Rational{Num: 30, Den: 1}
	r2997 := timeline.Rational{Num: 30000, Den: 1001}

	if got := frameSample(r30, 1); got != 1600 {
		t.Errorf("30 fps frame 1 = %d", got)
	}

	if got := frameSample(r2997, 1); got != 1601 {
		t.Errorf("29.97 fps frame 1 = %d", got)
	}

	if got := frameSample(r30, 30) - 1; got != 47999 {
		t.Errorf("an item of 30 frames ends at %d", got)
	}
}

func snapDoc() *timeline.Document {
	d := newDoc()
	a := clip("itm_a0000000001", trkBottom, 10, 60)
	a.SourceIn = 20
	put(d, a)
	put(d, clip("itm_a0000000002", trkBottom, 100, 50))

	img := timeline.NewItem("itm_a0000000003", timeline.ItemClip, trkBottom, 150, 10)
	img.Asset = astImage
	put(d, img)

	cap1 := timeline.NewItem("itm_a0000000004", timeline.ItemCaption, trkTop, 0, 60)
	cap1.Words = []timeline.Word{{Text: "one", Start: 0, Duration: 20}, {Text: "two", Start: 20, Duration: 30}}
	put(d, cap1)

	return d
}

func TestSnapPointsGolden(t *testing.T) {
	t.Parallel()

	for name, words := range map[string]bool{"without-words": false, "with-words": true} {
		first, err := json.MarshalIndent(SnapPoints(snapDoc(), words), "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		second, err := json.MarshalIndent(SnapPoints(snapDoc(), words), "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(first, second) {
			t.Errorf("%s: two runs differ", name)
		}

		first = append(first, '\n')
		path := filepath.Join("testdata", "snap", name+".json")

		if *update {
			if err := os.WriteFile(path, first, 0o600); err != nil {
				t.Fatal(err)
			}
		}

		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(first, want) {
			t.Errorf("%s differs from %s (run with -update after reviewing)", name, path)
		}
	}
}

func TestSnapLimits(t *testing.T) {
	t.Parallel()

	got := SnapPoints(snapDoc(), false).Limits
	// The first clip reads from source_in 20 of a 900-frame asset: its start edge can
	// go 20 frames earlier but stops at the timeline start; the neighbour bounds its end.
	if l := got["itm_a0000000001"]; l.TrimStartMin != 0 || l.TrimEndMax != 100 {
		t.Errorf("first clip %+v", l)
	}

	// The second clip starts at source_in 0: nothing earlier to read, so its start edge
	// cannot move earlier; the next item bounds its end.
	if l := got["itm_a0000000002"]; l.TrimStartMin != 100 || l.TrimEndMax != 150 {
		t.Errorf("second clip %+v", l)
	}

	// An image has no source limit; the last item reaches the end of the timeline.
	if l := got["itm_a0000000003"]; l.TrimStartMin != 150 || l.TrimEndMax != timeline.MaxFrames {
		t.Errorf("image %+v", l)
	}
}

func TestSnapLimitsFollowTheSource(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_a0000000001", trkBottom, 500, 100)
	c.SourceIn = 40
	c.Speed = timeline.Rational{Num: 2, Den: 1}
	put(d, c)

	l := SnapPoints(d, false).Limits["itm_a0000000001"]
	// 40 source frames at speed 2 are 20 timeline frames; (900-40)/2 = 430 frames remain.
	if l.TrimStartMin != 480 || l.TrimEndMax != 930 {
		t.Errorf("%+v", l)
	}
}

func TestAudioFadesGolden(t *testing.T) {
	t.Parallel()

	d := newDoc()
	c := clip("itm_c0000000001", trkTop, 0, 90)
	c.FadeIn, c.FadeOut = 15, 15
	c.Keyframes = timeline.Keyframes{"volume": {{T: 0, Value: 1}, {T: 90, Value: 0.5}}}
	put(d, c)

	got, err := json.MarshalIndent(Audio(d, 0, 90), "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	got = append(got, '\n')
	path := filepath.Join("testdata", "audio", "fades.json")

	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("audio plan differs from %s (run with -update after reviewing)", path)
	}
}

// The document of a clip.set_color golden case renders to the colour matrix of its
// want file: command and plan agree on what the stored values mean.
func TestColorGoldenRendersToItsMatrix(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("..", "command", "testdata", "clip.set_color", "ok_all_three", "after.json"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := timeline.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	layers := Frame(doc, 0).Layers
	if len(layers) != 1 {
		t.Fatalf("layers = %+v", layers)
	}

	got, err := json.MarshalIndent(layers[0].ColorMatrix, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	got = append(got, '\n')
	path := filepath.Join("testdata", "color", "ok_all_three.want.json")

	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("matrix %s differs from %s", got, path)
	}
}
