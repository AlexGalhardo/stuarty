package export

import (
	"bytes"
	"context"
	"image"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/compositor"
	"github.com/AlexGalhardo/vukem-video/engine/rasterize"
)

// TestRenderPlansTextAtTheOutputScale: a frame asked for at 2.5 times the project size draws the text
// of a plan made for that size (SDD-004 section 5.3), the plan the preview executor gets for the same
// width, not the bitmap of the project size stretched.
func TestRenderPlansTextAtTheOutputScale(t *testing.T) {
	doc := loadDir(t, filepath.Join("testdata", "golden", "text-style"))
	for id, it := range doc.Items {
		if it.Type == timeline.ItemClip {
			delete(doc.Items, id)
		}
	}
	const outScale = 2.5

	rd, err := NewRenderer(context.Background(), RenderConfig{
		Runner: shared.runner, Doc: doc, JobID: "render-scale", ArtifactsDir: artifactsDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rd.Close() }()

	got, err := rd.Render(context.Background(), 0, outScale)
	if err != nil {
		t.Fatal(err)
	}

	fp := plan.FrameWith(doc, 0, plan.Options{Measure: rasterize.Measure(rasterize.Bundled()), OutputScale: outScale})
	if len(fp.Layers) != 1 || fp.Layers[0].Bitmap == nil || fp.Layers[0].Bitmap.PixelScale != outScale {
		t.Fatalf("the plan for the text is %+v, want one bitmap rasterized at %v", fp.Layers, outScale)
	}

	want := image.NewRGBA(got.Bounds())
	comp := &compositor.Compositor{Bitmaps: rasterize.NewCache(rasterize.Bundled(), 0).Get}
	if err := comp.Compose(want, fp, outScale); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Error("the frame is not the one composed from the plan made for its size")
	}
}
