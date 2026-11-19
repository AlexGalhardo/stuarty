package export

import (
	"fmt"
	"image"
	"image/draw"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/compositor"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
)

// animCacheBytes is how many bytes of cropped and scaled frames of animated images one frame source
// keeps. A looping animation asks for the same few frames again and again; past the budget a frame is
// decoded again from the sequence each time it is asked for.
const animCacheBytes = 128 << 20

// animKey names one delivered frame of the animation of one item, including the crop and the size it
// was scaled to, so a later frame with another crop is not the cached picture of the first.
type animKey struct {
	item       string
	frame      int
	x, y, w, h int
	tw, th     int
}

// animImage is the picture of an animated image layer (SDD-004 section 3.3): the frame the plan names, read
// from the frame sequence artifact of ingest, with the same crop, scale and straight-alpha conversion as a
// still image. The frame index comes from the plan only; nothing here depends on the clock or on the order of the frames
// asked for, so a preview and an export give the same pixels.
func (f *frameSource) animImage(l *plan.Layer, a *timeline.Asset) (compositor.Image, error) {
	seq, err := f.sequence(l.AssetID, a)
	if err != nil {
		return compositor.Image{}, err
	}
	n := seq.Frames()
	pix, err := seq.Frame(((l.SourceFrame % n) + n) % n)
	if err != nil {
		return compositor.Image{}, fmt.Errorf("animated image %s: %w", l.AssetID, err)
	}
	src := toPremultiplied(pix, seq.W, seq.H)

	cx, cy, cw, ch := cropRect(l, a)
	cw, ch = min(cw, max(src.W-cx, 0)), min(ch, max(src.H-cy, 0))
	tw, th, scaled := f.target(l, cw, ch)
	key := animKey{l.ItemID, ((l.SourceFrame % n) + n) % n, cx, cy, cw, ch, tw, th}
	if img, ok := f.animFrames[key]; ok {
		return img, nil
	}
	img := cropImage(src, cx, cy, cw, ch)
	if scaled && (tw != cw || th != ch) {
		img = compositor.Resize(img, tw, th)
	}
	if size := int64(len(img.Pix)); f.animBytes+size <= animCacheBytes {
		f.animFrames[key] = img
		f.animBytes += size
	}
	return img, nil
}

// sequence is the open frame sequence of an animated asset, made on the first use.
func (f *frameSource) sequence(assetID string, a *timeline.Asset) (*ingest.Anim, error) {
	if seq, ok := f.anims[assetID]; ok {
		return seq, nil
	}
	path, err := f.s.artifactOf(a, ingest.AnimKind, ingest.AnimExt)
	if err != nil {
		return nil, err
	}
	seq, err := ingest.OpenAnim(path)
	if err != nil {
		return nil, fmt.Errorf("animated image %s: %w", assetID, err)
	}
	f.anims[assetID] = seq
	return seq, nil
}

// toPremultiplied turns straight-alpha RGBA pixels into the premultiplied image of the compositor, by the
// conversion of loadImage, so a sticker of a still image and one frame of an animation blend the same way.
func toPremultiplied(pix []byte, w, h int) compositor.Image {
	nr := &image.NRGBA{Pix: pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	rgba := image.NewRGBA(nr.Rect)
	draw.Draw(rgba, rgba.Rect, nr, nr.Rect.Min, draw.Src)
	return compositor.Image{Pix: rgba.Pix, W: w, H: h, Opaque: isOpaque(rgba.Pix)}
}

func isOpaque(pix []byte) bool {
	for i := 3; i < len(pix); i += 4 {
		if pix[i] != 255 {
			return false
		}
	}
	return true
}
