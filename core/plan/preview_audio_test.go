package plan

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const hashSound = "sha256:0000000000000000000000000000000000000000000000000000000000000001"

func previewDoc() *timeline.Document {
	d := newDoc()

	a := d.Assets[astSound]
	a.Hash = hashSound
	d.Assets[astSound] = a

	return d
}

func soundItem(id string, start int, chain ...timeline.Processor) timeline.Item {
	it := timeline.NewItem(id, timeline.ItemAudio, trkSound, start, 60)
	it.Asset = astSound
	it.Processors = chain

	return it
}

var hp = timeline.Processor{Kind: timeline.ProcEQ, Params: map[string]float64{"hp_hz": 90}}

func processedKey(t *testing.T, d *timeline.Document, id string) *ProcessedAudio {
	t.Helper()

	audio := Window(d, 0, 120).Audio

	for i := range audio {
		if audio[i].Item == id {
			return audio[i].Processed
		}
	}

	t.Fatalf("no audio entry for %s", id)

	return nil
}

func TestWindowAudioProcessedKey(t *testing.T) {
	t.Parallel()

	d := previewDoc()
	put(d, soundItem("itm_a", 0, hp))
	put(d, soundItem("itm_b", 30, hp))
	put(d, soundItem("itm_plain", 0))

	if processedKey(t, d, "itm_plain") != nil {
		t.Error("an item with nothing to process has a processed file")
	}

	a, b := processedKey(t, d, "itm_a"), processedKey(t, d, "itm_b")
	if a == nil || len(a.Key) != 64 || a.Start != 0 || a.Frames != 60 {
		t.Fatalf("processed = %+v", a)
	}

	if b.Key != a.Key || b.Start != 30 {
		t.Errorf("two items with one chain and one range must share the key: %+v and %+v", a, b)
	}

	// Another item, the volume and the fades are not in the key; a parameter is.
	other := soundItem("itm_c", 10)
	other.Volume = 0.2
	put(d, other)

	louder := d.Items["itm_a"]
	louder.Volume, louder.FadeOut = 2, 12
	put(d, louder)

	if got := processedKey(t, d, "itm_a"); got.Key != a.Key {
		t.Error("an edit outside the audio entry moved the key")
	}

	moved := d.Items["itm_a"]
	moved.Processors = []timeline.Processor{{Kind: timeline.ProcEQ, Params: map[string]float64{"hp_hz": 91}}}
	put(d, moved)

	if got := processedKey(t, d, "itm_a"); got.Key == a.Key {
		t.Error("a changed parameter kept the key")
	}
}

func TestPreviewSubjectGivesTheSameSource(t *testing.T) {
	t.Parallel()

	d := previewDoc()
	it := soundItem("itm_a", 12, hp)
	it.SourceIn = 7
	put(d, it)

	want, ok := PreviewSource(d, "itm_a", 20, 50)
	if !ok {
		t.Fatal("no source")
	}

	got, ok := PreviewSource(PreviewSubject(d, "itm_a"), "itm_a", 20, 50)
	if !ok || PreviewKey(d, &want) != PreviewKey(PreviewSubject(d, "itm_a"), &got) {
		t.Errorf("the subject renders another source: %+v and %+v", got, want)
	}
}

// The mixer reads a reversed item from the end of a source frame and a speed curve at frame boundaries, so the frame
// rate changes the samples: two projects whose sources are equal in samples at 30 and at 25 fps have two keys, and
// one never plays the file of the other.
func TestPreviewKeyHoldsTheFrameRate(t *testing.T) {
	t.Parallel()

	// 6 frames at 30 fps and 5 at 25 are 9600 samples; read backwards from source frame 1, both start at sample 9600.
	sourceAt := func(fps timeline.Rational, frames int) (*timeline.Document, AudioSource) {
		d := previewDoc()
		d.Settings.FPS = fps
		it := soundItem("itm_a", 0, hp)
		it.Duration, it.SourceIn, it.Reverse = frames, 1, true
		put(d, it)

		src, ok := PreviewSource(d, "itm_a", 0, frames)
		if !ok {
			t.Fatal("no source")
		}

		return d, src
	}

	d30, s30 := sourceAt(timeline.Rational{Num: 30, Den: 1}, 6)
	d25, s25 := sourceAt(timeline.Rational{Num: 25, Den: 1}, 5)

	if s30.SourceStart != s25.SourceStart || s30.Samples != s25.Samples {
		t.Fatalf("the sources differ in samples: %+v and %+v", s30, s25)
	}

	if PreviewKey(d30, &s30) == PreviewKey(d25, &s25) {
		t.Error("a reversed source at 30 and at 25 fps has one key")
	}
}
