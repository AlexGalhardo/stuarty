package export

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
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

// TestImageBandsInvertedMaskOfASourceOffTheCanvas: a layer masked by the inverse of a layer that reaches no row of
// the canvas shows whole. The source is in the draw even with no row of its own: left out, the masked layer
// would be dropped as one whose source is lost.
func TestImageBandsInvertedMaskOfASourceOffTheCanvas(t *testing.T) {
	src := sourcePNG(t)
	cfg := ImageConfig{AssetPath: func(string) (string, error) { return src, nil }}
	base := lookBase()
	photo, stencil := firstLayerOfKind(base, coreimage.KindImage), lyr('p')

	moved := applyLook(t, base, lookOp("layer.transform", fmt.Sprintf(`{"layer_id": %q, "y": -3000}`, stencil)))
	masked := applyLook(t, moved, lookOp(
		"layer.mask_set",
		fmt.Sprintf(`{"layer_id": %q, "kind": "layer", "source": %q, "invert": true}`, photo, stencil),
	))

	want := renderBands(t, moved, cfg, 1)

	for _, bands := range []int{0, 1, 3} {
		if got := renderBands(t, masked, cfg, bands); !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("%d bands: the masked layer differs in %d bytes", bands, differingBytes(got.Pix, want.Pix))
		}
	}

	cached := cachedConfig(t, NewImageCache(0), "u1")
	if got := renderWith(t, masked, cached); !bytes.Equal(got.Pix, want.Pix) {
		t.Fatalf("from the cache: the masked layer differs in %d bytes", differingBytes(got.Pix, want.Pix))
	}
}

// TestImageLayerGrownPastTheSideOfATexture: a look may grow a layer past the 8,192 pixels of a texture. The
// digest was taken from the render before epic 55, which drew such a layer whole.
func TestImageLayerGrownPastTheSideOfATexture(t *testing.T) {
	const (
		w, h = 8150, 16
		want = "5f1085aeea79b8d220e7f5145e7631c0c26b5723216531cf920a52e460edfc34"
	)

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y * 9), B: 90, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "wide.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	d := &coreimage.Document{
		SchemaVersion: coreimage.SchemaVersion, ID: "img_aaaaaaaaaa", Name: "wide", Width: 400, Height: 200,
		Background: "#101820",
		Assets:     map[string]coreimage.AssetRef{testHash: {Hash: testHash, Name: "w.png", Width: w, Height: h}},
		Layers: []coreimage.Layer{{
			ID: lyr('a'), Kind: coreimage.KindImage, Name: "w", Visible: true, Opacity: 100, X: -3000, Y: 80, W: w,
			H: h, Rotation: 1, Asset: testHash,
		}},
	}
	coreimage.ReorderAll(d)

	d = applyLook(t, d, lookOp("layer.filter_add", `{"layer_id": "`+lyr('a')+`", "kind": "blur", "radius": 40}`))

	got := renderBands(t, d, ImageConfig{AssetPath: func(string) (string, error) { return path, nil }}, 0)

	sum := sha256.Sum256(got.Pix)
	if hex.EncodeToString(sum[:]) != want {
		t.Fatalf("the layer renders to %x, want %s", sum, want)
	}
}

// TestImageLayerLimitIsTheSameInAnyNumberOfBands: the limit of 64 layers of the module is on the draw. Counted per
// band, a document would be drawn on a machine with many CPUs and refused on one with few.
func TestImageLayerLimitIsTheSameInAnyNumberOfBands(t *testing.T) {
	d := &coreimage.Document{
		SchemaVersion: coreimage.SchemaVersion, ID: "img_aaaaaaaaaa", Name: "many", Width: 320, Height: 640,
		Background: "#101820",
	}

	for i := range maxPlanLayers + 1 {
		d.Layers = append(d.Layers, coreimage.Layer{
			ID: fmt.Sprintf("lyr_%010d", i), Kind: coreimage.KindShape, Shape: coreimage.ShapeRect, Name: "s",
			Visible: true, Opacity: 100, X: 4, Y: i * 9, W: 100, H: 8, Fill: "#C0805A",
		})
	}

	coreimage.ReorderAll(d)

	for _, bands := range []int{0, 1, 8} {
		_, err := renderImage(context.Background(), d, ImageConfig{}, bands)
		if err == nil || !strings.Contains(err.Error(), "render_layers") {
			t.Fatalf("%d bands: err = %v, want the limit of layers", bands, err)
		}
	}
}

// preparedRun prepares the layers of a document on a run with the given limit of prepared bytes.
func preparedRun(t *testing.T, d *coreimage.Document, limit int64) ([]drawLayer, plan.FramePlan, error) {
	t.Helper()

	src := sourcePNG(t)
	fonts := rasterize.Bundled()
	fp := coreimage.Plan(d, plan.Options{Measure: rasterize.Measure(fonts), OutputScale: 1})
	run := &imageRun{
		ctx: context.Background(), plan: fp, raster: rasterize.NewCache(fonts, 0), preparedLimit: limit,
		cfg: ImageConfig{AssetPath: func(string) (string, error) { return src, nil }},
	}

	drawn, err := run.prepareAll(fp.Layers)

	return drawn, fp, err
}

// TestImagePreparedPicturesOfARenderAreBounded: the prepared pictures are all in memory until the draw, and a
// document may have 500 large layers, so a render that would hold more than its limit is refused.
func TestImagePreparedPicturesOfARenderAreBounded(t *testing.T) {
	d := lookBase()

	drawn, _, err := preparedRun(t, d, 0)
	if err != nil {
		t.Fatal(err)
	}

	var total int64
	for i := range drawn {
		total += int64(drawn[i].pic.Bytes())
	}

	if _, _, err := preparedRun(t, d, total); err != nil {
		t.Fatalf("a render that fits its limit exactly: %v", err)
	}

	_, _, err = preparedRun(t, d, total-1)
	if err == nil || !strings.Contains(err.Error(), "render_texture_bytes") {
		t.Fatalf("err = %v, want the limit of the prepared pictures", err)
	}
}

// TestImageBandCountBoundsWhatTheInstancesHold: every band holds the pictures that reach it, so the bands of a
// stack of large layers are fewer than the CPUs allow.
func TestImageBandCountBoundsWhatTheInstancesHold(t *testing.T) {
	drawn, fp, err := preparedRun(t, lookBase(), 0)
	if err != nil {
		t.Fatal(err)
	}

	if n := bandCount(drawn, 0, fp.Height, 8, 1<<40); n != 8 {
		t.Fatalf("with memory to spare: %d bands, want 8", n)
	}

	if n := bandCount(drawn, 0, fp.Height, 8, 0); n != 1 {
		t.Fatalf("with no memory to spare: %d bands, want 1", n)
	}

	budget := bandsHold(drawn, 0, fp.Height, 3)

	n := bandCount(drawn, 0, fp.Height, 8, budget)
	if n < 3 || bandsHold(drawn, 0, fp.Height, n) > budget {
		t.Fatalf("%d bands hold %d bytes, over the budget of %d", n, bandsHold(drawn, 0, fp.Height, n), budget)
	}
}

// TestImageSlotIsGivenUpWithTheContext: with every instance of the process at work, a render whose frame was
// replaced stops waiting at once.
func TestImageSlotIsGivenUpWithTheContext(t *testing.T) {
	for range cap(imageSlots) {
		imageSlots <- struct{}{}
	}

	defer func() {
		for range cap(imageSlots) {
			<-imageSlots
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := (&imageRun{ctx: ctx}).acquireSlot(); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the error of the context", err)
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
