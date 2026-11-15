package command

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func duckDoc() *timeline.Document {
	return base().clip(itemA, vidTr, 0, 300, 0).audio(itemB, 0, 300, 0).d
}

const duckOp = `{"item_id": "` + itemB + `", "speech_item_ids": ["` + itemA + `"],
	"resolved": {"speech": [{"item_id": "` + itemA + `", "ranges": [{"start": 60, "end": 120}]}]}}`

func applyBeta(t *testing.T, doc *timeline.Document, name, args string) *timeline.Document {
	t.Helper()
	res, err := BetaBuild().Apply(doc, Operation{Op: name, Args: json.RawMessage(args)}, Env{})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res.Doc
}

// RF-043: with speech from 2 s to 4 s at 30 fps, the music is at its ducked gain at
// 3 s and at the original gain at 0 s and 6 s.
func TestDuckGainAtFrames(t *testing.T) {
	t.Parallel()

	doc := applyBeta(t, duckDoc(), "audio.duck", duckOp)
	it := doc.Items[itemB]
	keys := it.Keyframes["volume"]

	for _, c := range []struct {
		frame int
		want  float64
	}{{0, 1}, {90, 0.251}, {180, 1}} {
		if got := timeline.ValueAt(keys, c.frame, it.Volume); got != c.want {
			t.Errorf("volume at frame %d = %v, want %v", c.frame, got, c.want)
		}
	}
}

// Ducking and then keyframe.clear on volume gives back the original document.
func TestDuckThenClearRestoresTheDocument(t *testing.T) {
	t.Parallel()

	orig := duckDoc()
	ducked := applyBeta(t, orig, "audio.duck", duckOp)

	if bytes.Equal(timeline.Canonical(ducked), timeline.Canonical(orig)) {
		t.Fatal("ducking changed nothing")
	}

	cleared := applyBeta(t, ducked, "keyframe.clear", `{"item_id": "`+itemB+`", "property": "volume"}`)
	if !bytes.Equal(timeline.Canonical(cleared), timeline.Canonical(orig)) {
		t.Errorf(
			"keyframe.clear did not restore the document:\n%s\nwant\n%s",
			timeline.Canonical(cleared),
			timeline.Canonical(orig),
		)
	}
}

// The keyframes of a ducked item are valid for every layout of speech: ramps stay
// inside the item and times strictly increase.
func TestDuckKeyframesAreValid(t *testing.T) {
	t.Parallel()

	for _, attack := range []int{0, 1, 6, 40} {
		for _, release := range []int{0, 1, 15, 60} {
			ranges := []SilenceRange{{0, 3}, {5, 9}, {30, 31}, {32, 60}, {290, 300}}
			keys := duckKeyframes(ranges, 300, attack, release, 1, 0.25)

			for i, k := range keys {
				if k.T < 0 || k.T > 300 || (i > 0 && k.T <= keys[i-1].T) {
					t.Fatalf(
						"attack %d release %d: keyframe %d at t=%d breaks the order or the range: %v",
						attack,
						release,
						i,
						k.T,
						keys,
					)
				}
			}
		}
	}
}
