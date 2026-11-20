package plan

import (
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// soundChild is a timeline with a clip with audio at frames 0 to 59 and another from 60 to 89.
func soundChild() *timeline.Document {
	d := childDoc(tlChild)
	put(d, clip("itm_s1", trkBottom, 0, 60))
	late := clip("itm_s2", trkTop, 60, 30)
	late.SourceIn = 100
	put(d, late)

	return d
}

func findSource(t *testing.T, srcs []AudioSource, id string) AudioSource {
	t.Helper()

	for i := range srcs {
		if srcs[i].ItemID == id {
			return srcs[i]
		}
	}

	t.Fatalf("no source %s in %v", id, srcs)

	return AudioSource{}
}

func TestNestedAudioMovesTheSourcesToTheItem(t *testing.T) {
	t.Parallel()

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 30, 90, func(i *timeline.Item) { i.SourceIn = 15 }))

	srcs := AudioWith(parent, 0, 200, production{tlChild: soundChild()}.opts())
	if len(srcs) != 2 {
		t.Fatalf("got %d sources, want the two clips of the child: %+v", len(srcs), srcs)
	}

	// The child's clip itm_s1 is at child frames 0 to 59; the item shows child frames 15 to 104 from frame 30, so
	// the source starts at the parent's frame 30 and is cut at 15 frames in: it plays child frames 15 to 59.
	s1 := findSource(t, srcs, compID+"/itm_s1")
	if s1.StartSample != 30*1600 {
		t.Errorf("itm_s1 starts at sample %d, want frame 30 = %d", s1.StartSample, 30*1600)
	}

	// The window of 30..120 over the item holds child frames 15 to 104: the first clip from its frame 15 on.
	if s1.AssetID != astVideo || s1.Samples != 45*1600 || s1.SourceStart != 15*1600 {
		t.Errorf("itm_s1 = %+v, want %d samples of the asset from source sample %d", s1, 45*1600, 15*1600)
	}

	// itm_s2 starts at child frame 60: parent frame 30 + (60 - 15) = 75, for the 30 frames that remain.
	s2 := findSource(t, srcs, compID+"/itm_s2")
	if s2.StartSample != 75*1600 || s2.Samples != 30*1600 || s2.SourceStart != 100*1600 {
		t.Errorf("itm_s2 = %+v, want start %d, %d samples, source %d", s2, 75*1600, 30*1600, 100*1600)
	}
}

func TestNestedAudioIsCutAtTheEndOfTheItemAndOfTheWindow(t *testing.T) {
	t.Parallel()

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 70))

	opts := production{tlChild: soundChild()}.opts()

	// The item is 70 frames: itm_s2 (frames 60 to 89 of the child) plays only its first 10 frames.
	if s2 := findSource(t, AudioWith(parent, 0, 200, opts), compID+"/itm_s2"); s2.Samples != 10*1600 {
		t.Errorf("itm_s2 has %d samples, want %d", s2.Samples, 10*1600)
	}

	// A window of frames 20 to 40 clips what the item gives.
	srcs := AudioWith(parent, 20, 40, opts)
	if len(srcs) != 1 || srcs[0].StartSample != 20*1600 || srcs[0].Samples != 20*1600 ||
		srcs[0].SourceStart != 20*1600 {
		t.Errorf("window 20..40 = %+v, want itm_s1 from frame 20 for 20 frames", srcs)
	}

	// A window after the item has no sound.
	if srcs := AudioWith(parent, 100, 140, opts); len(srcs) != 0 {
		t.Errorf("a window after the item sounds: %+v", srcs)
	}
}

func TestNestedAudioGainIsTheGainOfTheBusTimesTheGainOfTheSource(t *testing.T) {
	t.Parallel()

	child := soundChild()
	s1 := child.Items["itm_s1"]
	s1.Volume = 0.8
	child.Items["itm_s1"] = s1

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 90, func(i *timeline.Item) {
		i.Volume = 0.5
		i.FadeIn = 30
	}))

	srcs := AudioWith(parent, 0, 90, production{tlChild: child}.opts())
	g := findSource(t, srcs, compID+"/itm_s1").Gain

	// The fade-in of 30 frames takes the bus from 0 to 0.5 over 48,000 samples; the source is at 0.8 all along.
	at := func(sample int64) float64 { return gainAtSample(g, sample) }

	near(t, "gain at the start", at(0), 0)
	near(t, "gain half way through the fade", at(24000), 0.5*0.5*0.8)
	near(t, "gain after the fade", at(48000), 0.5*0.8)
	near(t, "gain at the end of the source", at(59*1600), 0.5*0.8)

	// The bus alone, for the other clip that starts after the fade.
	near(t, "other clip", gainAtSample(findSource(t, srcs, compID+"/itm_s2").Gain, 0), 0.5)
}

