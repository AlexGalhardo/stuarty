package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The golden cases of epic 71 (SDD-002 C.88 and C.90), in testdata/golden-epic71 with the layout of the
// cases of golden_alpha_test.go: one folder for each caption style, and one for the blurred region. They are
// kept apart from testdata/golden, which the preview-against-export tests of the editor read as one list.
// Regenerate with go test ./engine/export -run Epic71 -update; every regeneration is a reviewed change.

func epic71Dir(name string) string { return filepath.Join("testdata", "golden-epic71", name) }

// captionStyleCase is a caption over a backdrop clip, drawn with the track style that caption.apply_style
// stores for the style; one frame inside each of its three words.
func captionStyleCase(id string) alphaCase {
	spec, _ := command.CaptionStyleByID(id)
	b := newCase("caption style "+id, rate(30, 1))
	b.backdrop(60)
	track := b.track()
	tr := b.doc.Tracks[track]
	tr.CaptionStyle = command.ResolveCaptionStyle(spec, goldenW, goldenH)
	b.doc.Tracks[track] = tr
	b.add(timeline.ItemCaption, track, 0, 60, func(it *timeline.Item) {
		it.Words = []timeline.Word{
			{Text: "Meet", Start: 0, Duration: 20},
			{Text: "the", Start: 20, Duration: 20},
			{Text: "editor", Start: 40, Duration: 20},
		}
	})

	return b.build(10, 30, 50)
}

func epic71Cases() map[string]func() alphaCase {
	cases := map[string]func() alphaCase{}
	for _, id := range command.CaptionStyleIDs() {
		cases["caption-style-"+id] = func() alphaCase { return captionStyleCase(id) }
	}

	return cases
}

// Acceptance criterion 1 of epic 71: every style gives its golden frames, in one and in eight bands.
func TestGoldenEpic71Cases(t *testing.T) {
	setup(t)

	for name, build := range epic71Cases() {
		t.Run(name, func(t *testing.T) {
			dir := epic71Dir(name)
			if *update {
				writeCaseAt(t, dir, name, build())
			}

			doc := loadDir(t, dir)
			frames := readFrames(t, dir)
			whole := renderDoc(t, doc, name, 0, frames)
			banded := renderDoc(t, doc, name, 8, frames)

			for i, pix := range whole {
				want := readPNG(t, framePNG(dir, frames[i]))
				if got := sum(pix); got != sum(want) {
					t.Errorf("frame %d: %s, reference %s", frames[i], got, sum(want))
					saveMismatch(t, name, frames[i], doc.Settings.Width, doc.Settings.Height, pix)
				}

				if sum(banded[i]) != sum(pix) {
					t.Errorf("frame %d differs between 1 band and 8 bands", frames[i])
				}
			}
		})
	}
}

// Two styles never draw the same frame: a style that looks like another is a copy, not a style.
func TestEpic71CaptionStylesLookDifferent(t *testing.T) {
	setup(t)

	seen := map[string]string{}

	for name := range epic71Cases() {
		dir := epic71Dir(name)
		h := sum(readPNG(t, framePNG(dir, 30)))

		if other, ok := seen[h]; ok {
			t.Errorf("%s and %s draw the same frame", name, other)
		}

		seen[h] = name
	}
}

// blurRegionCase is a portrait clip of 60 frames whose burned-in time code is blurred by clip.blur_region from
// frame 20 for 20 frames; the document is the one the command makes, drawn by the export compositor. The listed
// frames are the last one before the range, its first and last, and the first one after it.
func blurRegionCase() alphaCase {
	b := newCase("blurred region", rate(30, 1))
	b.backdrop(60)
	doc := b.doc

	// The ids of the case builder carry an i, which the command refuses in an id; the clip gets a valid one.
	const clipID = "itm_c000000001"
	var oldID string
	for id := range doc.Items {
		oldID = id
	}

	it := doc.Items[oldID]
	delete(doc.Items, oldID)
	it.ID = clipID
	doc.Items[clipID] = it

	args := fmt.Sprintf(
		`{"item_id": %q, "x": 0, "y": 0.02, "width": 0.95, "height": 0.1, "start": "20f", "duration": "20f",`+
			` "new_id": "itm_b000000001", "new_track_id": "trk_b000000001"}`,
		clipID,
	)
	res, err := command.Default().Apply(doc, command.Operation{Op: "clip.blur_region", Args: json.RawMessage(args)},
		command.Env{})
	if err != nil {
		panic(err.Error())
	}

	return alphaCase{doc: res.Doc, fixtures: b.fixtures, frames: []int{19, 20, 39, 40}}
}

// gradientEnergy is the sum of the horizontal differences of the red channel inside a rectangle: sharp text has a
// lot of it, a blur very little.
func gradientEnergy(pix []byte, w, x0, y0, x1, y1 int) int {
	total := 0
	for y := y0; y < y1; y++ {
		for x := x0 + 1; x < x1; x++ {
			total += absInt(int(pix[(y*w+x)*4]) - int(pix[(y*w+x-1)*4]))
		}
	}

	return total
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

// Acceptance criterion 2 of epic 71: the pixels of the blurred region differ from the source and the text in them
// is gone; the pixels outside it, and every frame outside the range, are the source; the range is exact to the
// frame. The reference frames are the ones this export draws.
func TestEpic71BlurRegionExport(t *testing.T) {
	setup(t)

	const name = "blur-region"
	dir := epic71Dir(name)
	if *update {
		writeCaseAt(t, dir, name, blurRegionCase())
	}

	blurred := loadDir(t, dir)
	frames := readFrames(t, dir)

	plain := timeline.Clone(blurred)
	for id, it := range plain.Items {
		if it.Name == command.BlurItemName {
			delete(plain.Items, id)
		}
	}

	if len(plain.Items) != len(blurred.Items)-1 {
		t.Fatalf("the case has %d items, want the clip and one blur item", len(blurred.Items))
	}

	got := renderDoc(t, blurred, name+"-blurred", 0, frames)
	src := renderDoc(t, plain, name+"-source", 0, frames)
	w, h := blurred.Settings.Width, blurred.Settings.Height

	for i, f := range frames {
		want := readPNG(t, framePNG(dir, f))
		if sum(got[i]) != sum(want) {
			t.Errorf("frame %d: %s, reference %s", f, sum(got[i]), sum(want))
			saveMismatch(t, name, f, w, h, got[i])
		}

		inRange := f >= 20 && f < 40
		if !inRange {
			if !bytes.Equal(got[i], src[i]) {
				t.Errorf("frame %d is outside the range 20 to 39 and differs from the source", f)
			}

			continue
		}

		// Inside the region, on the burned-in text and away from the edge of the picture: the text is smeared.
		before, after := gradientEnergy(src[i], w, 6, 6, 92, 17), gradientEnergy(got[i], w, 6, 6, 92, 17)
		if before == 0 || after*5 > before {
			t.Errorf("frame %d: the gradient of the region went from %d to %d, want at most a fifth", f, before, after)
		}

		// Outside the region, with a margin of its soft edge: the source, byte for byte.
		for y := range h {
			for x := range w {
				if y >= 4 && y < 42 && x < 176 {
					continue
				}

				o := (y*w + x) * 4
				if !bytes.Equal(got[i][o:o+4], src[i][o:o+4]) {
					t.Fatalf("frame %d: pixel %d,%d is outside the region and differs from the source", f, x, y)
				}
			}
		}
	}
}
