package compositor

import "image"

// Resize scales src to w by h pixels with the sampler of the compositor. A source at least twice as
// large as the target is first halved by averaging 2x2 blocks, so that a photo scaled down to a frame
// is not point-sampled (bilinear alone skips most of its pixels). The result is premultiplied like src.
func Resize(src Image, w, h int) Image {
	if w <= 0 || h <= 0 || src.W <= 0 || src.H <= 0 {
		return Image{}
	}
	for src.W >= 2*w && src.H >= 2*h {
		src = halve(src)
	}
	if src.W == w && src.H == h {
		return src
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	c := Compositor{Bands: 1}
	whole := placement{cx: float64(w) / 2, cy: float64(h) / 2, w: float64(w), h: float64(h)}
	c.sample(dst, src, whole, 255, blendNormal)
	return Image{Pix: dst.Pix, W: w, H: h, Opaque: src.Opaque}
}

// halve averages each 2x2 block, rounding to nearest; an odd last row or column is dropped.
func halve(src Image) Image {
	w, h := src.W/2, src.H/2
	out := make([]byte, w*h*4)
	stride := src.W * 4
	for y := range h {
		for x := range w {
			i := (2*y)*stride + 2*x*4
			o := (y*w + x) * 4
			for k := range 4 {
				sum := uint32(
					src.Pix[i+k],
				) + uint32(
					src.Pix[i+4+k],
				) + uint32(
					src.Pix[i+stride+k],
				) + uint32(
					src.Pix[i+stride+4+k],
				)
				out[o+k] = u8((sum + 2) >> 2)
			}
		}
	}
	return Image{Pix: out, W: w, H: h, Opaque: src.Opaque}
}
