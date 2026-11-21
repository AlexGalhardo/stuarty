package export

import (
	"context"
	"errors"
	"fmt"
	"image"

	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/compositor"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
)

// ImageConfig is what the render of an image document needs from the machine (SDD-021 section 6).
type ImageConfig struct {
	// AssetPath gives the path of the normalized PNG of an image asset, by content hash.
	AssetPath func(hash string) (string, error)
	// LUTPath gives the path of the normalized .cube file of a lookup-table asset (SDD-021 section 3.1), by
	// content hash. It is needed only by a document with a layer that has a table.
	LUTPath func(hash string) (string, error)
	// Fonts is the font set of the text layers; the bundled fonts when nil.
	Fonts *rasterize.FontSet
	// Cache, when set, lets the render start from the picture of the layers that an earlier render of the same
	// document already drew (SDD-021 section 6.1); CacheScope is the owner of those pictures, a user in network
	// mode, and a render never reads a picture of another scope. A custom Fonts set turns the cache off.
	Cache      *ImageCache
	CacheScope string
}

// RenderImage draws an image document at its canvas size with the CPU path of the Rust renderer (the bit-exact
// one, through wazero), as the export of a video does: it builds the one-frame plan, rasterizes each text and
// shape, decodes each image layer, uploads them and takes the pixels. The same document and the same files
// always give the same pixels. The result is premultiplied RGBA, as image.RGBA is.
func RenderImage(ctx context.Context, doc *coreimage.Document, cfg ImageConfig) (*image.RGBA, error) {
	return renderImage(ctx, doc, cfg, 0)
}

// renderImage is RenderImage with the number of bands the canvas is drawn in; 0 is as many as the work and the
// machine allow (SDD-021 section 6.2). The pixels never depend on it.
func renderImage(ctx context.Context, doc *coreimage.Document, cfg ImageConfig, bands int) (*image.RGBA, error) {
	fonts := cfg.Fonts
	if fonts == nil {
		fonts = rasterize.Bundled()
	}

	fp := coreimage.Plan(doc, plan.Options{Measure: rasterize.Measure(fonts), OutputScale: 1})

	live := make([]plan.Layer, 0, len(fp.Layers))

	for i := range fp.Layers {
		if opacity(&fp.Layers[i]) != 0 {
			live = append(live, fp.Layers[i])
		}
	}

	// The cache keys layers by what they draw; a custom font set is not part of the key, so it is not cached.
	var chain *layerChain
	if cfg.Cache != nil && cfg.Fonts == nil {
		chain = newLayerChain(fp, live)
	}

	start, base := 0, (*image.RGBA)(nil)
	if chain != nil {
		// What the cache keeps was drawn from files the caller could read then, not necessarily now: an asset the
		// caller lost access to must not stay in the picture because it is cached (the export, with no cache, reads
		// every file).
		if err := verifyAccess(cfg, live); err != nil {
			return nil, err
		}

		start, base = cfg.Cache.nearest(cfg.CacheScope, chain)
		// with no layer to draw there is nothing cached (nearest starts at one layer): the canvas is drawn below
		if base != nil && start == len(live) {
			return cloneRGBA(base), nil
		}
	}

	// A first stage that ends one layer before the top is kept, so that the next edit of the top layer draws
	// that layer over it; the stage after it draws the rest.
	stops := []int{len(live)}
	if chain != nil && len(live)-1 > start && len(live) >= 2 && chain.valid[len(live)-1] {
		stops = []int{len(live) - 1, len(live)}
	}

	// The instances that do not test the context at every loop back edge draw about three times faster
	// (SDD-021 section 13.2); the cancellation is kept by checking the context between the layers and by
	// not waiting for the work in flight (SDD-021 section 6.2).
	run := &imageRun{ctx: ctx, cfg: cfg, plan: fp, raster: rasterize.NewCache(fonts, 0), bands: bands}
	if chain != nil {
		run.cache = cfg.Cache
	}

	for _, stop := range stops {
		img, err := run.stage(base, live[start:stop])
		if err != nil {
			return nil, err
		}

		if chain != nil && stop < len(live) {
			cfg.Cache.put(cfg.CacheScope, chain.digest[stop], img)
		}

		base, start = img, stop
	}

	if chain != nil {
		cfg.Cache.put(cfg.CacheScope, chain.digest[len(live)], cloneRGBA(base))
	}

	return base, nil
}

