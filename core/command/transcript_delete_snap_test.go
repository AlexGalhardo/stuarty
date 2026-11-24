package command

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	tx "github.com/AlexGalhardo/stuarty/core/transcript"
)

// snapWords are words with a pause of 6 to 10 frames between neighbours, inside the 9 frame window of the default.
var snapWords = []tword{
	{"one", 62, 70}, {"two", 78, 86}, {"three", 96, 110}, {"four", 118, 130}, {"five", 140, 150},
}

// snapSilences lie in the pauses but not exactly at the whisper edges: after each word the silence starts 2 frames
// late and ends 3 frames before the next word.
var snapSilences = [][2]int{{72, 75}, {88, 93}, {112, 115}, {132, 137}}

func snapArgsJSON(from, to int, extra string) string {
	parts := make([]string, len(snapSilences))
	for i, s := range snapSilences {
		parts[i] = fmt.Sprintf(`{"start": %d, "end": %d}`, s[0], s[1])
	}
	words := resolvedJSON(snapWords)
	words = strings.Replace(words, `]}`, fmt.Sprintf(`], "silences": [%s], "silence_analysis": "read"}`,
		strings.Join(parts, ", ")), 1)
	return fmt.Sprintf(`{"asset_id": %q, "from_word": %d, "to_word": %d, %s%s}`, vidAsset, from, to, extra, words)
}

