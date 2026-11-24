package command

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	tx "github.com/AlexGalhardo/stuarty/core/transcript"
)

// Acceptance criterion 1 of epic 69 at every speed: each item loses the part of the range it shows, in timeline
// frames, within one frame, and undo restores the document. The bound is checked in exact integers: with speed n/d,
// a source range of length L lasts L x d / n frames, so |lost x n - L x d| <= n.
func TestTranscriptDeleteRangeLosesTheRangeWithinOneFrame(t *testing.T) {
	speeds := []timeline.Rational{
		{Num: 1, Den: 1},
		{Num: 1, Den: 2},
		{Num: 3, Den: 2},
		{Num: 2, Den: 1},
		{Num: 4, Den: 3},
		{Num: 5, Den: 4},
		{Num: 7, Den: 3},
		{Num: 2, Den: 3},
	}
	// Words of different lengths, so that the edges of the ranges land on every remainder of the ratios.
	var words []tword
	at := 60
	for i := range 24 {
		length := 3 + i%5
		words = append(words, tword{fmt.Sprintf("w%d", i), at, at + length})
		at += length + i%3
	}
	layouts := map[string]func(*B, func(*timeline.Item)){
		"one clip": func(b *B, sp func(*timeline.Item)) { b.clip(itemA, vidTr, 30, 97, 60, sp) },
		"two pieces of the asset on a track": func(b *B, sp func(*timeline.Item)) {
			b.clip(itemA, vidTr, 0, 23, 60, sp).clip(itemB, vidTr, 23, 41, 95, sp)
		},
		"a clip and its sound": func(b *B, sp func(*timeline.Item)) {
			b.clip(itemA, vidTr, 10, 61, 62, sp)
			b.audio(itemB, 10, 61, 62, func(it *timeline.Item) { it.Asset = vidAsset; sp(it) })
		},
	}

	for name, layout := range layouts {
		for _, sp := range speeds {
			t.Run(fmt.Sprintf("%s at %d over %d", name, sp.Num, sp.Den), func(t *testing.T) {
				b := base()
				layout(b, func(it *timeline.Item) { it.Speed = sp })
				doc := b.d
				for from := range words {
					for to := from; to < min(from+6, len(words)); to++ {
						checkCutBound(t, doc, words, from, to, sp)
					}
				}
			})
		}
	}
}

// startsBeforeTheLastFrames: the piece after a cut starts at the first source frame after the range, so at a speed
// above 1 its end can read up to two timeline frames less of the source. Only a word that starts there may leave.
func startsBeforeTheLastFrames(doc *timeline.Document, ranges []tx.Range) bool {
	for _, r := range ranges {
		it := doc.Items[r.ItemID]
		if r.Start < it.Start+it.Duration-2 {
			return true
		}
	}

	return false
}

func checkCutBound(t *testing.T, doc *timeline.Document, words []tword, from, to int, sp timeline.Rational) {
	t.Helper()
	checkSnapOffIsEpic69(t, doc, words, from, to)

	n := 0
	env := Env{NewIDs: func(prefix string) string {
		n++
		return fmt.Sprintf("%s_%010d", prefix, n)
	}}
	args := fmt.Sprintf(`{"asset_id": %q, "from_word": %d, "to_word": %d, %s}`,
		vidAsset, from, to, resolvedJSON(words))

	res, err := Default().Apply(doc, Operation{Op: "transcript.delete_range", Args: json.RawMessage(args)}, env)
	if err != nil {
		if AsError(err).Code != CodeInvalidState {
			t.Fatalf("words %d-%d: %v", from, to, err)
		}

		return // no item shows this range
	}

	if vs := timeline.Validate(res.Doc); len(vs) > 0 {
		t.Fatalf("words %d-%d: invalid document: %v", from, to, vs)
	}

	mapped := make([]tx.Word, len(words))
	for i, w := range words {
		mapped[i] = tx.Word{Text: w.text, Start: w.start, End: w.end}
	}

	was, now := tx.Map(doc, vidAsset, mapped), tx.Map(res.Doc, vidAsset, mapped)
	for i := range mapped {
		switch inRange := i >= from && i <= to; {
		case inRange && len(now.Words[i].Ranges) != 0:
			t.Errorf("words %d-%d: word %d was removed and still has %v", from, to, i, now.Words[i].Ranges)
		case !inRange && len(now.Words[i].Ranges) == 0 && startsBeforeTheLastFrames(doc, was.Words[i].Ranges):
			t.Errorf("words %d-%d: word %d left the timeline", from, to, i)
		}
	}

	if _, again := Default().Apply(
		res.Doc, Operation{Op: "transcript.delete_range", Args: json.RawMessage(args)}, env,
	); again == nil {
		t.Errorf("words %d-%d: a second delete of the same range found something to cut", from, to)
	}

	cuts, _ := res.Effective["cuts"].([]map[string]any)
	for _, cut := range cuts {
		it := doc.Items[cut["item_id"].(string)]
		lost, _ := cut["removed"].(int)
		a := max(words[from].start-it.SourceIn, 0)
		// The part of the range the item shows, times d: the visible source ends at duration x n / d.
		shown := min((words[to].end-it.SourceIn)*sp.Den, it.Duration*sp.Num) - a*sp.Den
		if diff := lost*sp.Num - shown; diff > sp.Num || diff < -sp.Num {
			t.Errorf("words %d-%d on %s: lost %d frames of a range of %d/%d frames",
				from, to, it.ID, lost, shown, sp.Num)
		}
	}

	back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
	if len(skipped) != 0 {
		t.Fatalf("words %d-%d: the inverse skipped %v", from, to, skipped)
	}

	if !reflect.DeepEqual(back.Doc.Items, doc.Items) {
		t.Errorf("words %d-%d: the inverse did not restore the items", from, to)
	}
}
