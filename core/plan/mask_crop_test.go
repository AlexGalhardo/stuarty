package plan

import (
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func TestAnimatedMaskGivesEachFrameItsRectangle(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_a", trkBottom, 0, 10)
	it.Mask = &timeline.Mask{Shape: "rect", X: 0, Y: 0, Width: 0.5, Height: 1, Feather: 4}
	it.Keyframes = timeline.Keyframes{
		"mask_x":       {{T: 0, Value: 0, Easing: "linear"}, {T: 10, Value: 0.5, Easing: "linear"}},
		"mask_feather": {{T: 0, Value: 0, Easing: "linear"}, {T: 10, Value: 10, Easing: "linear"}},
	}
	put(d, it)

	m := Frame(d, 5).Layers[0].Mask
	if m == nil || m.X != 0.25 || m.Width != 0.5 || m.Feather != 5 || m.Shape != "rect" {
		t.Errorf("frame 5 mask %+v, want x 0.25, width 0.5, feather 5", m)
	}

	w := Window(d, 0, 10).Layers[0]
	if len(w.Masks) != 10 || w.Masks[0].X != 0 || w.Masks[9].X != 0.45 {
		t.Errorf("window masks %+v, want 10 entries from x 0 to 0.45", w.Masks)
	}

	for f := range 10 {
		if got, want := w.Masks[f], Frame(d, f).Layers[0].Mask; *got != *want {
			t.Errorf("frame %d: window mask %+v, frame mask %+v", f, got, want)
		}
	}
}

func TestStaticMaskAndCropCarryNoSeries(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_a", trkBottom, 0, 10)
	it.Mask = &timeline.Mask{Shape: "ellipse", X: 0.1, Y: 0.1, Width: 0.8, Height: 0.8}
	it.Crop = &timeline.Crop{Left: 0.1}
	put(d, it)

	w := Window(d, 0, 10).Layers[0]
	if w.Masks != nil || w.Crops != nil || w.Mask == nil || w.Crop == nil {
		t.Errorf("static mask and crop must stay constant: masks %v crops %v", w.Masks, w.Crops)
	}
}

func TestMaskKeyframesStayInsideTheItem(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_a", trkBottom, 0, 10)
	it.Mask = &timeline.Mask{Shape: "rect", X: 0, Y: 0, Width: 0.5, Height: 0.5}
	it.Keyframes = timeline.Keyframes{
		"mask_x": {{T: 0, Value: 0.9, Easing: "linear"}, {T: 10, Value: 0.9, Easing: "linear"}},
	}
	put(d, it)

	if m := Frame(d, 3).Layers[0].Mask; m.X+m.Width > 1.0001 {
		t.Errorf("mask %+v goes outside the item", m)
	}
}

func TestAnimatedCropGivesEachFrameItsRect(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_a", trkBottom, 0, 10)
	it.Keyframes = timeline.Keyframes{
		"crop_left": {{T: 0, Value: 0, Easing: "linear"}, {T: 10, Value: 0.5, Easing: "linear"}},
	}
	put(d, it)

	asset := d.Assets[it.Asset]
	sw := float64(asset.Width)

	if c := Frame(d, 0).Layers[0].Crop; c.X != 0 || c.W != sw {
		t.Errorf("frame 0 crop %+v, want the whole source", c)
	}

	if c := Frame(d, 5).Layers[0].Crop; c.X != round3(0.25*sw) || c.W != round3(0.75*sw) {
		t.Errorf("frame 5 crop %+v, want a quarter cut from the left", c)
	}

	w := Window(d, 0, 10).Layers[0]
	if len(w.Crops) != 10 {
		t.Fatalf("window crops %v, want 10 entries", w.Crops)
	}

	for f := range 10 {
		c := Frame(d, f).Layers[0].Crop
		if w.Crops[f] != [4]float64{c.X, c.Y, c.W, c.H} {
			t.Errorf("frame %d: window crop %v, frame crop %+v", f, w.Crops[f], c)
		}
	}
}

func TestShadowAndOutlineReachTheLayerAndTheWindow(t *testing.T) {
	t.Parallel()

	d := newDoc()
	it := clip("itm_a", trkBottom, 0, 10)
	it.Shadow = &timeline.Shadow{Color: "#112233", Opacity: 0.6, DX: 4.1234, DY: -3, Blur: 9}
	it.Outline = &timeline.Outline{Color: "#FFFFFF", Width: 5}
	put(d, it)

	l := Frame(d, 2).Layers[0]
	if l.Shadow == nil || *l.Shadow != (ShadowSpec{Color: "#112233", Opacity: 0.6, DX: 4.123, DY: -3, Blur: 9}) {
		t.Errorf("layer shadow %+v", l.Shadow)
	}

	if l.Outline == nil || *l.Outline != (OutlineSpec{Color: "#FFFFFF", Width: 5}) {
		t.Errorf("layer outline %+v", l.Outline)
	}

	if w := Window(d, 0, 10).Layers[0]; w.Shadow == nil || w.Outline == nil {
		t.Errorf("window lost the shadow or the outline: %+v %+v", w.Shadow, w.Outline)
	}

	plain := newDoc()
	put(plain, clip("itm_p", trkBottom, 0, 10))

	if l := Frame(plain, 0).Layers[0]; l.Shadow != nil || l.Outline != nil {
		t.Error("a clip with neither must give none")
	}
}