// / verifyAccess asks for the file of every image and lookup table the layers name, without reading it, so that a
// render that starts from the cache refuses what the export would refuse.
func verifyAccess(cfg ImageConfig, layers []plan.Layer) error {
	seen := map[string]bool{}

	for i := range layers {
		l := &layers[i]

		if l.Kind == plan.Media && !seen[l.AssetID] {
			seen[l.AssetID] = true

			if cfg.AssetPath == nil {
				return errors.New("no image files are available")
			}

			if _, err := cfg.AssetPath(l.AssetID); err != nil {
				return fmt.Errorf("layer %s: %w", l.ItemID, err)
			}
		}

		if l.Lut != nil && !seen["lut:"+l.Lut.Asset] {
			seen["lut:"+l.Lut.Asset] = true

			if cfg.LUTPath == nil {
				return exportErr(ReasonRender, "No lookup table files are available.", errors.New("no lut source"), "")
			}

			if _, err := cfg.LUTPath("sha256:" + l.Lut.Asset); err != nil {
				return exportErr(
					ReasonRender,
					"A lookup table of the image is not available.",
					fmt.Errorf("layer %s: %w", l.ItemID, err),
					"",
				)
			}
		}
	}

	return nil
}

// prefixItem is the id of the texture that holds the picture of the layers already drawn (SDD-021 section 6.1).
const prefixItem = "stuarty-prefix"

// imageRun is one render of a document: the plan, the rasterizer cache and the context; the instances of the
// module that prepare and draw it are made by the stages (image_bands.go).
type imageRun struct {
	ctx    context.Context
	cfg    ImageConfig
	plan   plan.FramePlan
	raster *rasterize.Cache
	bands  int
	// cache keeps the prepared layers, when the render may use one (SDD-021 section 6.2).
	cache *ImageCache
}

// withoutLostClips drops the layers whose mask source did not make it to the renderer (a text with no pixels, an
// opacity that rounds to none): a layer clipped to nothing draws nothing, and the renderer refuses a clip whose
// source it does not hold (SDD-013 section 3.11.4).
func withoutLostClips(layers []plan.Layer) []plan.Layer {
	for {
		have := make(map[string]bool, len(layers))
		for i := range layers {
			have[layers[i].ItemID] = true
		}

		kept := layers[:0]

		for i := range layers {
			if c := layers[i].Clip; c != nil && !have[c.Source] {
				continue
			}

			kept = append(kept, layers[i])
		}

		if len(kept) == len(layers) {
			return kept
		}

		layers = kept
	}
}

// imageTexture is the pixels of an image layer: the part of the source the layer shows, scaled down to the box
// when the box is smaller, as the video export does, so a large photo is averaged and not point sampled.
func imageTexture(l *plan.Layer, cfg ImageConfig) (compositor.Image, error) {
	if cfg.AssetPath == nil {
		return compositor.Image{}, errors.New("no image files are available")
	}

	path, err := cfg.AssetPath(l.AssetID)
	if err != nil {
		return compositor.Image{}, err
	}

	src, err := loadImage(path)
	if err != nil {
		return compositor.Image{}, fmt.Errorf("image %s: %w", l.AssetID, err)
	}

	x, y := min(max(round(l.Crop.X), 0), max(src.W-1, 0)), min(max(round(l.Crop.Y), 0), max(src.H-1, 0))
	w, h := min(max(round(l.Crop.W), 1), src.W-x), min(max(round(l.Crop.H), 1), src.H-y)
	img := cropImage(src, x, y, w, h)

	if tw, th := max(round(l.W), 1), max(round(l.H), 1); l.Rotation == 0 && tw < w && th < h {
		img = compositor.Resize(img, tw, th)
	}

	return img, nil
}
