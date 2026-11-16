package compositor

// flipped is src mirrored across its vertical axis (flipX), its horizontal axis (flipY) or both, the
// mirror of SDD-004 section 3.10. The mirror is an exact permutation of pixels, so drawing it with the
// copy or with the sampler gives the pixels the unflipped layer has at (w - lx, ly) and (lx, h - ly).
// The result lives in memory that the next call reuses and never aliases src.
func (c *Compositor) flipped(src Image, flipX, flipY bool) Image {
	if !flipX && !flipY {
		return src
	}
	n := src.W * src.H * 4
	if cap(c.flipBuf) < n {
		c.flipBuf = make([]byte, n)
	}
	out := Image{Pix: c.flipBuf[:n], W: src.W, H: src.H, Opaque: src.Opaque}
	for y := range src.H {
		sy := y
		if flipY {
			sy = src.H - 1 - y
		}
		row := src.Pix[sy*src.W*4 : (sy+1)*src.W*4]
		dst := out.Pix[y*src.W*4 : (y+1)*src.W*4]
		if !flipX {
			copy(dst, row)
			continue
		}
		for x := range src.W {
			copy(dst[x*4:x*4+4], row[(src.W-1-x)*4:(src.W-x)*4])
		}
	}
	return out
}
