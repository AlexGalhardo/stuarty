package export

import (
	"bytes"
	"context"
	"runtime"
	"testing"
)

// TestImageReferenceDocumentIsTheSameBytesForEveryWorkerCount: the reference document of NFR-061 exports to the same
// bytes with 1, 2 and 8 workers (the number of workers and of bands follows the CPUs that may run, SDD-021 section
// 6.2), and twice with the same count (NFR-062). The bands are cut where the work is, and the pixels never depend on
// where.
func TestImageReferenceDocumentIsTheSameBytesForEveryWorkerCount(t *testing.T) {
	if testing.Short() {
		t.Skip("three exports of the reference document") // verify:env-skip short mode: about a minute
	}

	d := referenceDocument(t)
	src := referenceSource(t)
	cfg := ImageConfig{AssetPath: func(string) (string, error) { return src, nil }}

	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))

	var first []byte

	for _, workers := range []int{8, 1, 2, 8} {
		runtime.GOMAXPROCS(workers)

		img, err := RenderImage(context.Background(), d, cfg)
		if err != nil {
			t.Fatal(err)
		}

		enc, err := EncodeImage(context.Background(), img, ImageEncodeOptions{Format: ImagePNG})
		if err != nil {
			t.Fatal(err)
		}

		if first == nil {
			first = enc

			continue
		}

		if !bytes.Equal(first, enc) {
			t.Fatalf("the export with %d workers differs from the first one", workers)
		}
	}
}
