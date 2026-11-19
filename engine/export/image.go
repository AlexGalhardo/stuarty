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

	var (
		textures []textureUpload
		bitmaps  []bitmapUpload
		kept     = make([]plan.Layer, 0, len(fp.Layers))
	)

	for i := range fp.Layers {
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

			textures = append(textures, textureUpload{l.ItemID, img, img.Opaque})
		case plan.Bitmap:
			bm, err := cache.Get(*l.Bitmap)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", l.ItemID, err)
			}

			if bm.Width <= 0 || bm.Height <= 0 {
				continue
			}

			bitmaps = append(bitmaps, bitmapUpload{l.Bitmap.Key(), bm})
		}

		kept = append(kept, l)
	}

	fp.Layers = kept

	host, err := renderhost.New(ctx, renderhost.Config{})
	if err != nil {
		return nil, rendererError(err)
	}

	defer func() { _ = host.Close(context.WithoutCancel(ctx)) }()

	for _, t := range textures {
		if err := host.PutTexture(ctx, t.item, t.img.W, t.img.H, t.img.Pix, t.opaque); err != nil {
			return nil, rendererError(err)
		}
	}

	for _, b := range bitmaps {
		if err := host.PutBitmap(ctx, b.key, b.bm); err != nil {
			return nil, rendererError(err)
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, doc.Width, doc.Height))
	out := renderhost.Output{Width: doc.Width, Height: doc.Height, Scale: 1}

	if err := host.Render(ctx, renderhost.RenderInput{Output: out, Plan: fp}, dst); err != nil {
		return nil, rendererError(err)
	}

	return dst, nil
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
