package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// Golden cases of layer.replace_source (SDD-021 section 3.2, epic 30).

// hashFaceB has the size of hashFace, hashWide the proportions of the box of the photo at another size. They are
// made of pairs no other case uses, so a case that wants an unknown file still has one.
var (
	hashFaceB = "sha256:" + strings.Repeat("1a", 32)
	hashWide  = "sha256:" + strings.Repeat("2b", 32)
	hashTall  = "sha256:" + strings.Repeat("3c", 32)
)

func init() {
	testAssets[hashFaceB] = AssetInfo{Name: "face-cut.png", Width: 1600, Height: 900}
	testAssets[hashWide] = AssetInfo{Name: "face-small.png", Width: 800, Height: 450}
	testAssets[hashTall] = AssetInfo{Name: "tall.png", Width: 300, Height: 900}

	photo := lid('b')
	arg := func(id, hash string) string { return fmt.Sprintf(`{"layer_id": %q, "asset": %q}`, id, hash) }

	// withTunedPhoto has the photo moved, turned and cropped, with a look, so the cases show what stays.
	withTunedPhoto := func() *Document {
		d := baseDoc()
		l := &d.Layers[1]
		l.X, l.Y, l.W, l.H, l.Rotation, l.Opacity = 100, 60, 640, 360, 15, 70
		l.Crop = &Crop{X: 0, Y: 0, W: 800, H: 450}
		l.Adjust = &Adjust{Brightness: 10}
		l.Filters = []Filter{NewFilter(FilterBlur)}
		l.Mask = &Mask{Kind: MaskEllipse, X: 100, Y: 100, W: 800, H: 800}
		l.Blend = "multiply"
		l.Slot = "photo"

		return d
	}

	withEmptySlot := func() *Document {
		d := baseDoc()
		d.Assets = nil
		d.Layers[1].Asset, d.Layers[1].Slot = "", "photo"

		return d
	}

	withLockedPhoto := func() *Document {
		d := baseDoc()
		d.Layers[1].Locked = true

		return d
	}

	// withFullTable has the photo and 64 other layers, each with an image of its own: the table can take the new
	// file only when the old one leaves it, and it does not leave room for one more.
	withFullTable := func() *Document {
		d := baseDoc()

		for i := range MaxAssets {
			h := fmt.Sprintf("sha256:%064x", i+1)
			d.Assets[h] = AssetRef{Hash: h, Name: "n.png", Width: 10, Height: 10}
			d.Layers = append(d.Layers, Layer{
				ID: "lyr_" + fmt.Sprintf(
					"%010d",
					i,
				),
				Kind:    KindImage,
				Name:    "n",
				Visible: true,
				Opacity: 100,
				W:       10,
				H:       10,
				Asset:   h,
			})
		}

		return d
	}

	addCases(
		// 1600x900 at the box 1280x720: the proportions of the box, a file of another size.
		gc("layer.replace_source", "same_proportions", arg(photo, hashWide)),
		// the size of the old file: the box and the crop stay, the table holds the new file only.
		gc("layer.replace_source", "same_size_keeps_the_crop", arg(photo, hashFaceB)).on(withTunedPhoto),
		// 400x400 into the box 1280x720: the largest square inside it, centred.
		gc("layer.replace_source", "square_into_a_wide_box", arg(photo, hashLogo)),
		// 300x900 into a turned, cropped and tuned layer: everything else stays, the crop goes.
		gc("layer.replace_source", "tall_into_a_tuned_layer", arg(photo, hashTall)).on(withTunedPhoto),
		// the file of another layer stays in the table when the old one is still used.
		gc("layer.replace_source", "old_file_still_in_use", arg(photo, hashLogo)).on(func() *Document {
			d := baseDoc()
			d.Layers = append(d.Layers, Layer{
				ID: lid(
					'p',
				),
				Kind:    KindImage,
				Name:    "Copy",
				Visible: true,
				Opacity: 100,
				W:       100,
				H:       56,
				Asset:   hashFace,
			})

			return d
		}),
		gc("layer.replace_source", "fills_an_empty_slot", arg(photo, hashLogo)).on(withEmptySlot),
		gc("layer.replace_source", "not_a_hash", arg(photo, "face.png")),
		gc("layer.replace_source", "unknown_file", arg(photo, "sha256:"+strings.Repeat("9", 64))),
		gc("layer.replace_source", "no_such_layer", arg(lid('z'), hashLogo)),
		gc("layer.replace_source", "not_a_layer_id", arg("title", hashLogo)),
		gc("layer.replace_source", "a_text_layer", arg(lid('c'), hashLogo)),
		gc("layer.replace_source", "locked", arg(photo, hashLogo)).on(withLockedPhoto),
		gc("layer.replace_source", "too_many_images", arg(photo, hashLogo)).on(withFullTable),
	)
}

func replaceOnce(t *testing.T, d *Document, id, hash string) (*Document, BatchResult) {
	t.Helper()

	ReorderAll(d)

	res := mustApply(t, d, op("layer.replace_source", fmt.Sprintf(`{"layer_id": %q, "asset": %q}`, id, hash)))

	l, ok := Find(res.Doc, id)
	if !ok {
		t.Fatalf("the layer %s is gone", id)
	}

	_ = l

	return res.Doc, res
}

