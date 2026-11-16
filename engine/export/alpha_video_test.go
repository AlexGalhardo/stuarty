package export

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// A video with an alpha channel (ProRes 4444, or VP9 in WebM) is composed over the layers below: where
// it is transparent the clip underneath shows, where it is opaque its own colour does (VV-013).
func TestVideoAlphaIsComposedOverTheLayersBelow(t *testing.T) {
	for _, scene := range []string{"alpha-overlay", "alpha-overlay-vp9"} {
		t.Run(scene, func(t *testing.T) { videoAlphaIsComposed(t, scene) })
	}
}

func videoAlphaIsComposed(t *testing.T, scene string) {
	t.Helper()

	doc := loadScene(t, scene)

	if !doc.Assets["ast_p7v3r1a9c2"].HasAlpha {
		t.Fatal("the ingest of a ProRes 4444 clip with alpha did not report has_alpha")
	}

	if doc.Assets["ast_b4s3c1p5k2"].HasAlpha {
		t.Fatal("the ingest of an opaque clip reported has_alpha")
	}

	render := func(id string, drop bool) []byte {
		t.Helper()

		d := doc
		if drop {
			c := *doc
			c.Items = map[string]timeline.Item{}

			for k := range doc.Items {
				if doc.Items[k].Asset != "ast_p7v3r1a9c2" {
					c.Items[k] = doc.Items[k]
				}
			}

			d = &c
		}

		rd, err := NewRenderer(context.Background(), RenderConfig{
			Runner: shared.runner, Doc: d, JobID: id, ArtifactsDir: artifactsDir(),
		})
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = rd.Close() }()

		img, err := rd.Render(context.Background(), 5, 1)
		if err != nil {
			t.Fatal(err)
		}

		return append([]byte(nil), img.Pix...)
	}

	with := render("alpha-with", false)
	base := render("alpha-base", true)

	px := func(pix []byte, x, y int) [3]byte {
		i := (y*320 + x) * 4

		return [3]byte{pix[i], pix[i+1], pix[i+2]}
	}

	left := px(with, 40, 90)
	if want := px(base, 40, 90); left != want {
		t.Errorf("transparent part of the overlay = %v, want the clip below %v", left, want)
	}

	if left == [3]byte{} {
		t.Error("the transparent part of the overlay is black")
	}

	right := px(with, 280, 90)
	if right[0] < 240 || right[1] > 15 || right[2] > 15 {
		t.Errorf("opaque part of the overlay = %v, want red", right)
	}
}
