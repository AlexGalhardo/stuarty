package command

import (
	"encoding/json"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// RF-024: scale 1 at frame 0 and 1.2 at frame 30 with ease_in_out is 1.1 at frame 15
// (4u^3 at u = 0.5 is 0.5).
func TestKeyframeEaseInOutMiddle(t *testing.T) {
	t.Parallel()

	doc := base().text(itemC, vidTr, 30, 90).d

	for _, args := range []map[string]any{
		{"item_id": itemC, "property": "scale", "at": "30f", "value": 1, "easing": "ease_in_out"},
		{"item_id": itemC, "property": "scale", "at": "60f", "value": 1.2},
	} {
		raw, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}

		res, err := Default().Apply(doc, Operation{Op: "keyframe.set", Args: raw}, Env{})
		if err != nil {
			t.Fatal(err)
		}

		doc = res.Doc
	}

	keys := doc.Items[itemC].Keyframes["scale"]
	if got := timeline.ValueAt(keys, 15, 1); got != 1.1 {
		t.Errorf("ValueAt(15) = %v, want 1.1", got)
	}
}
