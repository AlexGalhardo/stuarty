package export

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	coreimage "github.com/AlexGalhardo/stuarty/core/image"
)

// The prepared picture of a layer is kept under the content of the layer without its opacity, position, blend
// mode and mask (SDD-021 section 6.2): an edit of those prepares nothing again.

func preparedMisses(c *ImageCache) int { return c.Stats().PreparedMisses }

func TestImageCacheKeepsThePreparedLayer(t *testing.T) {
	d := lookAll(t)
	ids := layerIDs(d)
	cache := NewImageCache(0)
	cfg := cachedConfig(t, cache, "u1")
	plain := ImageConfig{AssetPath: cfg.AssetPath}

	renderWith(t, d, cfg)

	first := cache.Stats()
	if first.PreparedLayers == 0 || first.PreparedMisses != first.PreparedLayers {
		t.Fatalf("the first render must prepare each layer once and keep it: %+v", first)
	}

	photo := firstLayerOfKind(d, coreimage.KindImage)
	edits := map[string]*coreimage.Document{
		"opacity of the bottom layer": withOpacity(t, d, ids[0], 40),
		"opacity of the bar":          withOpacity(t, d, lyr('c'), 40),
		"position of the photo": applyLook(
			t,
			d,
			lookOp("layer.transform", fmt.Sprintf(`{"layer_id": %q, "x": 33, "y": 21}`, photo)),
		),
		"blend mode of the bar": applyLook(
			t,
			d,
			lookOp("layer.blend", fmt.Sprintf(`{"layer_id": %q, "mode": "multiply"}`, lyr('c'))),
		),
		"mask of the photo": applyLook(
			t,
			d,
			lookOp("layer.mask_set", fmt.Sprintf(`{"layer_id": %q, "kind": "ellipse"}`, photo)),
		),
		"opacity and position at once": withOpacity(
			t,
			applyLook(t, d, lookOp("layer.transform", fmt.Sprintf(`{"layer_id": %q, "x": 5}`, photo))),
			photo,
			70,
		),
	}

	for name, edited := range edits {
		t.Run(name, func(t *testing.T) {
			before := cache.Stats()

			got := renderWith(t, edited, cfg)

			if after := cache.Stats(); after.PreparedMisses != before.PreparedMisses ||
				after.PreparedHits == before.PreparedHits {
				t.Fatalf("an edit that no look reads prepared a layer again: before %+v, after %+v", before, after)
			}

			if want := renderWith(t, edited, plain); !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf(
					"the picture from kept layers differs from the full render in %d bytes",
					differingBytes(got.Pix, want.Pix),
				)
			}
		})
	}

	t.Run("a look, the size or the source prepares that layer again", func(t *testing.T) {
		for name, edited := range map[string]*coreimage.Document{
			"a filter": applyLook(t, d, lookOp("layer.filter_add", fmt.Sprintf(`{"layer_id": %q, "kind": "blur", "radius": 3}`, lyr('c')))),
			"the size": applyLook(t, d, lookOp("layer.transform", fmt.Sprintf(`{"layer_id": %q, "w": 90, "h": 60}`, lyr('c')))),
		} {
			before := preparedMisses(cache)
			renderWith(t, edited, cfg)

			if got := preparedMisses(cache) - before; got != 1 {
				t.Errorf("%s: %d layers prepared again, want 1", name, got)
			}
		}
	})
}

func TestImageCachePreparedLayersAreNotSharedAcrossUsers(t *testing.T) {
	d := lookAll(t)
	cache := NewImageCache(0)

	renderWith(t, d, cachedConfig(t, cache, "user-a"))

	hits, misses := cache.Stats().PreparedHits, cache.Stats().PreparedMisses

	renderWith(t, d, cachedConfig(t, cache, "user-b"))

	if s := cache.Stats(); s.PreparedHits != hits || s.PreparedMisses == misses {
		t.Fatalf("user b used a prepared layer of user a: %+v", s)
	}
}

func TestImageCachePreparedLayersAreBoundedByTheSameLimit(t *testing.T) {
	const limit = 200 << 10

	d := lookAll(t)
	cache := NewImageCache(limit)
	cfg := cachedConfig(t, cache, "u1")

	for i, id := range layerIDs(d) {
		renderWith(t, withOpacity(t, d, id, 10+i), cfg)

		if s := cache.Stats(); s.Bytes+s.PreparedBytes > limit {
			t.Fatalf(
				"after %d renders the cache holds %d bytes, over its limit of %d",
				i+1,
				s.Bytes+s.PreparedBytes,
				limit,
			)
		}
	}
}

// TestImageCacheRefusesAnAssetTheCallerLostWhileItIsCached: what is cached was drawn from files the caller could
// read then; the render asks for the files again, so a lost asset is as missing as it is to the export.
func TestImageCacheRefusesAnAssetTheCallerLostWhileItIsCached(t *testing.T) {
	d := lookAll(t)
	cfg := cachedConfig(t, NewImageCache(0), "u1")

	renderWith(t, d, cfg)

	lost := errors.New("the image is not available to this caller")
	cfg.AssetPath = func(string) (string, error) { return "", lost }

	if _, err := RenderImage(t.Context(), d, cfg); !errors.Is(err, lost) {
		t.Fatalf("a cached document whose asset was lost rendered: %v", err)
	}
}
