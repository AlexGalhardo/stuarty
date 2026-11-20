package export

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
)

// BenchmarkImageReference is NFR-061 on the reference document of TestImageReferenceDocument (4,000 by 4,000
// pixels, 30 layers with their looks): compose_ms is the time to compose the picture after an edit of the top
// layer, with the CPU path of the renderer and the cache of the pictures below it (SDD-021 section 6.1), the
// native half of "composed for the preview within 100 ms after an edit"; s_export is the time of a cold export,
// to compose the whole document with no cache and encode the PNG ("exported within 2 s"). The browser half is
// measured by the Playwright spec of the image editor.
func BenchmarkImageReference(b *testing.B) {
	d := referenceDocument(b)
	src := referenceSource(b)
	paths := func(string) (string, error) { return src, nil }
	cold := ImageConfig{AssetPath: paths}
	cached := ImageConfig{AssetPath: paths, Cache: NewImageCache(0), CacheScope: "benchmark"}
	ctx := context.Background()

	top := ""

	coreimage.Walk(d, func(l, _ *coreimage.Layer, _ int) { top = l.ID })

	// The picture of a first render stays in the cache: the edits that follow start from it.
	if _, err := RenderImage(ctx, d, cached); err != nil {
		b.Fatal(err)
	}

	for i := 0; b.Loop(); i++ {
		start := time.Now()

		img, err := RenderImage(ctx, d, cold)
		if err != nil {
			b.Fatal(err)
		}

		if _, err := EncodeImage(ctx, img, ImageEncodeOptions{Format: ImagePNG}); err != nil {
			b.Fatal(err)
		}

		export := time.Since(start)

		args := json.RawMessage(fmt.Sprintf(`{"layer_id": %q, "opacity": %d}`, top, 50+i%40))

		res, err := coreimage.ApplyBatch(d, []command.Operation{{Op: "layer.set", Args: args}}, coreimage.Env{})
		if err != nil {
			b.Fatal(err)
		}

		start = time.Now()

		if _, err := RenderImage(ctx, res.Doc, cached); err != nil {
			b.Fatal(err)
		}

		b.ReportMetric(float64(time.Since(start).Milliseconds()), "compose_ms")
		b.ReportMetric(export.Seconds(), "s_export")
	}
}
