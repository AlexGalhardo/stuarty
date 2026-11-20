package export

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"testing"

	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/plan"
)

func cachedConfig(t *testing.T, cache *ImageCache, scope string) ImageConfig {
	t.Helper()

	src := sourcePNG(t)

	return ImageConfig{
		AssetPath: func(string) (string, error) { return src, nil }, Cache: cache, CacheScope: scope,
	}
}

// layerIDs lists the layers of a document, bottom first.
func layerIDs(d *coreimage.Document) []string {
	var ids []string

	coreimage.Walk(d, func(l, _ *coreimage.Layer, _ int) { ids = append(ids, l.ID) })

	return ids
}

func withOpacity(t *testing.T, d *coreimage.Document, id string, pct int) *coreimage.Document {
	t.Helper()

	return applyLook(t, d, lookOp("layer.set", fmt.Sprintf(`{"layer_id": %q, "opacity": %d}`, id, pct)))
}

func renderWith(t *testing.T, d *coreimage.Document, cfg ImageConfig) *image.RGBA {
	t.Helper()

	img, err := RenderImage(context.Background(), d, cfg)
	if err != nil {
		t.Fatal(err)
	}

	return img
}

// TestImageCacheRenderIsTheFullRender: a render that starts from a cached picture is the same pixels as the
// render of the whole document with no cache, whichever layer was edited (looks, masks and blend modes
// included, which read the picture below them).
func TestImageCacheRenderIsTheFullRender(t *testing.T) {
	base := lookAll(t)
	ids := layerIDs(base)

	cache := NewImageCache(0)
	cfg := cachedConfig(t, cache, "u1")
	plain := cachedConfig(t, nil, "")

	if !bytes.Equal(renderWith(t, base, cfg).Pix, renderWith(t, base, plain).Pix) {
		t.Fatal("the first render with a cache differs from the render with none")
	}

	for _, id := range ids {
		edited := withOpacity(t, base, id, 55)

		got, want := renderWith(t, edited, cfg), renderWith(t, edited, plain)
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Errorf("editing %s: the render from the cache differs from the full render", id)
		}

		// and the render of the document again, now from a cache that holds the edited one
		if again := renderWith(t, edited, cfg); !bytes.Equal(again.Pix, want.Pix) {
			t.Errorf("editing %s: the second render differs", id)
		}
	}
}

func TestImageCacheHitsAfterAnEditOfTheTopLayer(t *testing.T) {
	base := lookBase() // no layer is clipped to another, so the cut below the top layer is allowed
	ids := layerIDs(base)
	cache := NewImageCache(0)
	cfg := cachedConfig(t, cache, "u1")

	renderWith(t, base, cfg)

	if s := cache.Stats(); s.Hits != 0 || s.Misses != 1 || s.Pictures < 2 {
		t.Fatalf("after the first render: %+v (a miss, the picture below the top layer and the whole one)", s)
	}

	renderWith(t, base, cfg) // nothing changed: the whole picture is in the cache

	if s := cache.Stats(); s.Hits != 1 {
		t.Fatalf("the same document again: %+v", s)
	}

	renderWith(t, withOpacity(t, base, ids[len(ids)-1], 40), cfg)

	if s := cache.Stats(); s.Hits != 2 {
		t.Fatalf("an edit of the top layer must start from the picture below it: %+v", s)
	}

	before := cache.Stats().Hits

	renderWith(t, withOpacity(t, base, ids[0], 40), cfg)

	if s := cache.Stats(); s.Hits != before {
		t.Fatalf("an edit of the bottom layer leaves nothing to start from: %+v", s)
	}
}

