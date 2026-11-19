package image

import (
	"bytes"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

func planOf(d *Document) plan.FramePlan {
	return Plan(d, plan.Options{Measure: testMeasure, OutputScale: 1})
}

// TestPlanGolden holds the plan of the base document: what the renderer is given for it.
func TestPlanGolden(t *testing.T) {
	compareFile(t, "testdata/plan", "base.json", pretty(t, planOf(baseDoc())))
}

func TestPlanFlattensTheTree(t *testing.T) {
	fp := planOf(baseDoc())

	var items []string
	for _, l := range fp.Layers {
		items = append(items, l.ItemID)
	}

	// the group is gone: its children come in its place, bottom first, with its offset and its opacity
	want := []string{lid('a'), lid('b'), lid('c'), lid('d'), lid('f'), lid('g'), lid('h')}
	if len(items) != len(want) {
		t.Fatalf("layers %v, want %v", items, want)
	}

	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("layers %v, want %v", items, want)
		}
	}

	dot := fp.Layers[4]
	if dot.CX != 100 || dot.CY != 90 || dot.Opacity != 0.8 || dot.W != 100 {
		t.Errorf("the dot is in the group at 50,40 with its opacity of 80: got %+v", dot)
	}
}

func TestPlanLeavesOutWhatIsHiddenOrEmpty(t *testing.T) {
	d := baseDoc()
	d.Layers[4].Visible = false // the group
	d.Layers[3].Visible = false
	d.Layers[1].Asset = "" // an image slot with no file
	d.Assets = nil

	fp := planOf(d)
	if len(fp.Layers) != 3 { // fill, title, frame
		t.Fatalf("got %d layers: %+v", len(fp.Layers), fp.Layers)
	}
}

func TestPlanOfAnImageAndOfShapes(t *testing.T) {
	d := baseDoc()
	d.Layers[1].Crop = &Crop{X: 200, Y: 0, W: 900, H: 900}
	d.Layers[3].Shape, d.Layers[3].Radius, d.Layers[3].Rotation = ShapeRoundedRect, 12, -5

	fp := planOf(d)

	img := fp.Layers[1]
	if img.Kind != plan.Media || img.AssetID != hashFace || img.AssetKind != "image" ||
		img.Crop != (plan.Rect{X: 200, Y: 0, W: 900, H: 900}) {
		t.Errorf("the image layer is wrong: %+v", img)
	}

	bar := fp.Layers[3]
	if bar.Kind != plan.Bitmap || bar.Bitmap.Shape.Shape != ShapeRoundedRect || bar.Bitmap.Shape.Radius != 12 ||
		bar.Rotation != -5 || bar.CX != 400 || bar.CY != 520 {
		t.Errorf("the shape layer is wrong: %+v", bar)
	}
}

func TestPlanBackground(t *testing.T) {
	d := baseDoc()
	d.Background = "#10182080"

	if fp := planOf(d); fp.Background != "#101820" || fp.Transparent {
		t.Errorf("an alpha in the background is ignored: %+v", fp)
	}

	d.Background = Transparent
	if fp := planOf(d); !fp.Transparent {
		t.Error("a transparent background is transparent")
	}
}

func TestPlanPlacesTheTextBlockByItsAlign(t *testing.T) {
	d := baseDoc()
	title, _ := Find(d, lid('c'))

	title.Style.Align = new("left")
	left := planOf(d).Layers[2]

	title.Style.Align = new("right")
	right := planOf(d).Layers[2]

	title.Style.Align = new("center")
	centre := planOf(d).Layers[2]

	if !(left.CX < centre.CX && centre.CX < right.CX) {
		t.Errorf("left %v, centre %v, right %v: the block moves with the align", left.CX, centre.CX, right.CX)
	}

	if centre.CX != 620 { // the box is 120 to 1120
		t.Errorf("a centred block is on the centre of the box: %v", centre.CX)
	}

	if centre.Bitmap.Style.MaxWidth != 1000 || centre.Bitmap.Kind != "text" || centre.Bitmap.Text != "HOW IT WORKS" {
		t.Errorf("the bitmap wraps at the width of the box: %+v", centre.Bitmap)
	}
}

func TestPlanIsTheSameTwice(t *testing.T) {
	a, b := pretty(t, planOf(baseDoc())), pretty(t, planOf(baseDoc()))
	if !bytes.Equal(a, b) {
		t.Fatal("the same document gave two plans")
	}
}
