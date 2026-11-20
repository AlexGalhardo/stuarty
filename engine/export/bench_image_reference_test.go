package export

import (
	"context"
	"testing"
	"time"
)

// BenchmarkImageReference is NFR-061 on the reference document of TestImageReferenceDocument (4,000 by 4,000
// pixels, 30 layers with their looks): compose_ms is the time to compose the picture with the CPU path of the
// renderer, the native half of "composed for the preview within 100 ms after an edit", and s_export the time to
// compose it and encode the PNG ("exported within 2 s"). The browser half is measured by the Playwright spec of
// the image editor.
func BenchmarkImageReference(b *testing.B) {
	d := referenceDocument(b)
	src := referenceSource(b)
	cfg := ImageConfig{AssetPath: func(string) (string, error) { return src, nil }}
	ctx := context.Background()

	for b.Loop() {
		start := time.Now()

		img, err := RenderImage(ctx, d, cfg)
		if err != nil {
			b.Fatal(err)
		}

		composed := time.Since(start)

		if _, err := EncodeImage(ctx, img, ImageEncodeOptions{Format: ImagePNG}); err != nil {
			b.Fatal(err)
		}

		b.ReportMetric(float64(composed.Milliseconds()), "compose_ms")
		b.ReportMetric(time.Since(start).Seconds(), "s_export")
	}
}