func applySnapDelete(t *testing.T, doc *timeline.Document, args string) Result {
	t.Helper()
	n := 0
	env := Env{NewIDs: func(prefix string) string {
		n++
		return fmt.Sprintf("%s_%010d", prefix, n)
	}}
	res, err := Default().Apply(doc, Operation{Op: "transcript.delete_range", Args: json.RawMessage(args)}, env)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// Acceptance criterion 1 of epic 89: with the snap on, each edge of the removed source range lies in a silence.
func TestTranscriptDeleteSnapEdgesLieInSilences(t *testing.T) {
	for _, c := range []struct{ from, to int }{{1, 1}, {2, 2}, {1, 2}, {1, 3}} {
		t.Run(fmt.Sprintf("words %d to %d", c.from, c.to), func(t *testing.T) {
			doc := base().clip(itemA, vidTr, 30, 300, 60).d
			res := applySnapDelete(t, doc, snapArgsJSON(c.from, c.to, ""))
			rng, ok := res.Effective["range"].(map[string]any)
			if !ok {
				t.Fatalf("effective.range is %v", res.Effective["range"])
			}
			start, end := rng["start"].(int), rng["end"].(int)
			for name, edge := range map[string]int{"start": start, "end": end} {
				in := false
				for _, s := range snapSilences {
					in = in || s[0] <= edge && edge <= s[1]
				}
				if !in {
					t.Errorf("the %s edge %d is in no silence %v", name, edge, snapSilences)
				}
			}
			if start > snapWords[c.from].start || end < snapWords[c.to].end {
				t.Errorf("the cut [%d,%d) lost part of the chosen words", start, end)
			}
		})
	}
}

// Acceptance criterion 2 of epic 89: with snap "0s" the result is the result of epic 69, whatever silences are
// supplied.
func TestTranscriptDeleteSnapOffIsTheCutOfEpic69(t *testing.T) {
	layouts := map[string]func() *B{
		"speed 1": func() *B { return base().clip(itemA, vidTr, 30, 300, 60).text(itemText, vidTr, 330, 40) },
		"speed 3/2": func() *B {
			return base().clip(itemA, vidTr, 30, 200, 60, func(it *timeline.Item) {
				it.Speed = timeline.Rational{Num: 3, Den: 2}
			})
		},
		"a clip and its sound": func() *B {
			b := base().clip(itemA, vidTr, 30, 300, 60)
			return b.audio(itemB, 30, 300, 60, func(it *timeline.Item) { it.Asset = vidAsset })
		},
	}
	for name, layout := range layouts {
		for _, c := range []struct{ from, to int }{{0, 0}, {1, 1}, {2, 3}, {4, 4}} {
			t.Run(fmt.Sprintf("%s words %d to %d", name, c.from, c.to), func(t *testing.T) {
				off := applySnapDelete(t, layout().d, snapArgsJSON(c.from, c.to, `"snap": "0s", `))
				// The epic 69 call: the words only.
				old := applySnapDelete(
					t,
					layout().d,
					fmt.Sprintf(`{"asset_id": %q, "from_word": %d, "to_word": %d, %s}`,
						vidAsset, c.from, c.to, resolvedJSON(snapWords)),
				)
				if !reflect.DeepEqual(off, old) {
					t.Errorf("snap 0s differs from epic 69:\n%+v\n%+v", off, old)
				}
			})
		}
	}
}

// checkSnapOffIsEpic69 is acceptance criterion 2 of epic 89 on one case of the epic 69 tests: the same cut with
// `snap` "0s" and silences that the default window would snap to gives the same result, or the same error, as the
// call of epic 69 with the words only.
func checkSnapOffIsEpic69(t *testing.T, doc *timeline.Document, words []tword, from, to int) {
	t.Helper()
	// A silence across the end of each word that has a gap of 2 frames or more after it: the start edge of the
	// next word would move to it with the snap on.
	var parts []string
	for i := 1; i < len(words); i++ {
		if words[i].start-words[i-1].end >= 2 {
			parts = append(parts, fmt.Sprintf(`{"start": %d, "end": %d}`, words[i-1].end-1, words[i-1].end+1))
		}
	}
	snapped := strings.Replace(resolvedJSON(words), `]}`,
		fmt.Sprintf(`], "silences": [%s], "silence_analysis": "read"}`, strings.Join(parts, ", ")), 1)
	apply := func(args string) (Result, error) {
		n := 0
		env := Env{NewIDs: func(prefix string) string {
			n++
			return fmt.Sprintf("%s_%010d", prefix, n)
		}}
		return Default().Apply(doc, Operation{Op: "transcript.delete_range", Args: json.RawMessage(args)}, env)
	}
	old, oldErr := apply(fmt.Sprintf(`{"asset_id": %q, "from_word": %d, "to_word": %d, %s}`,
		vidAsset, from, to, resolvedJSON(words)))
	off, offErr := apply(fmt.Sprintf(`{"asset_id": %q, "from_word": %d, "to_word": %d, "snap": "0s", %s}`,
		vidAsset, from, to, snapped))
	if !reflect.DeepEqual(oldErr, offErr) || !reflect.DeepEqual(old, off) {
		t.Errorf("words %d-%d: snap 0s differs from epic 69: %v / %v", from, to, offErr, oldErr)
	}
}

// Acceptance criterion 6 of epic 89: undo of a snapped cut restores the document.
func TestTranscriptDeleteSnapUndo(t *testing.T) {
	doc := base().clip(itemA, vidTr, 30, 300, 60).audio(itemB, 30, 300, 60,
		func(it *timeline.Item) { it.Asset = vidAsset }).text(itemText, vidTr, 330, 40).d
	words := make([]tx.Word, len(snapWords))
	for i, w := range snapWords {
		words[i] = tx.Word{Text: w.text, Start: w.start, End: w.end}
	}
	before := tx.Map(doc, vidAsset, words)
	res := applySnapDelete(t, doc, snapArgsJSON(1, 2, ""))
	if rng := res.Effective["range"].(map[string]any); rng["start"] == snapWords[1].start {
		t.Fatalf("the cut did not snap: %v", rng)
	}
	back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
	if len(skipped) != 0 {
		t.Fatalf("the inverse skipped %v", skipped)
	}
	if !reflect.DeepEqual(back.Doc, doc) {
		t.Error("the inverse did not restore the document")
	}
	if !reflect.DeepEqual(tx.Map(back.Doc, vidAsset, words), before) {
		t.Error("the inverse did not restore the words")
	}
}
