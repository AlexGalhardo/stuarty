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

// mixPixel draws the premultiplied pixel (r, g, b, a), opacity already applied, over d. The
// destination is opaque, so its alpha is never changed by a blend mode.
func mixPixel(d []byte, r, g, b, a uint32, mode blendMode) {
	if mode == blendNormal {
		overPixel(d, r, g, b, a)
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
