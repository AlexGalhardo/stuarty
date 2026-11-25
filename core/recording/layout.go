package recording

import (
	"errors"
	"math"
)

// The layouts of a screen take and a webcam take (SDD-025 section 20.1). A layout is a pure function of the
// frame, the size of the two assets and a few options; the command writes its result into the clips as ordinary
// values, so nothing remembers the layout. Only additions, subtractions, products, quotients, min and max are
// used, and every length is rounded to 3 decimals (the crop sides and the scale to 4, so a rounding stays under a fifth of a pixel), so the result is the same on every machine.

// Layout is the id of an arrangement (SDD-000 section 19).
type Layout string

// The four layouts, in the order of SDD-000 section 19.
const (
	LayoutScreenOnly  Layout = "screen_only"
	LayoutPip         Layout = "pip"
	LayoutSplit       Layout = "split"
	LayoutCameraFocus Layout = "camera_focus"
)

// Layouts lists the layouts in the order of SDD-000 section 19.
var Layouts = []Layout{LayoutScreenOnly, LayoutPip, LayoutSplit, LayoutCameraFocus}

// Corner is the corner a picture in picture or an inset sits in.
type Corner string

// The four corners.
const (
	CornerTopLeft     Corner = "top_left"
	CornerTopRight    Corner = "top_right"
	CornerBottomLeft  Corner = "bottom_left"
	CornerBottomRight Corner = "bottom_right"
)

// Corners lists the corners in the order of SDD-000 section 19.
var Corners = []Corner{CornerTopLeft, CornerTopRight, CornerBottomLeft, CornerBottomRight}

// CameraShape is the shape of a camera in a picture in picture.
type CameraShape string

// The two shapes.
const (
	ShapeCircle CameraShape = "circle"
	ShapeRect   CameraShape = "rect"
)

// Format is one of the four project formats a layout is made for.
type Format string

// The four formats.
const (
	Format16x9 Format = "16:9"
	Format1x1  Format = "1:1"
	Format4x5  Format = "4:5"
	Format9x16 Format = "9:16"
)

// MaxLayoutPadding is the most the padding can be, a fraction of the shorter side of the frame.
const MaxLayoutPadding = 0.25

var formatRatios = []struct {
	format Format
	ratio  float64
}{{Format16x9, 16.0 / 9}, {Format1x1, 1}, {Format4x5, 0.8}, {Format9x16, 9.0 / 16}}

// FormatOf is the nearest of the four formats to a frame, by the ratio between the two aspect ratios, so a custom
// frame behaves as its nearest format.
func FormatOf(width, height int) Format {
	if width <= 0 || height <= 0 {
		return Format16x9
	}

	a := float64(width) / float64(height)
	best, away := Format16x9, math.Inf(1)

	for _, f := range formatRatios {
		d := max(a/f.ratio, f.ratio/a)
		if d < away {
			best, away = f.format, d
		}
	}

	return best
}

// Size is the size of an asset in pixels.
type Size struct{ W, H float64 }

// Rect is a rectangle of the frame in pixels, from its top left corner.
type Rect struct{ X, Y, W, H float64 }

// Crop is the part of the asset taken off on each side, fractions from 0 to 1.
type Crop struct{ Left, Top, Right, Bottom float64 }

// IsZero reports whether nothing is cropped.
func (c Crop) IsZero() bool { return c == Crop{} }

// Placed is a clip put in a rectangle: the values the clip gets and the rectangle its picture then covers.
type Placed struct {
	CX, CY float64
	Scale  float64
	Crop   Crop
	// Shown is the rectangle the picture covers in the frame; it never leaves the rectangle it was placed in.
	Shown Rect
}

// LayoutRequest is what a layout is made from. Padding is a fraction of the shorter side of the frame.
type LayoutRequest struct {
	Layout    Layout
	Corner    Corner
	Shape     CameraShape
	Padding   float64
	FrameW    int
	FrameH    int
	Screen    Size
	Camera    Size
	HasCamera bool
}

// LayoutResult is a planned layout.
type LayoutResult struct {
	Format Format
	Screen Placed
	Camera *Placed
	// CameraInFront says which clip is drawn over the other: the camera for a picture in picture, the screen for
	// a camera focus, and neither (the two do not overlap) when the camera is nil or the layout splits the frame.
	CameraInFront bool
	Overlap       bool
	// Circle says the camera has an elliptical mask over its whole picture.
	Circle bool
}

// ErrLayoutInput says the request cannot be planned: a size of zero or a layout that does not exist.
var ErrLayoutInput = errors.New("recording: the layout request has no frame, no asset size or an unknown layout")

// pipFactor is the side of the camera square of a picture in picture, a fraction of the shorter side of the
// screen rectangle, for each format.
var pipFactor = map[Format]float64{Format16x9: 0.26, Format1x1: 0.30, Format4x5: 0.32, Format9x16: 0.36}

