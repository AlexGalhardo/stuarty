// Package bitmap is the bitmap of the text and shape rasterizer (SDD-004 section 5): the pixels the
// Rust renderer draws for a plan.BitmapSpec, and the metrics the compositors place them with. It has no
// behaviour besides the PNG encoding, so the hosts of the renderer and its users can share the type.
package bitmap

import (
	"bytes"
	"image"
	"image/png"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// Bitmap is a rendered bitmap and its metrics.
type Bitmap struct {
	// Pix is RGBA, 8 bits per channel, straight (non-premultiplied) alpha,
	// Width*Height*4 bytes. It is shared by the cache: do not modify it.
	Pix           []byte
	Width, Height int
	// NaturalW and NaturalH are the size of the bitmap at PixelScale 1, in
	// frame pixels.
	NaturalW, NaturalH float64
	// CenterDX and CenterDY are the offset, at PixelScale 1, from the centre
	// of the bitmap to the centre of the text block (or shape). The item's
	// position names the centre of the block, so a shadow that reaches further
	// on one side moves the bitmap, not the block.
	CenterDX, CenterDY float64
	// WordBoxes is the box of each word of a caption at PixelScale 1, in the
	// coordinates of the bitmap, for the lint and the UI.
	WordBoxes []plan.Rect
}

// PNG encodes the bitmap as a PNG with straight alpha. The encoding is
// deterministic, so a PNG can be cached and compared by hash.
func (b Bitmap) PNG() ([]byte, error) {
	img := &image.NRGBA{Pix: b.Pix, Stride: b.Width * 4, Rect: image.Rect(0, 0, b.Width, b.Height)}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
