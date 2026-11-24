package command

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	tx "github.com/AlexGalhardo/stuarty/core/transcript"
)

// Acceptance criteria 1 and 2 of epic 69 on the timelines that are easy to get wrong: a changed speed, a clip already
// cut, a clip used twice and its sound on another track. The words of the range lose every range, every other word
// stays on the timeline, and the inverse restores the document exactly.
func TestTranscriptDeleteRangeOnHardTimelines(t *testing.T) {
	words := []tx.Word{
		{Text: "one", Start: 62, End: 70},
		{Text: "two", Start: 72, End: 80},
		{Text: "three", Start: 90, End: 110},
		{Text: "four", Start: 120, End: 140},
		{Text: "five", Start: 150, End: 170},
	}
	resolved := make([]tword, len(words))
	for i, w := range words {
		resolved[i] = tword{w.Text, w.Start, w.End}
	}

	cases := []struct {
		name     string
		doc      func() *B
		from, to int
	}{
		{"speed 3/2", func() *B {
			return base().clip(itemA, vidTr, 30, 100, 60, func(it *timeline.Item) {
				it.Speed = timeline.Rational{Num: 3, Den: 2}
			}).text(itemText, vidTr, 130, 40)
		}, 1, 2},
		{"speed 1/2", func() *B {
			return base().clip(itemA, vidTr, 30, 240, 60, func(it *timeline.Item) {
				it.Speed = timeline.Rational{Num: 1, Den: 2}
			})
		}, 2, 2},
		{"a clip already cut, the range across the cut", func() *B {
			return base().clip(itemA, vidTr, 0, 40, 60).clip(itemB, vidTr, 40, 80, 100)
		}, 1, 3},
		{"a clip used twice with its sound", func() *B {
			b := base().clip(itemA, vidTr, 0, 120, 60).clip(itemB, vidTr, 200, 120, 60)
			return b.audio(itemC, 0, 120, 60, func(it *timeline.Item) { it.Asset = vidAsset })
		}, 1, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := c.doc().d
			before := tx.Map(doc, vidAsset, words)
			checkSnapOffIsEpic69(t, doc, resolved, c.from, c.to)

			n := 0
			env := Env{NewIDs: func(prefix string) string {
				n++
				return fmt.Sprintf("%s_%010d", prefix, n)
			}}
			args := fmt.Sprintf(`{"asset_id": %q, "from_word": %d, "to_word": %d, %s}`,
				vidAsset, c.from, c.to, resolvedJSON(resolved))

			res, err := Default().Apply(doc, Operation{Op: "transcript.delete_range", Args: json.RawMessage(args)}, env)
			if err != nil {
				t.Fatal(err)
			}

			if vs := timeline.Validate(res.Doc); len(vs) > 0 {
				t.Fatalf("invalid document: %v", vs)
			}

			after := tx.Map(res.Doc, vidAsset, words)
			for i := range words {
				inRange := i >= c.from && i <= c.to
				if inRange && len(after.Words[i].Ranges) != 0 {
					t.Errorf("word %d was removed and still has %v", i, after.Words[i].Ranges)
				}

				if !inRange && len(after.Words[i].Ranges) != len(before.Words[i].Ranges) {
					t.Errorf("word %d had %d ranges and now has %d", i,
						len(before.Words[i].Ranges), len(after.Words[i].Ranges))
				}
			}

			back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
			if len(skipped) != 0 {
				t.Fatalf("the inverse skipped %v", skipped)
			}

			if !reflect.DeepEqual(tx.Map(back.Doc, vidAsset, words), before) {
				t.Error("the inverse did not restore the words")
			}

			if !reflect.DeepEqual(back.Doc.Items, doc.Items) {
				t.Error("the inverse did not restore the items")
			}
		})
	}
}
