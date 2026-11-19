package plan

import (
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Nested compositions (SDD-004 §3.16): a composition item shows another timeline of the project. The planner of
// the nested timeline is made once per item; its layers either replace the item in the parent's plan (the item
// draws exactly as the nested timeline would on its own) or become one group layer.

// kid is the nested timeline of one composition item.
type kid struct {
	p *planner
	// end is the end frame of the nested timeline: it shows nothing at or after it.
	end int
	// flat says the item draws exactly as its nested timeline would on its own, so its layers are the parent's.
	flat bool
}

// nested is the nested timeline of a composition item, nil for any other item and for a timeline the planner
// cannot use: the caller gave no resolver or the id is unknown, the timeline is already on the way down (a loop
// that a stored project cannot have, SDD-001 §3.5.1), or the nesting would be deeper than the limit.
func (p *planner) nested(it *timeline.Item) *kid {
	if it.Type != timeline.ItemComposition {
		return nil
	}

	if k, ok := p.kids[it.ID]; ok {
		return k
	}

	k := p.makeKid(it)
	if p.kids == nil {
		p.kids = map[string]*kid{}
	}

	p.kids[it.ID] = k

	return k
}

func (p *planner) makeKid(it *timeline.Item) *kid {
	if p.opts.Nested == nil || len(p.trail) > timeline.MaxCompositionDepth || slices.Contains(p.trail, it.Timeline) {
		return nil
	}

	doc, ok := p.opts.Nested(it.Timeline)
	if !ok || doc == nil {
		return nil
	}

	st, ns := p.doc.Settings, doc.Settings
	opts := p.opts
	// The text of the nested layers is rasterized for the size it is drawn at: the output scale and the
	// scale the nested frame is placed with (SDD-004 §5.3). It only picks the resolution of a bitmap.
	opts.OutputScale = max(p.opts.OutputScale, 1) * it.Scale * fitFactor(it.Fit, st, ns)

	child := newPlanner(doc, opts)
	child.prefix = p.prefix + it.ID + "/"
	child.trail = append(slices.Clone(p.trail), it.Timeline)

	return &kid{p: child, end: endFrame(doc), flat: flattenable(it, &st, &ns)}
}

// fitFactor is the factor that the fit of SDD-004 §3.4 gives the nested frame of size ns in a frame of size st.
func fitFactor(fit string, st, ns timeline.Settings) float64 {
	fw, fh, cw, ch := float64(st.Width), float64(st.Height), float64(ns.Width), float64(ns.Height)
	if cw <= 0 || ch <= 0 {
		return 1
	}

	switch fit {
	case "cover":
		return max(fw/cw, fh/ch)
	case "contain":
		return min(fw/cw, fh/ch)
	}

	return 1
}

// flattenable says the composition item draws exactly as its nested timeline would on its own: nothing of its
// own changes the picture and the nested frame lands on the parent's frame pixel for pixel (SDD-004 §3.16). It
// reads the stored item, never a frame, so all the layers of one item are flat or all are grouped.
func flattenable(it *timeline.Item, parent, nested *timeline.Settings) bool {
	return nested.Width == parent.Width && nested.Height == parent.Height && timeline.CompositionFeature(it) == ""
}

// frameAt is the frame of the nested timeline that frame r of the item shows, by the rule of SDD-004 §3.3 with
// the frame rate of the nested timeline as the asset's; false when the item is past the end of it.
func (k *kid) frameAt(it *timeline.Item, r int, fps timeline.Rational) (int, bool) {
	nf := k.p.doc.Settings.FPS
	if fps.Num == 0 || nf.Den == 0 || nf.Num == 0 {
		return 0, false
	}

	pos := int64(it.SourceIn) + int64(it.SourceOffset(r))
	cf := int(pos * int64(fps.Den) * int64(nf.Num) / (int64(fps.Num) * int64(nf.Den)))

	return cf, cf >= 0 && cf < k.end
}

// layers are the layers of the nested frame that frame r of a flat item shows, in the parent's plan.
func (k *kid) layers(it *timeline.Item, r int, fps timeline.Rational) []Layer {
	cf, ok := k.frameAt(it, r, fps)
	if !ok {
		return nil
	}

	return k.p.frame(cf).Layers
}

// groupLayer places a composition item that draws as one unit: a group layer with the placement of a media
// layer of a picture of the nested frame's size (SDD-004 §3.16).
func (p *planner) groupLayer(l Layer, it *timeline.Item, r int, a animated) (Layer, bool) {
	k := p.nested(it)
	if k == nil {
		return Layer{}, false
	}

	cf, ok := k.frameAt(it, r, p.doc.Settings.FPS)
	if !ok {
		return Layer{}, false
	}

	ns := k.p.doc.Settings
	b := fitFactor(it.Fit, p.doc.Settings, ns)
	cw, ch := float64(ns.Width), float64(ns.Height)

	l.Kind = Group
	l.Crop = Rect{W: cw, H: ch}
	l.CX, l.CY = round3(a.x), round3(a.y)
	l.W, l.H = round3(cw*b*a.scale), round3(ch*b*a.scale)
	l.Group = &GroupSpec{Width: ns.Width, Height: ns.Height, Layers: k.p.frame(cf).Layers}

	return l, true
}

// lookup finds the item a layer id names, in the planner that owns it: an id is an item of this document, or a
// path "<composition item>/<id in the nested timeline>" (SDD-004 §3.16).
func (p *planner) lookup(id string) (timeline.Item, *planner, bool) {
	head, rest, nested := strings.Cut(id, "/")

	it, ok := p.doc.Items[head]
	if !ok {
		return timeline.Item{}, nil, false
	}

	if !nested {
		return it, p, true
	}

	k := p.kids[head]
	if k == nil {
		return timeline.Item{}, nil, false
	}

	return k.p.lookup(rest)
}
