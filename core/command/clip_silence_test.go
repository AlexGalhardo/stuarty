package command

import (
	"fmt"
	"testing"
)

// An audio item (a voice-over, a podcast) is cut by its silences like a clip (VV-083),
// and a silence at the very start of the source range is removed too.
func TestRemoveSilencesAcceptsAudioItem(t *testing.T) {
	doc := base().audio(itemA, 0, 300, 0).d
	args := fmt.Sprintf(
		`{"item_id": %q, "padding": "0f", "new_ids": ["itm_s6t7v8w9x0"], "resolved": {"silences": [{"start": 0, "end": 30}, {"start": 100, "end": 160}]}}`,
		itemA,
	)
	res, err := Default().Apply(doc, op2("clip.remove_silences", args), Env{})
	if err != nil {
		t.Fatal(err)
	}
	first := res.Doc.Items[itemA]
	if first.Start != 0 || first.SourceIn != 30 || first.Duration != 70 {
		t.Errorf("first piece %+v", first)
	}
	if len(res.Doc.Items) != 2 {
		t.Errorf("want 2 items, got %d", len(res.Doc.Items))
	}
}
