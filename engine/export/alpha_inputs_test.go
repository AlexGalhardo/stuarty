package export

import (
	"context"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ingest/mediagen"
)

// TestAlphaInputsComposeWithCorrectEdges (epic 33, SDD-011 section 4.3.6): a clip made from each accepted input
// with alpha, put over a video, composes with a transparent left, an opaque red right and, in between, the soft edge
// blended straight: the half transparent pixel is half red over what is below, not a darker red (which is what a
// premultiplied reading of a straight alpha would give).
func TestAlphaInputsComposeWithCorrectEdges(t *testing.T) {
	for _, in := range mediagen.AlphaInputs() {
		t.Run(string(in), func(t *testing.T) { alphaInputComposes(t, in) })
	}
}

func alphaInputComposes(t *testing.T, in mediagen.AlphaInput) {
	t.Helper()

	const w, h, fps, frames = 320, 180, 30, 30

	base := ingestClip(t, fixtureSpec{Name: "edge-base.mp4", Width: w, Height: h, FPS: fps, Seconds: 2})
	over := ingestClip(t, fixtureSpec{
		Name: "edge-" + string(in) + "." + in.Ext(), Width: w, Height: h, FPS: fps, Seconds: 2, AlphaEdge: string(in),
	})

	if !over.facts.HasAlpha {
		t.Fatalf("the ingest of %s did not report has_alpha: %+v", in, over.facts)
	}

	kind := timeline.AssetVideo
	if !in.Video() {
		kind = timeline.AssetImage
	}

	doc := &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion, ID: "prj_a1p2h3a4b6", Name: "edge " + string(in),
		Settings: timeline.Settings{Width: w, Height: h, FPS: timeline.Rational{Num: fps, Den: 1}},
		Assets: map[string]timeline.Asset{
			"ast_b4s3c1p5k3": {ID: "ast_b4s3c1p5k3", Kind: timeline.AssetVideo, Name: "base.mp4"},
			"ast_p7v3r1a9c3": {ID: "ast_p7v3r1a9c3", Kind: kind, Name: "over"},
		},
		Tracks: map[string]timeline.Track{
			"trk_p1v2e3r4m6": {ID: "trk_p1v2e3r4m6", Kind: timeline.TrackVisual, Name: "Overlay", Order: "a0"},
			"trk_b1a2s3e4c6": {ID: "trk_b1a2s3e4c6", Kind: timeline.TrackVisual, Name: "Video", Order: "a1"},
		},
		Items: map[string]timeline.Item{},
	}
	useAsset(doc, "ast_b4s3c1p5k3", base)
	useAsset(doc, "ast_p7v3r1a9c3", over)

	clipOn := func(id, track, asset string) timeline.Item {
		it := timeline.NewItem(id, timeline.ItemClip, track, 0, frames)
		it.Asset = asset

		return it
	}

	doc.Items["itm_b1s2c3m4p6"] = clipOn("itm_b1s2c3m4p6", "trk_b1a2s3e4c6", "ast_b4s3c1p5k3")
	doc.Items["itm_p1v2c3m4p6"] = clipOn("itm_p1v2c3m4p6", "trk_p1v2e3r4m6", "ast_p7v3r1a9c3")

	render := func(id string, drop bool) []byte {
		t.Helper()

		d := doc
		if drop {
			c := *doc
			c.Items = map[string]timeline.Item{"itm_b1s2c3m4p6": doc.Items["itm_b1s2c3m4p6"]}
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

	with, below := render("edge-with-"+string(in), false), render("edge-below-"+string(in), true)

	px := func(pix []byte, fx float64) [3]int {
		i := (h/2*w + int(fx*w)) * 4

		return [3]int{int(pix[i]), int(pix[i+1]), int(pix[i+2])}
	}

	// A lossy encoder (VP9 at 4:2:0) moves the edge by a few levels; a premultiplied reading moves the half
	// transparent pixel by about 64 levels of red.
	tol := 24
	if in == mediagen.AlphaVP9 {
		tol = 40
	}

	near := func(got, want [3]int, what string) {
		t.Helper()

		for c := range 3 {
			if d := got[c] - want[c]; d > tol || d < -tol {
				t.Errorf("%s = %v, want about %v", what, got, want)

				return
			}
		}
	}

	// Transparent on the left: what is below shows through. Opaque on the right: the red of the animation.
	near(px(with, 0.12), px(below, 0.12), "the transparent part")
	near(px(with, 0.88), [3]int{255, 0, 0}, "the opaque part")

	// The middle of the ramp is half transparent: half red over the picture below, blended straight.
	b := px(below, 0.375)
	near(px(with, 0.375), [3]int{(255 + b[0]) / 2, b[1] / 2, b[2] / 2}, "the edge")
}
