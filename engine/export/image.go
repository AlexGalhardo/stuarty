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
	"github.com/AlexGalhardo/stuarty/engine/renderhost"
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
		start, base = cfg.Cache.nearest(cfg.CacheScope, chain)
		if start == len(live) {
			return cloneRGBA(base), nil
		}
	}

	// A first stage that ends one layer before the top is kept, so that the next edit of the top layer draws
	// that layer over it; the stage after it draws the rest.
	stops := []int{len(live)}
	if chain != nil && len(live)-1 > start && len(live) >= 2 && chain.valid[len(live)-1] {
		stops = []int{len(live) - 1, len(live)}
	}

	// The instance that does not test the context at every loop back edge draws about three times faster
	// (SDD-021 section 13.2); the cancellation is kept by checking the context between the layers and by
	// not waiting for the draw itself (renderDetached).
	host, err := renderhost.New(ctx, renderhost.Config{Uncancellable: true})
	if err != nil {
		return nil, rendererError(err)
	}

	run := &imageRun{
		ctx: ctx, cfg: cfg, host: host, plan: fp, raster: rasterize.NewCache(fonts, 0), tables: map[string]bool{},
	}

	defer func() {
		if !run.abandoned {
			_ = host.Close(context.WithoutCancel(ctx))
		}
	}()

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

// prefixItem is the id of the texture that holds the picture of the layers already drawn (SDD-021 section 6.1).
const prefixItem = "stuarty-prefix"

// imageRun is one render of a document on one instance of the module.
type imageRun struct {
	ctx       context.Context
	cfg       ImageConfig
	host      *renderhost.Host
	plan      plan.FramePlan
	raster    *rasterize.Cache
	tables    map[string]bool
	abandoned bool
}

// stage draws the layers over base, the picture of the layers below them, or over the background when base is
// nil, and gives the pixels. A picture drawn first and the layers over it are the same pixels as the layers
// drawn in one pass: the draw is a source-over of integers onto the frame, in order.
func (r *imageRun) stage(base *image.RGBA, layers []plan.Layer) (*image.RGBA, error) {
	ctx, host, fp := r.ctx, r.host, r.plan
	kept := make([]plan.Layer, 0, len(layers)+1)

	if base != nil {
		b := base.Rect
		if err := host.PutTexture(ctx, prefixItem, b.Dx(), b.Dy(), base.Pix, !fp.Transparent); err != nil {
			return nil, rendererError(err)
		}

		kept = append(kept, plan.Layer{
			ItemID: prefixItem, Kind: plan.Media, AssetID: prefixItem, AssetKind: "image",
			Crop: plan.Rect{W: float64(b.Dx()), H: float64(b.Dy())}, CX: float64(b.Dx()) / 2, CY: float64(b.Dy()) / 2,
			W: float64(b.Dx()), H: float64(b.Dy()), Scale: 1, Opacity: 1,
		})
	}

	// Each layer is uploaded as soon as it is drawn, so the process never holds the pixels of every layer at
	// once: a document of many large layers is bounded by the memory of the renderer module (out_of_memory),
	// not by the memory of the machine.
	for i := range layers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		l := layers[i]

		switch l.Kind {
		case plan.Media:
			img, err := imageTexture(&l, r.cfg)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", l.ItemID, err)
			}

			if err := host.PutTexture(ctx, l.ItemID, img.W, img.H, img.Pix, img.Opaque); err != nil {
				return nil, rendererError(err)
			}
		case plan.Bitmap:
			bm, err := r.raster.Get(*l.Bitmap)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", l.ItemID, err)
			}

			if bm.Width <= 0 || bm.Height <= 0 {
				continue
			}

			if err := host.PutBitmap(ctx, l.Bitmap.Key(), bm); err != nil {
				return nil, rendererError(err)
			}
		case plan.Group:
			// An image document has layers of its own kinds; a group is the plan of nested timelines (SDD-004 §3.16).
			return nil, fmt.Errorf("layer %s: %w: an image has no group", l.ItemID, compositor.ErrSource)
		}

		// A table is in the module before the layer that names it is drawn: a layer is never drawn without it.
		if err := putImageLUT(ctx, host, &l, r.tables, r.cfg); err != nil {
			return nil, err
		}

		kept = append(kept, l)
	}

	fp.Layers = withoutLostClips(kept)

	dst := image.NewRGBA(image.Rect(0, 0, fp.Width, fp.Height))
	out := renderhost.Output{Width: fp.Width, Height: fp.Height, Scale: 1}

	if err := renderDetached(ctx, host, renderhost.RenderInput{Output: out, Plan: fp}, dst, &r.abandoned); err != nil {
		return nil, err
	}

	return dst, nil
}

// renderDetached runs the draw on an instance that cannot be interrupted and gives up waiting when the context
// ends: the caller gets the context error at once, and the goroutine closes the instance when the draw, which
// the limits of the document bound, is over. abandoned tells the caller not to close the instance itself.
func renderDetached(
	ctx context.Context, host *renderhost.Host, in renderhost.RenderInput, dst *image.RGBA, abandoned *bool,
) error {
	done := make(chan error, 1)

	go func() {
		done <- host.Render(context.WithoutCancel(ctx), in, dst)

		close(done)
	}()

	select {
	case err := <-done:
		if err != nil {
			return rendererError(err)
		}

		return nil
	case <-ctx.Done():
		*abandoned = true

		go func() {
			for range done { // the draw has returned when the channel is closed
			}

			_ = host.Close(context.WithoutCancel(ctx))
		}()

		return ctx.Err()
	}
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