func floor4(v float64) float64 { return math.Floor(v*10000+1e-7) / 10000 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// place puts an asset in a rectangle of the frame: contain when cover is false, else cropped to its aspect. The
// scale is the largest value, rounded down to 4 decimals, with which the cropped picture stays inside the rectangle.
func place(r Rect, a Size, fw, fh float64, cover bool) Placed {
	var c Crop

	if cover {
		rho, alpha := r.W/r.H, a.W/a.H
		if alpha > rho {
			side := round4((1 - rho/alpha) / 2)
			c.Left, c.Right = side, side
		} else {
			side := round4((1 - alpha/rho) / 2)
			c.Top, c.Bottom = side, side
		}
	}

	cw, ch := a.W*(1-c.Left-c.Right), a.H*(1-c.Top-c.Bottom)
	b := min(fw/cw, fh/ch)
	scale := floor4(min(r.W/(cw*b), r.H/(ch*b)))
	w, h := cw*b*scale, ch*b*scale
	cx, cy := round3(r.X+r.W/2), round3(r.Y+r.H/2)

	return Placed{
		CX: cx, CY: cy, Scale: scale, Crop: c,
		Shown: Rect{X: round3(cx - w/2), Y: round3(cy - h/2), W: round3(w), H: round3(h)},
	}
}

// cornerRect puts a w by h rectangle in a corner of the outer one, inset by margin.
func cornerRect(outer Rect, w, h, margin float64, corner Corner) Rect {
	x, y := outer.X+margin, outer.Y+margin

	switch corner {
	case CornerTopRight:
		x = outer.X + outer.W - margin - w
	case CornerBottomLeft:
		y = outer.Y + outer.H - margin - h
	case CornerBottomRight:
		x, y = outer.X+outer.W-margin-w, outer.Y+outer.H-margin-h
	case CornerTopLeft:
	}

	return Rect{X: x, Y: y, W: w, H: h}
}

// PlanLayout makes the layout of a request.
func PlanLayout(q LayoutRequest) (LayoutResult, error) {
	if q.FrameW <= 0 || q.FrameH <= 0 || q.Screen.W <= 0 || q.Screen.H <= 0 || q.Padding < 0 ||
		q.Padding > MaxLayoutPadding ||
		(q.Layout != LayoutScreenOnly && (!q.HasCamera || q.Camera.W <= 0 || q.Camera.H <= 0)) {
		return LayoutResult{}, ErrLayoutInput
	}

	fw, fh := float64(q.FrameW), float64(q.FrameH)
	pad := round3(q.Padding * min(fw, fh))
	safe := Rect{X: pad, Y: pad, W: fw - 2*pad, H: fh - 2*pad}
	u := min(safe.W, safe.H)
	res := LayoutResult{Format: FormatOf(q.FrameW, q.FrameH)}
	wide := res.Format == Format16x9

	corner := q.Corner
	if corner == "" {
		corner = CornerBottomRight
	}

	switch q.Layout {
	case LayoutScreenOnly:
		res.Screen = place(safe, q.Screen, fw, fh, false)
	case LayoutPip:
		res.Screen = place(safe, q.Screen, fw, fh, false)
		s := res.Screen.Shown
		short := min(s.W, s.H)
		d := pipFactor[res.Format] * short
		cam := place(cornerRect(s, d, d, 0.04*short, corner), q.Camera, fw, fh, true)
		res.Camera, res.CameraInFront, res.Overlap = &cam, true, true
		res.Circle = q.Shape != ShapeRect
	case LayoutSplit:
		res.Screen, res.Camera = planSplit(q, safe, u, wide, fw, fh)
	case LayoutCameraFocus:
		cam := place(safe, q.Camera, fw, fh, true)
		w := 0.5 * safe.W
		if wide {
			w = 0.32 * safe.W
		}

		inset := cornerRect(safe, w, min(w*q.Screen.H/q.Screen.W, 0.5*safe.H), 0.03*u, corner)
		res.Screen = place(inset, q.Screen, fw, fh, false)
		res.Camera, res.Overlap = &cam, true
	default:
		return LayoutResult{}, ErrLayoutInput
	}

	return res, nil
}

// planSplit lays the two takes side by side in a wide frame and one above the other in the others.
func planSplit(q LayoutRequest, safe Rect, u float64, wide bool, fw, fh float64) (Placed, *Placed) {
	gap := 0.02 * u

	var screenPanel, cameraPanel Rect

	if wide {
		sw := (safe.W - gap) * 0.68
		screenPanel = Rect{X: safe.X, Y: safe.Y, W: sw, H: safe.H}
		cameraPanel = Rect{X: safe.X + sw + gap, Y: safe.Y, W: safe.W - gap - sw, H: safe.H}
	} else {
		sh := min(safe.W*q.Screen.H/q.Screen.W, 0.6*(safe.H-gap))
		screenPanel = Rect{X: safe.X, Y: safe.Y, W: safe.W, H: sh}
		cameraPanel = Rect{X: safe.X, Y: safe.Y + sh + gap, W: safe.W, H: safe.H - sh - gap}
	}

	cam := place(cameraPanel, q.Camera, fw, fh, true)

	return place(screenPanel, q.Screen, fw, fh, false), &cam
}
