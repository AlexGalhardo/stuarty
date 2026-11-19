package image

import (
	"math"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// Plan is the one frame of an image in the format of SDD-013 section 2 (SDD-021 section 5): the renderer needs
// no concept for it that a video does not have. Hidden layers make no plan layer, group offsets are added to
// the coordinates of the children and group opacities multiply theirs. The numbers are rounded to three
// decimals, as in the plan of a video, so the same document gives the same plan on every machine.
func Plan(d *Document, opts plan.Options) plan.FramePlan {
	fp := plan.FramePlan{
		Frame: 0, Width: d.Width, Height: d.Height, Background: "#000000", Layers: []plan.Layer{},
	}

	if d.Background == Transparent {
		fp.Transparent = true
	} else if len(d.Background) >= 7 {
		fp.Background = d.Background[:7] // an alpha in the colour is ignored, as in a video (SDD-004 section 6)
	}

	p := &planner{d: d, opts: opts, ps: plan.PixelScale(1, opts.OutputScale)}
	p.layers(d.Layers, 0, 0, 1, &fp)

	return fp
}

type planner struct {
	d    *Document
	opts plan.Options
	ps   float64
}

func round3(x float64) float64 { return math.Round(x*1000)/1000 + 0 }

func (p *planner) layers(ls []Layer, dx, dy int, opacity float64, fp *plan.FramePlan) {
	for i := range ls {
		l := &ls[i]
		if !l.Visible {
			continue
		}

		op := opacity * float64(l.Opacity) / 100

		if l.Kind == KindGroup {
			p.layers(l.Children, dx+l.X, dy+l.Y, op, fp)

			continue
		}

		if pl, ok := p.layer(l, dx, dy, op); ok {
			fp.Layers = append(fp.Layers, pl)
		}
	}
}

// layer is the plan layer of a layer that is not a group, in the coordinates of the canvas.
func (p *planner) layer(l *Layer, dx, dy int, opacity float64) (plan.Layer, bool) {
	out := plan.Layer{
		ItemID: l.ID, Rotation: float64(l.Rotation), Scale: 1, Opacity: round3(opacity),
		CX: round3(float64(l.X+dx) + float64(l.W)/2), CY: round3(float64(l.Y+dy) + float64(l.H)/2),
		W: float64(l.W), H: float64(l.H),
	}

	switch l.Kind {
	case KindImage:
		src, ok := p.d.Assets[l.Asset]
		if !ok {
			return plan.Layer{}, false // an image slot that has no file yet draws nothing
		}

		c := cropOf(l, src)
		out.Kind, out.AssetID, out.AssetKind = plan.Media, l.Asset, "image"
		out.Crop = plan.Rect{X: float64(c.X), Y: float64(c.Y), W: float64(c.W), H: float64(c.H)}
	case KindShape, KindFill:
		shape, stroke := l.Shape, l.StrokeColor

		if l.Kind == KindFill {
			shape = ShapeRect
		}

		if stroke == "" {
			stroke = "#000000"
		}

		out.Kind = plan.Bitmap
		out.Bitmap = &plan.BitmapSpec{
			Kind: "shape", ActiveWord: -1, PixelScale: p.ps,
			Shape: &plan.ShapeSpec{
				Shape: shape, Width: float64(l.W), Height: float64(l.H), Radius: float64(l.Radius), Fill: l.Fill,
				StrokeColor: stroke, StrokeWidth: float64(l.StrokeWidth),
			},
		}
	case KindText:
		p.text(l, dx, dy, &out)
	default:
		return plan.Layer{}, false
	}

	return out, true
}

// text sets the bitmap of a text layer and places its block in the box: at the top, and by the style's align at
// the left, the centre or the right. The measurer gives the natural size of the block; without one the block is
// the box.
func (p *planner) text(l *Layer, dx, dy int, out *plan.Layer) {
	spec := textSpec(p.d, l, p.ps)

	out.Kind, out.Bitmap = plan.Bitmap, &spec

	if p.opts.Measure == nil {
		return
	}

	m := measureText(&Env{Measure: p.opts.Measure}, p.d, l)
	bx, by := float64(l.X+dx), float64(l.Y+dy)
	cx := bx + float64(l.W)/2

	switch spec.Style.Align {
	case "left":
		cx = bx + m.w/2
	case "right":
		cx = bx + float64(l.W) - m.w/2
	}

	out.CX, out.CY, out.W, out.H = round3(cx), round3(by+m.h/2), round3(m.w), round3(m.h)
}
