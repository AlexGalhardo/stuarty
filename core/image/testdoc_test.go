package image

import (
	"strings"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const (
	hashFace = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashLogo = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	docID    = "img_aaaaaaaaaa"
)

// lid is a layer id made of one letter, for readable fixtures.
func lid(c byte) string { return "lyr_" + strings.Repeat(string(c), 10) }

// testAssets are the files the test environment knows.
var testAssets = map[string]AssetInfo{
	hashFace: {Name: "face.png", Width: 1600, Height: 900},
	hashLogo: {Name: "logo.png", Width: 400, Height: 400},
}

// testMeasure is a deterministic text measurer: a character is half the size wide, a line is size times
// line height high, words wrap greedily at max_width.
func testMeasure(spec plan.BitmapSpec) (float64, float64) {
	size, lh, maxw := spec.Style.Size, spec.Style.LineHeight, spec.Style.MaxWidth
	cw := size / 2
	lines, widest := 0, 0.0

	for para := range strings.SplitSeq(spec.Text, "\n") {
		cur := 0.0
		lines++

		for i, word := range strings.Fields(para) {
			w := float64(len([]rune(word))) * cw
			if i > 0 && maxw > 0 && cur+cw+w > maxw {
				widest = max(widest, cur)
				lines++
				cur = w

				continue
			}

			if i > 0 {
				cur += cw
			}

			cur += w
		}

		widest = max(widest, cur)
	}

	return widest, float64(lines) * size * lh
}

func testEnv(ids ...string) Env {
	next := 0

	return Env{
		NewIDs: func(prefix string) string {
			if next >= len(ids) {
				panic("the test needs a generated " + prefix + " id; give it in the case")
			}

			next++

			return ids[next-1]
		},
		Asset: func(h string) (AssetInfo, bool) {
			a, ok := testAssets[h]

			return a, ok
		},
		Measure: testMeasure,
	}
}

// baseDoc is the document most golden cases start from.
func baseDoc() *Document {
	title := &timeline.CaptionStyle{}
	title.FontFamily, title.Size, title.Color = new("Anton"), new(120.0), new(timeline.Color("#FFE600"))

	d := &Document{
		SchemaVersion: SchemaVersion, ID: docID, Name: "Episode 12", Width: 1280, Height: 720,
		Background: "#101820", Preset: "video_thumbnail",
		Assets: map[string]AssetRef{hashFace: {Hash: hashFace, Name: "face.png", Width: 1600, Height: 900}},
		Layers: []Layer{
			{
				ID:      lid('a'),
				Kind:    KindFill,
				Name:    "Backdrop",
				Visible: true,
				Opacity: 100,
				W:       1280,
				H:       720,
				Fill:    "#101820",
			},
			{
				ID: lid(
					'b',
				),
				Kind:    KindImage,
				Name:    "Photo",
				Visible: true,
				Opacity: 100,
				W:       1280,
				H:       720,
				Asset:   hashFace,
			},
			{
				ID: lid(
					'c',
				),
				Kind:    KindText,
				Name:    "Title",
				Visible: true,
				Opacity: 100,
				X:       120,
				Y:       300,
				W:       1000,
				H:       144,
				Text:    "HOW IT WORKS",
				Style:   title,
				Slot:    "title",
			},
			{
				ID: lid('d'), Kind: KindShape, Name: "Bar", Visible: true, Opacity: 100, X: 100, Y: 500, W: 600, H: 40,
				Shape: ShapeRect, Fill: "#E63946",
			},
			{
				ID: lid('e'), Kind: KindGroup, Name: "Badge", Visible: true, Opacity: 80, X: 50, Y: 40,
				Children: []Layer{
					{
						ID: lid('f'), Kind: KindShape, Name: "Dot", Visible: true, Opacity: 100, W: 100, H: 100,
						Shape: ShapeEllipse, Fill: "#FFFFFF",
					},
					{
						ID: lid(
							'g',
						),
						Kind:    KindText,
						Name:    "New",
						Visible: true,
						Opacity: 100,
						X:       10,
						Y:       30,
						W:       80,
						H:       36,
						Text:    "NEW",
					},
				},
			},
			{
				ID: lid('h'), Kind: KindShape, Name: "Frame", Visible: true, Locked: true, Opacity: 100, X: 10, Y: 10,
				W: 50, H: 50, Shape: ShapeRect, Fill: "#00FF00",
			},
		},
	}

	ReorderAll(d)

	return d
}
