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
	cache := rasterize.NewCache(fonts, 0)
	kept := make([]plan.Layer, 0, len(fp.Layers))
	tables := map[string]bool{}

	// The instance that does not test the context at every loop back edge draws about three times faster
	// (SDD-021 section 13.2); the cancellation is kept by checking the context between the layers and by
	// not waiting for the draw itself (renderDetached).
	host, err := renderhost.New(ctx, renderhost.Config{Uncancellable: true})
	if err != nil {
		return nil, rendererError(err)
	}

	abandoned := false

	defer func() {
		if !abandoned {
			_ = host.Close(context.WithoutCancel(ctx))
		}
	}()

	// Each layer is uploaded as soon as it is drawn, so the process never holds the pixels of every layer at
	// once: a document of many large layers is bounded by the memory of the renderer module (out_of_memory),
	// not by the memory of the machine.
	for i := range fp.Layers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		l := fp.Layers[i]
		if opacity(&l) == 0 {
			continue
		}

		switch l.Kind {
		case plan.Media:
			img, err := imageTexture(&l, cfg)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", l.ItemID, err)
			}

			if err := host.PutTexture(ctx, l.ItemID, img.W, img.H, img.Pix, img.Opaque); err != nil {
				return nil, rendererError(err)
			}
		case plan.Bitmap:
			bm, err := cache.Get(*l.Bitmap)
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
		if err := putImageLUT(ctx, host, &l, tables, cfg); err != nil {
			return nil, err
		}

		kept = append(kept, l)
	}

	fp.Layers = withoutLostClips(kept)

	dst := image.NewRGBA(image.Rect(0, 0, doc.Width, doc.Height))
	out := renderhost.Output{Width: doc.Width, Height: doc.Height, Scale: 1}

	if err := renderDetached(ctx, host, renderhost.RenderInput{Output: out, Plan: fp}, dst, &abandoned); err != nil {
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
