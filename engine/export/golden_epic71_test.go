package export

import (
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
