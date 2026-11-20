package export

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// videoFrame is frame 0 of a clip of the portrait fixture that fills the frame, drawn by the video export; the
// clip has the table of the kind at the intensity when tableKind is not empty.
func videoFrame(t *testing.T, tableKind string, intensity float64) []byte {
	t.Helper()

	b := newCase("lut parity "+tableKind, rate(30, 1))
	asset := b.video(portrait)

	var table string
	if tableKind != "" {
		table = b.lutAsset(tableKind)
	}

	b.clip(b.track(), asset, 0, 30, func(it *timeline.Item) {
		if table != "" {
			it.Lut = &timeline.Lut{Asset: table, Intensity: intensity}
		}
	})

	c := b.build(0)

	for id, spec := range c.fixtures {
		if spec.LUT != "" {
			useLUT(t, c.doc, id, spec)

			continue
		}

		useAsset(c.doc, id, ingestClip(t, spec))
	}

	return renderDoc(t, c.doc, "lut-parity-"+tableKind, 1, []int{0})[0]
}

// TestImageLUTMatchesVideoLUT: the same table at the same intensity gives the same bytes on an image layer as on a
// clip of the same size (SDD-004 section 3.6, the golden cases lut-warm and lut-teal-half): the picture of the clip
// without the table is the source of the image layer, so both draw the table over the same pixels.
func TestImageLUTMatchesVideoLUT(t *testing.T) {
	setup(t)

	plain := videoFrame(t, "", 0)

	// the frame of the clip, as a PNG file: the source of the image layer
	src := image.NewNRGBA(image.Rect(0, 0, goldenW, goldenH))
	copy(src.Pix, plain)

	for i := 3; i < len(src.Pix); i += 4 {
		if src.Pix[i] != 255 {
			t.Fatalf("the frame of the clip is not opaque at byte %d: the comparison needs an opaque source", i)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(t.TempDir(), "frame.png")
	if err := os.WriteFile(srcPath, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name      string
		kind      string
		intensity float64
		percent   string
	}{
		{"lut-warm", "warm", 1, "100"},
		{"lut-teal-half", "teal", 0.5, "50"},
	} {
		t.Run(c.name, func(t *testing.T) {
			want := videoFrame(t, c.kind, c.intensity)
			if bytes.Equal(want, plain) {
				t.Fatal("the table left the clip as it was")
			}

			hash, tablePath := lutFile(t, lutFixture(c.kind))
			d := &coreimage.Document{
				SchemaVersion: coreimage.SchemaVersion,
				ID:            "img_aaaaaaaaaa",
				Name:          "parity",
				Width:         goldenW,
				Height:        goldenH,
				Background:    "#101820",
				Assets: map[string]coreimage.AssetRef{
					testHash: {Hash: testHash, Name: "frame.png", Width: goldenW, Height: goldenH},
				},
				Layers: []coreimage.Layer{{
					ID: lyr('a'), Kind: coreimage.KindImage, Name: "Frame", Visible: true, Opacity: 100, W: goldenW,
					H: goldenH, Asset: testHash,
				}},
			}
			coreimage.ReorderAll(d)

			res, err := coreimage.ApplyBatch(d, []command.Operation{{Op: "layer.adjust", Args: []byte(
				`{"layer_id": "` + lyr(
					'a',
				) + `", "lut": {"asset": "` + hash + `", "intensity": ` + c.percent + `}}`,
			)}}, lutEnv(hash))
			if err != nil {
				t.Fatal(err)
			}

			got, err := RenderImage(t.Context(), res.Doc, ImageConfig{
				AssetPath: func(string) (string, error) { return srcPath, nil },
				LUTPath:   func(string) (string, error) { return tablePath, nil },
			})
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(got.Pix, want) {
				first := -1

				for i := range want {
					if got.Pix[i] != want[i] {
						first = i

						break
					}
				}

				t.Fatalf("the image layer differs from the clip (first byte %d)", first)
			}
		})
	}
}
