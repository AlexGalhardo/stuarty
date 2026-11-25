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
// pixels, 30 layers with their looks): compose_ms is the time to compose the picture after an edit of the
// opacity of a layer, with the CPU path of the renderer and the cache of SDD-021 sections 6.1 and 6.3, the native
// half of "composed for the preview within 100 ms after an edit" for an edit of any layer (author decision
// D-150). It is the slowest of three edits, reported apart as compose_top_ms (the top layer), compose_mid_ms
// (layer 15) and compose_bottom_ms (the bottom layer), and compose_drag_ms, the third edit in a row of layer 15 (a drag); the three edits follow one another, as a person's do, each
// starting from the picture the one before drew. s_export is the time of a cold export, to compose the whole
// document with no cache and no instance of the module held (the pool is drained first) and encode the PNG
// ("exported within 2 s"). The browser half is measured by the Playwright spec of the image editor.
func BenchmarkImageReference(b *testing.B) {
	d := referenceDocument(b)
	src := referenceSource(b)
	paths := func(string) (string, error) { return src, nil }
	cold := ImageConfig{AssetPath: paths}
	cached := ImageConfig{AssetPath: paths, Cache: NewImageCache(0), CacheScope: "benchmark"}
	ctx := context.Background()

	var ids []string

	coreimage.Walk(d, func(l, _ *coreimage.Layer, _ int) { ids = append(ids, l.ID) })

	targets := []struct {
		name string
		id   string
	}{
		{"compose_top_ms", ids[len(ids)-1]},
		{"compose_mid_ms", ids[len(ids)/2]},
		{"compose_bottom_ms", ids[0]},
	}

	// The picture of a first render stays in the cache: the edits that follow start from it.
	if _, err := RenderImage(ctx, d, cached); err != nil {
		b.Fatal(err)
	}

	cur := d

	for i := 0; b.Loop(); i++ {
		// A cold export starts with no instance of the module: the pool keeps them for 30 s, and an export after
		// an idle minute pays for the instances (SDD-021 section 6.2). The pool is filled again by the edit that
		// follows, as it is when a person edits.
		imagePool.Drain()

		start := time.Now()

		img, err := RenderImage(ctx, cur, cold)
		if err != nil {
			b.Fatal(err)
		}

		if _, err := EncodeImage(ctx, img, ImageEncodeOptions{Format: ImagePNG}); err != nil {
			b.Fatal(err)
		}

		export := time.Since(start)

		var worst int64

		for k, target := range targets {
			args := json.RawMessage(fmt.Sprintf(`{"layer_id": %q, "opacity": %d}`, target.id, 50+(i*3+k)%40))

			res, err := coreimage.ApplyBatch(cur, []command.Operation{{Op: "layer.set", Args: args}}, coreimage.Env{})
			if err != nil {
				b.Fatal(err)
			}

			cur = res.Doc

			start = time.Now()

			if _, err := RenderImage(ctx, cur, cached); err != nil {
				b.Fatal(err)
			}

			ms := time.Since(start).Milliseconds()
			worst = max(worst, ms)

			b.ReportMetric(float64(ms), target.name)
		}

		// A drag: the same middle layer edited again, with the instances of the bands holding the rest (SDD-021
		// section 6.3); not part of compose_ms, which is the first edit of a layer.
		var drag int64

		for k := range 3 {
			args := json.RawMessage(fmt.Sprintf(`{"layer_id": %q, "opacity": %d}`, targets[1].id, 20+(i*5+k)%30))

			res, err := coreimage.ApplyBatch(cur, []command.Operation{{Op: "layer.set", Args: args}}, coreimage.Env{})
			if err != nil {
				b.Fatal(err)
			}

			cur = res.Doc

			start = time.Now()

			if _, err := RenderImage(ctx, cur, cached); err != nil {
				b.Fatal(err)
			}

			drag = time.Since(start).Milliseconds()
		}

		b.ReportMetric(float64(drag), "compose_drag_ms")
		b.ReportMetric(float64(worst), "compose_ms")
		b.ReportMetric(export.Seconds(), "s_export")
	}
}
