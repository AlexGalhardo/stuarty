package compositor

import (
	"math"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// maxOutlineRadius is the largest outline radius in source pixels, the same bound as an effect length.
const maxOutlineRadius = 256

// shadowed draws the drop shadow and the outline of SDD-004 section 3.15 into the source, after the
// pixel effects and before the mask and the opacity. A shadow that multiplies to nothing and an
// outline whose radius rounds to 0 leave the source as it is, so a rotated layer is sampled the same.
func (c *Compositor) shadowed(src Image, p placement, l *plan.Layer, outScale float64) (Image, placement) {
	if l.Shadow == nil && l.Outline == nil || !(p.w > 0 && p.h > 0) || src.W < 1 || src.H < 1 {
		return src, p
	}

	var oc [4]byte
	ox, oy := 0, 0
	if l.Outline != nil {
		oc = parseRGBA(l.Outline.Color)
		if oc[3] != 0 {
			ox = min(srcLen(l.Outline.Width, outScale, src.W, p.w), maxOutlineRadius)
			oy = min(srcLen(l.Outline.Width, outScale, src.H, p.h), maxOutlineRadius)
		}
	}

	var sc [4]byte
	factor := uint32(0)
	dxs, dys, rx, ry := 0, 0, 0, 0
	if l.Shadow != nil {
		sc = parseRGBA(l.Shadow.Color)
		op := uint32(math.Round(min(max(l.Shadow.Opacity, 0), 1) * 255))
		factor = div255(op * uint32(sc[3]))
		if factor != 0 {
			dxs = srcLenSigned(l.Shadow.DX, outScale, src.W, p.w)
			dys = srcLenSigned(l.Shadow.DY, outScale, src.H, p.h)
			sigma := mul(l.Shadow.Blur, outScale) / 2
			rx = blurRadius(sigma / (p.w / float64(src.W)))
			ry = blurRadius(sigma / (p.h / float64(src.H)))
		}
	}
	if factor == 0 && ox == 0 && oy == 0 {
		return src, p
	}

	blurPadX, blurPadY := 3*rx, 3*ry
	padX := ox + blurPadX + iabs(dxs)
	padY := oy + blurPadY + iabs(dys)
	out := Image{Pix: make([]byte, (src.W+2*padX)*(src.H+2*padY)*4), W: src.W + 2*padX, H: src.H + 2*padY}

	var dilated []byte
	dw, dh := 0, 0
	if ox != 0 || oy != 0 {
		dilated, dw, dh = dilateAlpha(src, ox, oy)
	}
	if factor != 0 {
		sil := shadowSilhouette(src, dilated, dw, dh, ox, oy)
		spx, sw, sh := sil.Pix, sil.W, sil.H
		bpx, bpy := 0, 0
		if rx != 0 || ry != 0 {
			silP := placement{
				w: p.w * float64(sil.W) / float64(src.W),
				h: p.h * float64(sil.H) / float64(src.H),
			}
			blurred, _ := c.blurred(sil, silP, mul(l.Shadow.Blur, outScale)/2)
			spx = append([]byte(nil), blurred.Pix...)
			sw, sh = blurred.W, blurred.H
			bpx, bpy = (sw-sil.W)/2, (sh-sil.H)/2
		}
		paintColour(out, spx, sw, sh, padX-ox-bpx+dxs, padY-oy-bpy+dys, sc, factor)
	}
	if dilated != nil {
		paintColour(out, dilated, dw, dh, padX-ox, padY-oy, oc, uint32(oc[3]))
	}
	paintImage(out, src, padX, padY)

	return out, placement{
		cx: p.cx, cy: p.cy, rot: p.rot,
		w: mul(p.w, float64(out.W)) / float64(src.W),
		h: mul(p.h, float64(out.H)) / float64(src.H),
	}
}

// srcLen is a project length in source pixels, rounded half up, or 0 when it is not positive.
func srcLen(length, outScale float64, src int, drawn float64) int {
	n := srcLenSigned(length, outScale, src, drawn)
	if n < 0 {
		return 0
	}
	return n
}

func srcLenSigned(length, outScale float64, src int, drawn float64) int {
	if !(drawn > 0) {
		return 0
	}
	return round(length * outScale * float64(src) / drawn)
}

func iabs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// inDisc reports whether (dx, dy) is inside the axis-aligned ellipse of radii ox, oy, inclusive.
// A zero radius allows only that axis at 0, so a circle of radius 0 is the centre pixel.
func inDisc(dx, dy, ox, oy int) bool {
	switch {
	case ox == 0 && oy == 0:
		return dx == 0 && dy == 0
	case ox == 0:
		return dx == 0 && dy*dy <= oy*oy
	case oy == 0:
		return dy == 0 && dx*dx <= ox*ox
	default:
		dx2, dy2 := int64(dx)*int64(dx), int64(dy)*int64(dy)
		ox2, oy2 := int64(ox)*int64(ox), int64(oy)*int64(oy)
		return dx2*oy2+dy2*ox2 <= ox2*oy2
	}
}

// dilateAlpha is the outline image: each pixel is the largest source alpha inside the disc centred
// on the source position it covers. The image is larger than src by the radii on every side.
func dilateAlpha(src Image, ox, oy int) ([]byte, int, int) {
	w, h := src.W+2*ox, src.H+2*oy
	out := make([]byte, w*h*4)
	for y := range h {
		for x := range w {
			cx, cy := x-ox, y-oy
			maxA := byte(0)
			for dy := -oy; dy <= oy; dy++ {
				for dx := -ox; dx <= ox; dx++ {
					if !inDisc(dx, dy, ox, oy) {
						continue
					}
					sx, sy := cx+dx, cy+dy
					if sx < 0 || sy < 0 || sx >= src.W || sy >= src.H {
						continue
					}
					if a := src.Pix[(sy*src.W+sx)*4+3]; a > maxA {
						maxA = a
					}
				}
			}
			out[(y*w+x)*4+3] = maxA
		}
	}
	return out, w, h
}

// shadowSilhouette is the alpha the shadow blurs. With no outline it is the picture's alpha. With an
// outline it is the picture source-over the dilated outline, so a translucent pixel is not raised to
// the outline and an opaque one stays 255.
func shadowSilhouette(src Image, dilated []byte, dw, dh, ox, oy int) Image {
	if ox == 0 && oy == 0 {
		pix := make([]byte, src.W*src.H*4)
		for i := 3; i < len(src.Pix) && i < len(pix); i += 4 {
			pix[i] = src.Pix[i]
		}
		return Image{Pix: pix, W: src.W, H: src.H}
	}
	pix := make([]byte, dw*dh*4)
	for y := range dh {
		for x := range dw {
			ao := uint32(dilated[(y*dw+x)*4+3])
			ap := uint32(0)
			sx, sy := x-ox, y-oy
			if sx >= 0 && sy >= 0 && sx < src.W && sy < src.H {
				ap = uint32(src.Pix[(sy*src.W+sx)*4+3])
			}
			covered := ap + div255(ao*(255-ap))
			pix[(y*dw+x)*4+3] = byte(covered) //nolint:gosec // G115: at most 255
		}
	}
	return Image{Pix: pix, W: dw, H: dh}
}

// paintColour source-overs a recoloured alpha image. factor is the shadow's opacity weight, or the
// outline colour's alpha: the drawn alpha is div255(srcAlpha * factor).
func paintColour(dst Image, alpha []byte, w, h, x0, y0 int, color [4]byte, factor uint32) {
	if factor == 0 {
		return
	}
	for y := range h {
		dy := y0 + y
		if dy < 0 || dy >= dst.H {
			continue
		}
		for x := range w {
			dx := x0 + x
			if dx < 0 || dx >= dst.W {
				continue
			}
			a := div255(uint32(alpha[(y*w+x)*4+3]) * factor)
			if a == 0 {
				continue
			}
			overPixel(dst.Pix[(dy*dst.W+dx)*4:],
				div255(uint32(color[0])*a), div255(uint32(color[1])*a), div255(uint32(color[2])*a), a)
		}
	}
}

func paintImage(dst, src Image, x0, y0 int) {
	for y := range src.H {
		dy := y0 + y
		if dy < 0 || dy >= dst.H {
			continue
		}
		for x := range src.W {
			dx := x0 + x
			if dx < 0 || dx >= dst.W {
				continue
			}
			s := src.Pix[(y*src.W+x)*4:]
			a := uint32(s[3])
			if a == 0 {
				continue
			}
			overPixel(dst.Pix[(dy*dst.W+dx)*4:], uint32(s[0]), uint32(s[1]), uint32(s[2]), a)
		}
	}
}

// parseRGBA reads #RRGGBB (alpha 255) or #RRGGBBAA. Anything else is opaque black.
func parseRGBA(s string) [4]byte {
	out := [4]byte{0, 0, 0, 255}
	if (len(s) != 7 && len(s) != 9) || s[0] != '#' {
		return out
	}
	n := (len(s) - 1) / 2
	for i := range n {
		hi, ok1 := hexVal(s[1+2*i])
		lo, ok2 := hexVal(s[2+2*i])
		if !ok1 || !ok2 {
			return [4]byte{0, 0, 0, 255}
		}
		out[i] = hi<<4 | lo
	}
	return out
}