func TestImageCacheIsBounded(t *testing.T) {
	const picture = 320 * 180 * 4

	cache := NewImageCache(2*picture + picture/2)
	cfg := cachedConfig(t, cache, "u1")
	base := lookAll(t)
	ids := layerIDs(base)

	for i, id := range ids {
		renderWith(t, withOpacity(t, base, id, 10+i), cfg)

		if s := cache.Stats(); s.Bytes > 2*picture+picture/2 || s.Pictures > 2 {
			t.Fatalf("after %d renders the cache holds %d pictures, %d bytes", i+1, s.Pictures, s.Bytes)
		}
	}

	// a limit under one picture keeps none, and the render still works
	tiny := NewImageCache(picture - 1)
	got, want := renderWith(t, base, cachedConfig(t, tiny, "u1")), renderWith(t, base, cachedConfig(t, nil, ""))

	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("a cache that keeps nothing changed the render")
	}

	if s := tiny.Stats(); s.Pictures != 0 || s.Bytes != 0 {
		t.Fatalf("a picture larger than the limit was kept: %+v", s)
	}
}

// TestImageCacheIsNotSharedAcrossUsers: the picture of one user's document is never the start of another's, even
// for the same document (network mode, T-89).
func TestImageCacheIsNotSharedAcrossUsers(t *testing.T) {
	base := lookAll(t)
	cache := NewImageCache(0)

	renderWith(t, base, cachedConfig(t, cache, "user-a"))
	hits := cache.Stats().Hits

	renderWith(t, base, cachedConfig(t, cache, "user-b"))

	if s := cache.Stats(); s.Hits != hits {
		t.Fatalf("user b started from a picture of user a: %+v", s)
	}

	renderWith(t, base, cachedConfig(t, cache, "user-a"))

	if s := cache.Stats(); s.Hits != hits+1 {
		t.Fatalf("user a does not find its own picture: %+v", s)
	}
}

// TestImageCacheHandsOutNoPictureOfItsOwn: the render the caller gets is its own, so writing to it does not
// change the next render.
func TestImageCacheHandsOutNoPictureOfItsOwn(t *testing.T) {
	base := lookAll(t)
	cfg := cachedConfig(t, NewImageCache(0), "u1")

	first := renderWith(t, base, cfg)
	want := append([]byte(nil), first.Pix...)

	for i := range first.Pix {
		first.Pix[i] = 0
	}

	if second := renderWith(t, base, cfg); !bytes.Equal(second.Pix, want) {
		t.Fatal("the cache aliased the picture it gave out")
	}
}

func TestImageCacheKeyFollowsTheContentOfTheLayers(t *testing.T) {
	layer := func(item string, x float64) plan.Layer {
		return plan.Layer{ItemID: item, Kind: plan.Bitmap, CX: x, Opacity: 1}
	}
	fp := plan.FramePlan{Width: 10, Height: 10, Background: "#000000"}

	a := newLayerChain(fp, []plan.Layer{layer("a", 1), layer("b", 2), layer("c", 3)})
	b := newLayerChain(fp, []plan.Layer{layer("a", 1), layer("b", 9), layer("c", 3)})
	other := plan.FramePlan{Width: 10, Height: 10, Background: "#FFFFFF"}
	c := newLayerChain(other, []plan.Layer{layer("a", 1), layer("b", 2), layer("c", 3)})

	if a.digest[1] != b.digest[1] || a.digest[2] == b.digest[2] || a.digest[3] == b.digest[3] {
		t.Error("an edit must change the digest of its layer and the ones above it, and no other")
	}

	if a.digest[1] == c.digest[1] || a.digest[0] == c.digest[0] {
		t.Error("another background must change every digest")
	}
}

func TestImageCacheNeverCutsBetweenALayerAndItsClipSource(t *testing.T) {
	layers := []plan.Layer{
		{ItemID: "src", Kind: plan.Bitmap, Opacity: 1},
		{ItemID: "x", Kind: plan.Bitmap, Opacity: 1},
		{ItemID: "clipped", Kind: plan.Bitmap, Opacity: 1, Clip: &plan.ClipSpec{Source: "src"}},
		{ItemID: "gone", Kind: plan.Bitmap, Opacity: 1, Clip: &plan.ClipSpec{Source: "missing"}},
	}

	ch := newLayerChain(plan.FramePlan{Width: 4, Height: 4}, layers)

	// cut after 1 or 2 layers: "clipped" is above, its source below
	want := []bool{true, false, false, true, true}
	for k, w := range want {
		if ch.valid[k] != w {
			t.Errorf("a cut after %d layers is valid = %v, want %v", k, ch.valid[k], w)
		}
	}
}

