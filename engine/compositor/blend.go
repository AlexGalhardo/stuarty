package compositor

// blendMode is the mix of a layer's pixel with the destination (SDD-004 section 3.8).
type blendMode uint8

const (
	blendNormal blendMode = iota
	blendMultiply
	blendScreen
	blendOverlay
	blendAdd
)

// blendOf maps the name of plan.Layer.Blend; empty and unknown names draw as normal.
func blendOf(name string) blendMode {
	switch name {
	case "multiply":
		return blendMultiply
	case "screen":
		return blendScreen
	case "overlay":
		return blendOverlay
	case "add":
		return blendAdd
	}
	return blendNormal
}

// mixPixel draws the premultiplied pixel (r, g, b, a), opacity already applied, over d. On an
// opaque destination the alpha is never changed by a blend mode and the formulas are those of
// SDD-004 section 3.8; on a destination with alpha below 255 (a transparent canvas) every mode but
// normal uses the general form of SDD-013 section 3.11.2, as the Rust renderer does.
func mixPixel(d []byte, r, g, b, a uint32, mode blendMode) {
	if mode == blendNormal {
		overPixel(d, r, g, b, a)
		return
	}
	if da := uint32(d[3]); da != 255 {
		mixGeneral(d, r, g, b, a, da, mode)
		return
	}
	d[0] = blendChannel(mode, r, uint32(d[0]), a)
	d[1] = blendChannel(mode, g, uint32(d[1]), a)
	d[2] = blendChannel(mode, b, uint32(d[2]), a)
}

// blendChannel is one destination channel dv after mixing with the premultiplied source channel c
// of alpha a. The overlay term is signed (a - c can be negative only through rounding), so it is
// computed on int64 and clamped.
func blendChannel(mode blendMode, c, dv, a uint32) uint8 {
	ia := 255 - a
	switch mode {
	case blendMultiply:
		return u8(div255(dv * (c + ia)))
	case blendScreen:
		return u8(c + dv - div255(c*dv))
	case blendOverlay:
		if dv <= 127 {
			return u8(div255(dv * (2*c + ia)))
		}
		v := int64(dv*ia) + (2*int64(dv)-255)*(int64(a)-int64(c))
		return u8(int64(c) + div255(max(v, 0)))
	case blendAdd:
		return u8(c + dv)
	case blendNormal:
	}
	return u8(dv)
}

// mixGeneral is the general form of SDD-013 section 3.11.2 for a destination of alpha da below
// 255: when da is 0 the pixel is the source, as for normal; otherwise each channel is
// div255(c x (255 - da)) + div255(dc x (255 - a)) + div255(t x B) with t = div255(a x da), B the
// blend function of the straight source and destination values, and the alpha is a + da - t.
func mixGeneral(d []byte, r, g, b, a, da uint32, mode blendMode) {
	if a == 0 {
		return
	}
	if da == 0 {
		d[0], d[1], d[2], d[3] = u8(r), u8(g), u8(b), u8(a)
		return
	}
	t := div255(a * da)
	for k, c := range [3]uint32{r, g, b} {
		dc := uint32(d[k])
		bf := blendFn(mode, straight(c, a), straight(dc, da))
		d[k] = u8(div255(c*(255-da)) + div255(dc*(255-a)) + div255(t*bf))
	}
	d[3] = u8(a + da - t)
}

// straight is the straight value of the premultiplied channel c of alpha a, which is above 0.
func straight(c, a uint32) uint32 {
	return min((c*255+a/2)/a, 255)
}

// blendFn is the separable blend function B(s, b) of the straight source value s and the straight
// destination value b, 0 to 255 (SDD-013 section 3.11.2).
func blendFn(mode blendMode, s, b uint32) uint32 {
	mul := func(x, y uint32) uint32 { return div255(x * y) }
	scr := func(x, y uint32) uint32 { return x + y - mul(x, y) }
	switch mode {
	case blendMultiply:
		return mul(s, b)
	case blendScreen:
		return scr(s, b)
	case blendOverlay:
		if b <= 127 {
			return mul(s, 2*b)
		}
		return scr(s, 2*b-255)
	case blendAdd:
		return min(s+b, 255)
	case blendNormal:
	}
	return s
}
