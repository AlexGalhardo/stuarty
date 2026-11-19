package export

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// SDD-004 section 3.5, animated colour: the window of the preview carries color_matrices, and the renderers read
// one frame plan per frame. The two must name the same matrix at every frame, and the frames must differ, so the
// golden case draws a colour that really moves.
func TestColourKeyframesWindowEqualsFramePlans(t *testing.T) {
	c := colourKeyframesCase()
	w := plan.WindowWith(c.doc, 0, 30, plan.Options{})

	if len(w.Layers) != 1 || len(w.Layers[0].ColorMatrices) != 30 {
		t.Fatalf(
			"the window has %d layers and %d matrices, want 1 layer and 30 matrices",
			len(w.Layers),
			len(w.Layers[0].ColorMatrices),
		)
	}

	seen := map[string]bool{}

	for f := range 30 {
		fp := plan.Frame(c.doc, f)

		want := json.RawMessage("null")
		if m := fp.Layers[0].ColorMatrix; m != nil {
			raw, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}

			want = raw
		}

		got := w.Layers[0].ColorMatrices[f]
		if !bytes.Equal(got, want) {
			t.Errorf("frame %d: window matrix %s, frame plan matrix %s", f, got, want)
		}

		seen[string(got)] = true
	}

	if len(seen) < 25 {
		t.Errorf("only %d different matrices in 30 frames: the colour does not move", len(seen))
	}
}
