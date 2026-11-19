package image

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func lintOf(d *Document) []lint.Finding { return Lint(d, LintEnv{Measure: testMeasure}) }

func codes(fs []lint.Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Code+" "+f.Item)
	}

	return out
}

func TestLintCleanDocument(t *testing.T) {
	d := baseDoc()
	d.Layers = d.Layers[:3] // fill, photo, title: an image under the text is not judged

	if fs := lintOf(d); len(fs) != 0 {
		t.Fatalf("a clean document has no findings: %v", codes(fs))
	}
}

func TestLintEmptySlotBlocksAnExport(t *testing.T) {
	fs := lintOf(templateDoc())
	if len(fs) == 0 || fs[0].Code != CodeEmptySlot || fs[0].Severity != "error" || !Blocking(fs[0].Code) {
		t.Fatalf("an empty slot is the first finding and an error that blocks: %v", codes(fs))
	}

	for _, f := range fs {
		if f.Hint == "" || f.Message == "" {
			t.Errorf("a finding explains itself: %+v", f)
		}
	}
}

func TestLintTextOutside(t *testing.T) {
	d := baseDoc()
	title, _ := Find(d, lid('c'))

	title.X = 700 // 700 + 1000 is past 1280
	if !hasCode(lintOf(d), CodeTextOutside) {
		t.Error("a text past the right edge is reported")
	}

	title.X = 120
	if hasCode(lintOf(d), CodeTextOutside) {
		t.Error("a text inside is not reported")
	}

	title.X, title.Y, title.W, title.H, title.Rotation = 540, 300, 200, 40, 90
	if hasCode(lintOf(d), CodeTextOutside) {
		t.Error("a rotated text that still fits is not reported")
	}

	title.Y, title.W = 10, 500 // turned 90 degrees it is 500 high around y 30
	if !hasCode(lintOf(d), CodeTextOutside) {
		t.Error("the rotation counts")
	}

	title.Visible = false
	if hasCode(lintOf(d), CodeTextOutside) {
		t.Error("a hidden layer is not linted")
	}
}

func TestLintTextSmallUsesThePreset(t *testing.T) {
	d := baseDoc() // video_thumbnail: 36
	title, _ := Find(d, lid('c'))
	title.Style.Size = new(30.0)

	if !hasCode(lintOf(d), CodeTextSmall) {
		t.Error("30 is under 36 on a thumbnail")
	}

	d.Preset = "" // no preset: 24
	if hasCode(lintOf(d), CodeTextSmall) {
		t.Error("30 is over 24 with no preset")
	}

	title.Style.Size = new(20.0)
	if !hasCode(lintOf(d), CodeTextSmall) {
		t.Error("20 is under 24")
	}
}

func TestLintLowContrast(t *testing.T) {
	d := baseDoc()
	d.Layers = []Layer{d.Layers[0], d.Layers[2]} // the dark fill and the title
	ReorderAll(d)

	title, _ := Find(d, lid('c'))
	title.Style.Color = new(timeline.Color("#111820"))

	if !hasCode(lintOf(d), CodeLowContrast) {
		t.Error("dark text on a dark fill is reported")
	}

	title.Style.Color = new(timeline.Color("#FFE600"))
	if hasCode(lintOf(d), CodeLowContrast) {
		t.Error("yellow on a dark fill reads well")
	}

	// a plate under the text decides, not the background
	d.Layers = []Layer{d.Layers[0], {
		ID: lid('d'), Kind: KindShape, Name: "Plate", Visible: true, Opacity: 100, W: 1280, H: 720,
		Shape: ShapeRect, Fill: "#FFFF00",
	}, d.Layers[1]}
	ReorderAll(d)

	if !hasCode(lintOf(d), CodeLowContrast) {
		t.Error("yellow text on a yellow plate is reported")
	}

	d.Background = Transparent
	d.Layers = d.Layers[2:]
	ReorderAll(d)

	if hasCode(lintOf(d), CodeLowContrast) {
		t.Error("nothing is known under the text, so nothing is said")
	}
}

func TestLintUpscaledImage(t *testing.T) {
	d := baseDoc()
	img, _ := Find(d, lid('b'))
	img.Crop = &Crop{X: 0, Y: 0, W: 400, H: 225} // 1280 over 400 is more than 2 times

	if !hasCode(lintOf(d), CodeUpscaled) {
		t.Error("an image drawn at more than twice its source is reported")
	}

	img.Crop = nil
	if hasCode(lintOf(d), CodeUpscaled) {
		t.Error("an image drawn smaller than its source is not")
	}
}

func TestLintOrderIsStable(t *testing.T) {
	d := templateDoc()
	title, _ := Find(d, lid('c'))
	title.X, title.Style.Size = 900, new(10.0)

	a, b := codes(lintOf(d)), codes(lintOf(d))
	if len(a) < 3 {
		t.Fatalf("expected several findings, got %v", a)
	}

	for i := range a {
		if a[i] != b[i] {
			t.Fatal("two runs gave two orders")
		}
	}

	if a[0][:len(CodeEmptySlot)] != CodeEmptySlot {
		t.Errorf("errors come first: %v", a)
	}
}