// TestImageCacheStartsFromThePictureBelowTheTopLayer: with no clip in the document the cut below the top layer
// is used, and the picture drawn from it is the full render, with a blend mode that reads the picture below it
// on the top layer.
func TestImageCacheStartsFromThePictureBelowTheTopLayer(t *testing.T) {
	base := lookBase()
	photo, bar, top := firstLayerOfKind(base, coreimage.KindImage), lyr('c'), lyr('p')
	ops := lookOps(photo, bar, top)

	var all []string

	for _, name := range []string{"adjust_colour", "filter_sharpen", "filter_shadow", "blend_soft"} {
		for _, op := range ops[name] {
			base = applyLook(t, base, op)
		}

		all = append(all, name)
	}

	base = applyLook(t, base, lookOp("layer.blend", `{"layer_id": "`+top+`", "mode": "overlay"}`),
		lookOp("layer.filter_add", `{"layer_id": "`+top+`", "kind": "glow", "radius": 6, "color": "#00FFFF"}`))

	cache := NewImageCache(0)
	cfg, plain := cachedConfig(t, cache, "u1"), cachedConfig(t, nil, "")

	renderWith(t, base, cfg)

	for _, pct := range []int{80, 60, 30} {
		edited := withOpacity(t, base, top, pct)

		if got, want := renderWith(t, edited, cfg), renderWith(t, edited, plain); !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("opacity %d on the top layer: the render from the cache differs (looks %v)", pct, all)
		}
	}

	if s := cache.Stats(); s.Hits < 3 {
		t.Fatalf("every edit of the top layer must start from the cache: %+v", s)
	}
}

// TestImageCacheOnATransparentCanvas: the picture below the top layer has alpha, so it goes back as a texture
// that is not opaque, and the result is still the full render.
func TestImageCacheOnATransparentCanvas(t *testing.T) {
	base := imageDoc("transparent")
	base.Assets = nil
	base.Layers = append(base.Layers, coreimage.Layer{
		ID: lyr('z'), Kind: coreimage.KindShape, Name: "Half", Visible: true, Opacity: 60, X: 140, Y: 60, W: 120, H: 80,
		Shape: coreimage.ShapeRect, Fill: "#FF0000",
	})
	coreimage.ReorderAll(base)

	cache := NewImageCache(0)
	cfg, plain := cachedConfig(t, cache, "u1"), cachedConfig(t, nil, "")

	renderWith(t, base, cfg)

	for _, pct := range []int{90, 35} {
		edited := withOpacity(t, base, lyr('z'), pct)

		if got, want := renderWith(t, edited, cfg), renderWith(t, edited, plain); !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("opacity %d: the render from the cache differs on a transparent canvas", pct)
		}
	}

	if s := cache.Stats(); s.Hits < 2 {
		t.Fatalf("the edits must start from the cache: %+v", s)
	}
}

// TestImageCacheOnADocumentWithNoLayerToDraw: a document whose layers are all hidden (or a new one with none)
// has nothing to look up; with a cache it draws the canvas like the render with none.
func TestImageCacheOnADocumentWithNoLayerToDraw(t *testing.T) {
	base := imageDoc("transparent")
	base.Assets = nil
	base.Layers = nil

	cache := NewImageCache(0)
	cfg, plain := cachedConfig(t, cache, "u1"), cachedConfig(t, nil, "")

	for range 2 {
		if got, want := renderWith(t, base, cfg), renderWith(t, base, plain); !bytes.Equal(got.Pix, want.Pix) {
			t.Fatal("a document with no layer to draw renders differently with a cache")
		}
	}
}