// TestReplaceSourceKeepsTheLayer is acceptance criterion 1 of epic 30 at the core: the same layer, the same
// place, a new file; everything else of the layer is the one it had.
func TestReplaceSourceKeepsTheLayer(t *testing.T) {
	before := baseDoc()
	was := before.Layers[1]

	got, res := replaceOnce(t, baseDoc(), lid('b'), hashWide)

	l, _ := Find(got, lid('b'))
	if l.ID != was.ID || l.Name != was.Name || l.X != was.X || l.Y != was.Y || l.W != was.W || l.H != was.H ||
		l.Opacity != was.Opacity || l.Rotation != was.Rotation {
		t.Fatalf("the layer changed: %+v, was %+v", l, was)
	}

	if l.Asset != hashWide {
		t.Fatalf("asset %s", l.Asset)
	}

	if _, ok := got.Assets[hashFace]; ok {
		t.Error("the old file is still in the table of images")
	}

	if got.Assets[hashWide].Width != 800 || got.Assets[hashWide].Height != 450 {
		t.Errorf("the table holds %+v", got.Assets[hashWide])
	}

	if res.Effective[0]["layer_id"] != lid('b') || res.Effective[0]["asset"] != hashWide {
		t.Errorf("effective %v", res.Effective[0])
	}

	// the other layers are untouched
	for i := range before.Layers {
		if i == 1 {
			continue
		}

		if !bytes.Equal(canonicalLayer(&before.Layers[i]), canonicalLayer(&got.Layers[i])) {
			t.Errorf("layer %d changed", i)
		}
	}
}

func canonicalLayer(l *Layer) []byte {
	b, err := json.Marshal(l)
	if err != nil {
		panic(err)
	}

	return b
}

// TestReplaceSourceNeverStretches: a file of other proportions is contained in the old box and centred on it.
func TestReplaceSourceNeverStretches(t *testing.T) {
	cases := []struct {
		name       string
		hash       string
		x, y, w, h int
	}{
		{"a square", hashLogo, 280, 0, 720, 720},
		{"a tall file", hashTall, 520, 0, 240, 720},
		{"the proportions of the box", hashWide, 0, 0, 1280, 720},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := replaceOnce(t, baseDoc(), lid('b'), c.hash)

			l, _ := Find(got, lid('b'))
			if l.X != c.x || l.Y != c.y || l.W != c.w || l.H != c.h {
				t.Fatalf("box %d,%d %dx%d, want %d,%d %dx%d", l.X, l.Y, l.W, l.H, c.x, c.y, c.w, c.h)
			}
		})
	}
}

// TestReplaceSourceCropRule: the crop stays for a file of the size of the old one and goes for any other.
func TestReplaceSourceCropRule(t *testing.T) {
	tuned := func() *Document {
		d := baseDoc()
		d.Layers[1].Crop = &Crop{X: 10, Y: 10, W: 800, H: 450}

		return d
	}

	got, _ := replaceOnce(t, tuned(), lid('b'), hashFaceB)
	if l, _ := Find(got, lid('b')); l.Crop == nil || l.Crop.W != 800 {
		t.Fatalf("the crop must stay for a file of the same size: %+v", l.Crop)
	}

	got, _ = replaceOnce(t, tuned(), lid('b'), hashWide)
	if l, _ := Find(got, lid('b')); l.Crop != nil {
		t.Fatalf("the crop must go for a file of another size: %+v", l.Crop)
	}
}

// TestReplaceSourceUndo: one batch, and the inverse puts back the file, the table, the box and the crop.
func TestReplaceSourceUndo(t *testing.T) {
	before := baseDoc()
	ReorderAll(before)

	res := mustApply(t, before, op("layer.replace_source", `{"layer_id": "`+lid('b')+`", "asset": "`+hashLogo+`"}`))

	undone, skipped := ApplyPrimitives(res.Doc, res.Inverse, nil)
	if len(skipped) > 0 {
		t.Fatalf("undo skipped %v", skipped)
	}

	if !bytes.Equal(Canonical(undone.Doc), Canonical(before)) {
		t.Fatal("undo did not restore the document")
	}
}

// TestReplaceSourceWithTheSameFileChangesNothing: no diff, so no revision.
func TestReplaceSourceWithTheSameFileChangesNothing(t *testing.T) {
	d := baseDoc()
	ReorderAll(d)

	res, err := ApplyBatch(d, []command.Operation{
		op("layer.replace_source", `{"layer_id": "`+lid('b')+`", "asset": "`+hashFace+`"}`),
	}, testEnv())
	if err != nil {
		t.Fatal(err)
	}

	if !res.NoOp() {
		t.Fatalf("a layer given its own file is not a change: %v", res.Diff)
	}
}

// TestReplaceSourceHonoursTheLimitOfImages: the old file leaves the table before the ceiling is judged.
func TestReplaceSourceHonoursTheLimitOfImages(t *testing.T) {
	d := baseDoc()

	for i := range MaxAssets - 1 {
		h := fmt.Sprintf("sha256:%064x", i+1)
		d.Assets[h] = AssetRef{Hash: h, Name: "n.png", Width: 10, Height: 10}
		d.Layers = append(d.Layers, Layer{
			ID: fmt.Sprintf(
				"lyr_%010d",
				i,
			),
			Kind:    KindImage,
			Name:    "n",
			Visible: true,
			Opacity: 100,
			W:       10,
			H:       10,
			Asset:   h,
		})
	}

	// 63 other files and the photo's own: replacing the photo's file gives 63 + 1 = 64, which fits.
	got, _ := replaceOnce(t, d, lid('b'), hashLogo)
	if len(got.Assets) != MaxAssets {
		t.Fatalf("the table holds %d files", len(got.Assets))
	}
}
