package export

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"sync/atomic"
	"testing"
	"time"

	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/plan"
)

func renderBands(t *testing.T, d *coreimage.Document, cfg ImageConfig, bands int) *image.RGBA {
	t.Helper()

	img, err := renderImage(context.Background(), d, cfg, bands)
	if err != nil {
		t.Fatal(err)
	}

	return img
}

// TestImageBandsEqualTheWholeRender: the picture drawn in bands on instances of their own is the picture drawn
// in one band, for every look, every mask and every blend mode of the catalogue (SDD-021 section 6.2).
func TestImageBandsEqualTheWholeRender(t *testing.T) {
	src := sourcePNG(t)
	cfg := ImageConfig{AssetPath: func(string) (string, error) { return src, nil }}
	base := lookBase()
	photo, bar, stencil := firstLayerOfKind(base, coreimage.KindImage), lyr('c'), lyr('p')

	docs := map[string]*coreimage.Document{"plain": base, "every look": lookAll(t)}
	for name, ops := range lookOps(photo, bar, stencil) {
		docs[name] = applyLook(t, base, ops...)
	}

	for name, d := range docs {
		t.Run(name, func(t *testing.T) {
			whole := renderBands(t, d, cfg, 1)

			for _, bands := range []int{2, 3, 7, 16} {
				if got := renderBands(t, d, cfg, bands); !bytes.Equal(got.Pix, whole.Pix) {
					t.Fatalf(
						"%d bands differ from the whole picture in %d bytes",
						bands,
						differingBytes(got.Pix, whole.Pix),
					)
				}
			}
		})
	}
}

func differingBytes(a, b []byte) int {
	n := 0

	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			n++
		}
	}

	return n
}

// TestImageBandsStartingFromACachedPicture: an edit that draws only the rows a layer reaches over the cached
// picture gives the picture of the whole document, in any number of bands.
func TestImageBandsStartingFromACachedPicture(t *testing.T) {
	d := lookAll(t)
	ids := layerIDs(d)
	cache := NewImageCache(0)
	cfg := cachedConfig(t, cache, "scope")

	_ = renderBands(t, d, cfg, 3) // caches the picture below the top layer and the whole picture

	for i, bands := range []int{1, 4, 9} {
		edited := withOpacity(t, d, ids[len(ids)-1], 40+i*10)

		got := renderBands(t, edited, cfg, bands)
		want := renderBands(t, edited, ImageConfig{AssetPath: cfg.AssetPath}, 1)

		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("%d bands over the cached picture differ in %d bytes", bands, differingBytes(got.Pix, want.Pix))
		}
	}

	if st := cache.Stats(); st.Hits == 0 {
		t.Fatalf("the edits never started from the cache: %+v", st)
	}
}

// TestImageReferencePixelsAreTheOnesOfBeforeEpic55: the reference document of NFR-061 renders to the pixels
// that the single-instance render of epic 54 drew (the digest was taken from that code), so the bands, the
// parallel preparation and the prepared pictures changed no pixel.
func TestImageReferencePixelsAreTheOnesOfBeforeEpic55(t *testing.T) {
	if testing.Short() {
		t.Skip("the reference document is 4,000 by 4,000 pixels with 30 layers") // verify:env-skip short mode
	}

	const want = "7707d721d7b02d1cbd1587d6ae0e76f2aaf336301a6a14859490c3b1a80adab0"

	src := referenceSource(t)

	img, err := RenderImage(context.Background(), referenceDocument(t), ImageConfig{
		AssetPath: func(string) (string, error) { return src, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(img.Pix)
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("the reference document renders to %s, want %s", got, want)
	}
}

func TestFanOutRunsEveryIndexOnceAndReturnsTheFirstError(t *testing.T) {
	var ran [50]atomic.Int32

	if err := fanOut(context.Background(), 4, len(ran), func(i int) error {
		ran[i].Add(1)

		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for i := range ran {
		if ran[i].Load() != 1 {
			t.Fatalf("index %d ran %d times", i, ran[i].Load())
		}
	}

	boom := errors.New("boom")

	var started atomic.Int32

	err := fanOut(context.Background(), 2, 1000, func(int) error {
		if started.Add(1) == 3 {
			return boom
		}

		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}

	if n := started.Load(); n > 100 {
		t.Fatalf("%d indices ran after the error", n)
	}
}

// TestFanOutGivesUpWaitingWhenTheContextEnds: work that cannot be stopped does not hold the caller back, and
// what is in flight finishes on its own (SDD-021 section 6.2).
func TestFanOutGivesUpWaitingWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	release, finished, started := make(chan struct{}), make(chan struct{}), make(chan struct{}, 4)

	go func() {
		<-started // both workers are in flight when the context ends
		<-started
		cancel()
	}()

	err := fanOut(ctx, 2, 4, func(int) error {
		started <- struct{}{}
		<-release
		finished <- struct{}{}

		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the context error while the work is still running", err)
	}

	close(release)

	for range 2 {
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Fatal("the work in flight did not finish on its own")
		}
	}
}

func TestWorkerCountIsBoundedByCPUsAndByTheLargestLayer(t *testing.T) {
	if n := workerCount(nil); n < 1 || n > maxImageWorkers {
		t.Fatalf("workers for no layer: %d", n)
	}

	small := []plan.Layer{
		{Kind: plan.Bitmap, W: 100, H: 100},
		{Kind: plan.Media, W: 50, H: 50, Crop: plan.Rect{W: 64, H: 64}},
	}
	if n := workerCount(small); n != workerCount(nil) {
		t.Fatalf("small layers: %d workers, want as many as with none", n)
	}

	// 8,192 by 8,192 pixels at 32 bytes a pixel is 2 GiB, the whole budget: one worker.
	huge := []plan.Layer{{Kind: plan.Media, W: 100, H: 100, Crop: plan.Rect{W: 8192, H: 8192}}}
	if n := workerCount(huge); n != 1 {
		t.Fatalf("a layer of 8,192 by 8,192 pixels: %d workers, want 1", n)
	}
}
