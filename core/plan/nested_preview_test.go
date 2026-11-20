package plan

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// processedChild is a timeline with one clip with audio at frames 0 to 59 that has a chain.
func processedChild(chain ...timeline.Processor) *timeline.Document {
	d := childDoc(tlChild)

	a := d.Assets[astVideo]
	a.Hash = hashSound
	d.Assets[astVideo] = a

	it := clip("itm_s1", trkBottom, 0, 60)
	it.Processors = chain
	put(d, it)

	return d
}

func nestedAudioOf(t *testing.T, parent *timeline.Document, opts Options, from, to int) WindowAudio {
	t.Helper()

	audio := WindowWith(parent, from, to, opts).Audio
	for i := range audio {
		if a := audio[i]; a.Item == compID+"/itm_s1" {
			return a
		}
	}

	t.Fatalf("no audio entry for the nested item in %d..%d", from, to)

	return WindowAudio{}
}

// VV-275: the entry of an item with a chain inside a composition names the file of its processed sound, like the
// entry of an item of the document does, and the key is the one of the content.
func TestNestedWindowAudioNamesTheProcessedFile(t *testing.T) {
	t.Parallel()

	opts := production{tlChild: processedChild(hp)}.opts()

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 30, 60))

	got := nestedAudioOf(t, parent, opts, 0, 120).Processed
	if got == nil || len(got.Key) != 64 || got.Start != 30 || got.Frames != 60 {
		t.Fatalf("processed = %+v, want a key from frame 30 for 60 frames", got)
	}

	// The same item with a window of the plan that shows only a part of it: the file is the whole item.
	if part := nestedAudioOf(t, parent, opts, 50, 70).Processed; part == nil || *part != *got {
		t.Errorf("a window of the item gave %+v, want %+v", part, got)
	}

	// The composition at another place of the timeline is the same sound: the key does not hold the position.
	moved := parentDoc()
	put(moved, composition(compID, trkPar, tlChild, 100, 60))

	if other := nestedAudioOf(
		t,
		moved,
		opts,
		0,
		200,
	).Processed; other == nil || other.Key != got.Key ||
		other.Start != 100 {
		t.Errorf("moved composition: %+v, want the key %s from frame 100", other, got.Key)
	}

	// The key is the one of the source the file is rendered from.
	src, owner, ok := NestedPreviewSource(parent, opts, compID+"/itm_s1", 30, 90)
	if !ok || PreviewKey(owner, &src) != got.Key {
		t.Errorf("the key of NestedPreviewSource is %q, want %q (ok %v)", PreviewKey(owner, &src), got.Key, ok)
	}

	if len(src.Chain) != 1 || src.Gain != nil || src.StartSample != 0 || src.Track != "" || src.BusChain != nil {
		t.Errorf("source = %+v, want the chain and nothing of the envelope, the track and the position", src)
	}

	// A parameter of the chain changes the key, an item with no chain has no file.
	louder := hp
	louder.Params = map[string]float64{"hp_hz": 91}

	if other := nestedAudioOf(
		t,
		parent,
		production{tlChild: processedChild(louder)}.opts(),
		0,
		120,
	).Processed; other == nil ||
		other.Key == got.Key {
		t.Errorf("a changed parameter kept the key: %+v", other)
	}

	if plain := nestedAudioOf(t, parent, production{tlChild: processedChild()}.opts(), 0, 120).Processed; plain != nil {
		t.Errorf("an item with nothing to process has a processed file: %+v", plain)
	}
}

// The speed of the composition item is part of the sound the file holds, as the export plays it.
func TestNestedPreviewSourceHoldsTheSpeedOfTheComposition(t *testing.T) {
	t.Parallel()

	opts := production{tlChild: processedChild(hp)}.opts()

	parent := parentDoc()
	put(parent, composition(compID, trkPar, tlChild, 0, 30, func(i *timeline.Item) {
		i.Speed = timeline.Rational{Num: 2, Den: 1}
	}))

	src, _, ok := NestedPreviewSource(parent, opts, compID+"/itm_s1", 0, 30)
	if !ok || src.Speed != (timeline.Rational{Num: 2, Den: 1}) {
		t.Fatalf("source = %+v (ok %v), want speed 2", src, ok)
	}

	if got := nestedAudioOf(t, parent, opts, 0, 30).Processed; got == nil || got.Frames != 30 {
		t.Errorf("processed = %+v, want the 30 frames of the composition item", got)
	}
}
