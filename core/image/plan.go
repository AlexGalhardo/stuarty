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
	p.sources = clipSources(d)
	p.layers(d.Layers, 0, 0, 1, true, &fp)
	dropOrphanClips(&fp)

	return fp
}

type planner struct {
	d       *Document
	opts    plan.Options
	ps      float64
	sources map[string]bool // the layers that a visible layer takes its mask from (SDD-021 section 5)
}

func round3(x float64) float64 { return math.Round(x*1000)/1000 + 0 }

func (p *planner) layers(ls []Layer, dx, dy int, opacity float64, shown bool, fp *plan.FramePlan) {
	for i := range ls {
		l := &ls[i]
		vis := shown && l.Visible

		op := opacity * float64(l.Opacity) / 100

		if l.Kind == KindGroup {
			p.layers(l.Children, dx+l.X, dy+l.Y, op, vis, fp)

			continue
		}

		// The renderer skips a layer whose opacity rounds to no weight (SDD-013 section 3.3 step 2.1), so a source
		// with that opacity, its own or through its groups, is not drawn, like a hidden one.
		stencil := p.sources[l.ID] && (!vis || math.Round(round3(op)*255) == 0)

		switch {
		case stencil:
			// A hidden source still gives its shape to the layers that are masked by it: it is in the plan,
			// drawn by nobody, with the opacity 1 that the renderer needs to keep it (SDD-013 section 3.11.4).
			if pl, ok := p.layer(l, dx, dy, 1); ok {
				pl.Stencil = true
				fp.Layers = append(fp.Layers, pl)
			}
		case vis:
			if pl, ok := p.layer(l, dx, dy, op); ok {
				fp.Layers = append(fp.Layers, pl)
			}
		}
	}
}

// clipSources is the set of layers that a visible layer takes its mask from.
func clipSources(d *Document) map[string]bool {
	out := map[string]bool{}

	var visit func(ls []Layer, shown bool)

	visit = func(ls []Layer, shown bool) {
		for i := range ls {
			l := &ls[i]
			vis := shown && l.Visible

			if l.Kind == KindGroup {
				visit(l.Children, vis)

				continue
			}

			if vis && l.Mask != nil && l.Mask.Kind == MaskLayer {
				out[l.Mask.Source] = true
			}
		}
	}

	visit(d.Layers, true)

	return out
}

// dropOrphanClips removes the layers whose mask source is not in the plan (an image slot with no file yet
// draws nothing, so a layer clipped to it draws nothing either).
func dropOrphanClips(fp *plan.FramePlan) {
	for {
		have := map[string]bool{}
		for i := range fp.Layers {
			have[fp.Layers[i].ItemID] = true
		}

		kept := fp.Layers[:0]
		changed := false

		for i := range fp.Layers {
			if fp.Layers[i].Clip != nil && !have[fp.Layers[i].Clip.Source] {
				changed = true

				continue
			}

			kept = append(kept, fp.Layers[i])
		}

		fp.Layers = kept

		if !changed {
			return
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

	p.look(l, &out)

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
