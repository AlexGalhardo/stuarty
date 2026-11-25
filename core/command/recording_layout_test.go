package command

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Epic 66 criterion 2: after a layout is applied, undo puts the previous framing back, for a clip that had a fit, a
// crop, a position and a mask of its own, and for the order of the tracks.
func TestApplyLayoutUndoRestoresThePreviousFraming(t *testing.T) {
	t.Parallel()

	doc := layoutDoc(1080, 1920, true, func(d *timeline.Document) {
		screen := d.Items[itemA]
		x, y := 100.0, 700.0
		screen.X, screen.Y, screen.Scale, screen.Fit = &x, &y, 0.4, "cover"
		screen.Crop = &timeline.Crop{Left: 0.1, Right: 0.2}
		screen.Mask = &timeline.Mask{Shape: "rect", X: 0, Y: 0, Width: 1, Height: 1}
		d.Items[itemA] = screen
	})().d

	before := timeline.Canonical(doc)
	args := layoutArgs("pip", withCamera(`"padding": 0.05`)...)

	res, err := Default().Apply(doc, Operation{Op: "recording.apply_layout", Args: json.RawMessage(args)}, Env{})
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(timeline.Canonical(res.Doc), before) {
		t.Fatal("the layout changed nothing")
	}

	moved := res.Doc.Items[itemA]
	if moved.Fit != "contain" || moved.Crop != nil || moved.Mask != nil {
		t.Errorf("the screen clip after the layout: fit %q crop %v mask %v", moved.Fit, moved.Crop, moved.Mask)
	}

	back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
	if len(skipped) != 0 || !bytes.Equal(timeline.Canonical(back.Doc), before) {
		t.Errorf("undo did not restore the previous framing (skipped %v)", skipped)
	}

	restored := back.Doc.Items[itemA]
	if restored.Fit != "cover" || restored.Crop == nil || restored.Crop.Left != 0.1 || restored.Scale != 0.4 ||
		restored.Mask == nil {
		t.Errorf("the screen clip after undo: %+v", restored)
	}
}
