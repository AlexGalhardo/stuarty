package command

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Merging the two parts of a split gives back the words, the timing and the style.
func TestSplitMergeRoundTrip(t *testing.T) {
	t.Parallel()

	for k := 1; k <= 3; k++ {
		t.Run(fmt.Sprintf("before_word_%d", k), func(t *testing.T) {
			t.Parallel()

			doc := meetWords(func(it *timeline.Item) {
				it.Style = styleOf(map[string]any{"size": 60})
				it.LinkedItem = itemA
			})().d
			orig := doc.Items[itemC]

			split, err := json.Marshal(map[string]any{"item_id": itemC, "before_word": k, "new_id": itemB})
			if err != nil {
				t.Fatal(err)
			}

			res, err := Default().Apply(doc, Operation{Op: "caption.split", Args: split}, Env{})
			if err != nil {
				t.Fatal(err)
			}

			merge, err := json.Marshal(map[string]any{"item_ids": []string{itemC, itemB}})
			if err != nil {
				t.Fatal(err)
			}

			res, err = Default().Apply(res.Doc, Operation{Op: "caption.merge", Args: merge}, Env{})
			if err != nil {
				t.Fatal(err)
			}

			got := res.Doc.Items[itemC]
			if !reflect.DeepEqual(got.Words, orig.Words) || got.Start != orig.Start || got.Duration != orig.Duration ||
				!reflect.DeepEqual(got.Style, orig.Style) || got.LinkedItem != orig.LinkedItem {
				t.Errorf("merged caption differs:\n got %+v\nwant %+v", got, orig)
			}

			if _, ok := res.Doc.Items[itemB]; ok {
				t.Error("the second part was not removed")
			}
		})
	}
}
