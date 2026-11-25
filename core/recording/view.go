package recording

import "math/big"

// Unit is the fixed-point scale of a position inside the source: ten-thousandths of its width or height. Every
// value that decides a keyframe is an integer, so the result is the same on every system (SDD-025 section 17).
const Unit = 10000

// A length on the frame is held in milli-pixels (1000 per pixel) and a scale in thousandths (1000 is 1x).
const (
	milli = 1000
	// ScaleOne is the scale of a clip that is not zoomed, in thousandths.
	ScaleOne = 1000
)

// View is how a clip lies on the frame of the project: the size of the frame, the size of the source of the clip
// and how the source is fitted into the frame before the scale, the position and the zoom of the item apply
// (SDD-001 section 3.5). The clip has no crop and is not rotated or flipped (the commands check it).
type View struct {
	FrameW, FrameH int
	SrcW, SrcH     int
	// Fit is "cover", "contain" or "none".
	Fit string
}

// Image is the size of the fitted source at scale 1, in milli-pixels.
func (v View) Image() (w, h int64) {
	fw, fh := int64(v.FrameW), int64(v.FrameH)
	sw, sh := int64(v.SrcW), int64(v.SrcH)

	// The side that decides the fit: the frame is wider than the source (relative to its height) or not.
	wide := fw*sh >= fh*sw

	switch v.Fit {
	case "none":
		return sw * milli, sh * milli
	case "contain":
		wide = !wide
	}

	if wide {
		return fw * milli, sh * fw * milli / sw
	}

	return sw * fh * milli / sh, fh * milli
}

// Visible is the share of the source the frame shows at scale 1, in Unit; each is at most Unit.
func (v View) Visible() (vw, vh int) {
	iw, ih := v.Image()

	vw = int(min(int64(Unit), int64(v.FrameW)*milli*Unit/max(iw, 1)))
	vh = int(min(int64(Unit), int64(v.FrameH)*milli*Unit/max(ih, 1)))

	return max(vw, 1), max(vh, 1)
}

// Centre is the centre of the item that puts the point (fx, fy) of the source (in Unit) at the centre of the
// frame when the item is scaled by scale (thousandths), in milli-pixels of the frame. The picture is held where
// it still covers the frame; on an axis where it is smaller than the frame the item stays centred.
func (v View) Centre(fx, fy int, scale int64) (x, y int64) {
	iw, ih := v.Image()

	return v.axisCentre(v.FrameW, iw, fx, scale), v.axisCentre(v.FrameH, ih, fy, scale)
}

func (View) axisCentre(frame int, image int64, focus int, scale int64) int64 {
	half := int64(frame) * milli / 2
	shown := image * scale / ScaleOne

	if shown <= int64(frame)*milli {
		return half
	}

	off := -(int64(focus) - Unit/2) * shown / Unit
	room := (shown - int64(frame)*milli) / 2

	return half + max(-room, min(room, off))
}

// Project is where the pixel (sx, sy) of the source is on the frame, in milli-pixels, when the item is centred on
// (cx, cy) milli-pixels and scaled by scale (thousandths). The point is the middle of the pixel.
func (v View) Project(sx, sy int, scale, cx, cy int64) (x, y int64) {
	return v.ProjectMilli(int64(2*sx+1)*milli/2, int64(2*sy+1)*milli/2, scale, cx, cy)
}

// ProjectMilli is Project for a point given in milli-pixels from the top left corner of the source, so that a
// position between two pixels (the smoothed pointer) keeps its fraction.
func (v View) ProjectMilli(ux, uy int64, scale, cx, cy int64) (x, y int64) {
	iw, ih := v.Image()

	x = cx + mulDiv3((2*ux-int64(v.SrcW)*milli), iw, scale, 2*int64(v.SrcW)*milli*ScaleOne)
	y = cy + mulDiv3((2*uy-int64(v.SrcH)*milli), ih, scale, 2*int64(v.SrcH)*milli*ScaleOne)

	return x, y
}

// mulDiv3 is a*b*c/d, truncated toward zero, without the overflow that a very wide source could cause.
func mulDiv3(a, b, c, d int64) int64 {
	n := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	n.Mul(n, big.NewInt(c))

	return n.Quo(n, big.NewInt(d)).Int64()
}