func TestNestedAudioNeedsASoundingItem(t *testing.T) {
	t.Parallel()

	child := soundChild()
	opts := production{tlChild: child}.opts()

	for name, mod := range map[string]func(*timeline.Item){
		"muted":    func(i *timeline.Item) { i.AudioMuted = true },
		"disabled": func(i *timeline.Item) { i.Disabled = true },
	} {
		parent := parentDoc()
		put(parent, composition(compID, trkPar, tlChild, 0, 90, mod))

		if srcs := AudioWith(parent, 0, 90, opts); len(srcs) != 0 {
			t.Errorf("%s composition sounds: %+v", name, srcs)
		}
	}

	// A muted track silences its compositions.
	parent := parentDoc()
	tr := parent.Tracks[trkPar]
	tr.Muted = true
	parent.Tracks[trkPar] = tr
	put(parent, composition(compID, trkPar, tlChild, 0, 90))

	if srcs := AudioWith(parent, 0, 90, opts); len(srcs) != 0 {
		t.Errorf("a composition on a muted track sounds: %+v", srcs)
	}

	// With no resolver the composition is silent and the plain sources are those of Audio.
	parent = parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 90))
	put(parent, clip("itm_own", trkOver, 0, 60))

	if got, want := AudioWith(parent, 0, 90, Options{}), Audio(parent, 0, 90); !reflect.DeepEqual(got, want) {
		t.Errorf("with no resolver AudioWith = %+v, want Audio = %+v", got, want)
	}
}

func TestNestedAudioOtherFrameRateAndTwoLevels(t *testing.T) {
	t.Parallel()

	// A child at 24 fps: its frame 24 is one second in, which is parent frame 30.
	child := soundChild()
	child.Settings.FPS = timeline.Rational{Num: 24, Den: 1}
	late := child.Items["itm_s2"]
	late.Start, late.Duration = 24, 24
	child.Items["itm_s2"] = late

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 120))

	s2 := findSource(t, AudioWith(parent, 0, 200, production{tlChild: child}.opts()), compID+"/itm_s2")
	if s2.StartSample != 48000 || s2.Samples != 48000 {
		t.Errorf("itm_s2 starts at sample %d for %d, want one second for one second", s2.StartSample, s2.Samples)
	}

	// Two levels: the ids are the whole path and the starts add up.
	mid := childDoc(tlInner)
	put(mid, composition("itm_mid", trkTop, tlChild, 30, 90))

	outer := parentDoc()
	put(outer, composition(compID, trkPar, tlInner, 60, 120))

	srcs := AudioWith(outer, 0, 300, production{tlInner: mid, tlChild: soundChild()}.opts())
	deep := findSource(t, srcs, compID+"/itm_mid/itm_s1")

	if deep.StartSample != 90*1600 || !strings.HasPrefix(deep.ItemID, compID+"/") {
		t.Errorf("the nested clip starts at sample %d, want frame 90 = %d", deep.StartSample, 90*1600)
	}
}

func TestNestedAudioWindowHasTheSameSources(t *testing.T) {
	t.Parallel()

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 10, 90, func(i *timeline.Item) { i.Volume = 0.5 }))

	opts := production{tlChild: soundChild()}.opts()
	w := WindowWith(parent, 0, 120, opts)

	if len(w.Audio) != 2 {
		t.Fatalf("window audio has %d entries, want 2: %+v", len(w.Audio), w.Audio)
	}

	var s1 WindowAudio

	for _, a := range w.Audio {
		if a.Item == compID+"/itm_s1" {
			s1 = a
		}
	}

	if s1.First != 10 || s1.Last != 69 || len(s1.Gain) != 60 || s1.Asset != astVideo {
		t.Errorf(
			"itm_s1 window entry = first %d last %d, %d gains, asset %q",
			s1.First,
			s1.Last,
			len(s1.Gain),
			s1.Asset,
		)
	}

	near(t, "window gain", s1.Gain[0], 0.5)
}

func TestNestedAudioTrimFront(t *testing.T) {
	t.Parallel()

	s := AudioSource{
		StartSample: 100, SourceStart: 5000, Samples: 1000, Speed: timeline.Rational{Num: 2, Den: 1},
		Gain: []GainPoint{{Sample: 0, Gain: 1}, {Sample: 1000, Gain: 0}},
	}

	if !s.trim(300, 5000) {
		t.Fatal("a source that plays on after the cut was dropped")
	}

	// 200 samples cut at speed 2 move the source by 400.
	if s.StartSample != 300 || s.SourceStart != 5400 || s.Samples != 800 {
		t.Errorf("trimmed to %+v", s)
	}

	near(t, "gain at the new start", s.Gain[0].Gain, 0.8)

	rev := AudioSource{StartSample: 0, Samples: 100, Speed: timeline.Rational{Num: 1, Den: 1}, Reverse: true}
	if rev.trim(10, 100) {
		t.Error("a reversed source was cut at its start")
	}
}
