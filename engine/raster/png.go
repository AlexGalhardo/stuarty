package raster

import (
	"bytes"
	"image"
	"image/png"
)

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
