package lint

import (
	"fmt"
	"math"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Rect is a box in project pixels, rotation already applied.
type Rect struct{ X0, Y0, X1, Y1 float64 }

// Measurer gives the box of a visual item: for text and captions every line of the
// widest layout, with rotation and scale applied (SDD-004 §5). The rasterizer of
// engine/rasterize implements it for the service; core/lint does not import engine, so a
// test supplies a fake. ok is false when the item cannot be measured, and the rules
// that need the box then say nothing.
type Measurer interface {
	Measure(doc *timeline.Document, it *timeline.Item) (r Rect, ok bool)
}

// DocumentMeasurer is a Measurer that prepares once per document. Lint asks it
// for the measurer of a document at the start of each run; the returned
// Measurer is used for that run only, by one goroutine.
type DocumentMeasurer interface {
	Measurer
	ForDocument(doc *timeline.Document) Measurer
}

// boxOf is the box of an item: the Measurer's, or for text and captions the one made
// from Env.TextSize.
func (c *check) boxOf(it *timeline.Item, safe box) (box, bool) {
	if c.env.Measurer != nil {
		r, ok := c.env.Measurer.Measure(c.doc, it)

		return box{r.X0, r.Y0, r.X1, r.Y1}, ok
	}

	if c.env.TextSize == nil || (it.Type != timeline.ItemText && it.Type != timeline.ItemCaption) {
		return box{}, false
	}

	w, h, ok := c.env.TextSize(c.doc, it)
	if !ok {
		return box{}, false
	}

	return c.itemBox(it, w*it.Scale, h*it.Scale, safe), true
}

// safeRect is plan.SafeArea for the frame, the one place the margins exist.
func (c *check) safeRect() (box, bool) {
	if c.env.SafeArea == nil {
		return box{}, false
	}

	st := c.doc.Settings
	x, y, w, h := c.env.SafeArea(st.Width, st.Height)

	return box{float64(x), float64(y), float64(x + w), float64(y + h)}, true
}

// safeArea checks the box of every visible text and caption item against the safe
// rectangle of SDD-004 §4.1, for every frame size.
func (c *check) safeArea() {
	safe, ok := c.safeRect()
	if !ok || (c.env.Measurer == nil && c.env.TextSize == nil) {
		return
	}

	for _, it := range c.sorted() {
		if (it.Type != timeline.ItemText && it.Type != timeline.ItemCaption) || !c.visibleTrack(it.Track) {
			continue
		}

		// The box of an item that slides or animates is only one moment of it (VV-121).
		if movesInOrOut(it) {
			continue
		}

		b, ok := c.boxOf(it, safe)
		if !ok || (b.x0 >= safe.x0-edgeSlack && b.y0 >= safe.y0-edgeSlack && b.x1 <= safe.x1+edgeSlack &&
			b.y1 <= safe.y1+edgeSlack) {
			continue
		}

		msg, hint := c.safeAreaText(it, b, safe)
		c.add(CodeSafeArea, it.ID, itemRange(it), msg, hint)
	}
}

// safeAreaText words the first edge the box crosses, bottom first (the one that
// platform interfaces cover most).
func (c *check) safeAreaText(it *timeline.Item, b, safe box) (msg, hint string) {
	st := c.doc.Settings
	w, h := float64(st.Width), float64(st.Height)
	pct := func(margin, whole float64) int { return int(math.Round(margin / whole * 100)) }
	vertical := `set the caption track style position to "middle", or raise the item with item.set_transform y.`

	if it.Type != timeline.ItemCaption {
		vertical = "move the item inside the safe area with item.set_transform."
	}

	label := "caption text"
	if it.Type != timeline.ItemCaption {
		label = string(it.Type)
	}

	switch {
	case b.y1 > safe.y1+edgeSlack:
		return fmt.Sprintf("%s spans y %.0f-%.0f of %d; the safe area ends at y %.0f (%d%% bottom margin).",
			label, b.y0, b.y1, st.Height, safe.y1, pct(h-safe.y1, h)), vertical
	case b.y0 < safe.y0-edgeSlack:
		return fmt.Sprintf("%s spans y %.0f-%.0f of %d; the safe area starts at y %.0f (%d%% top margin).",
			label, b.y0, b.y1, st.Height, safe.y0, pct(safe.y0, h)), vertical
	case b.x1 > safe.x1+edgeSlack:
		return fmt.Sprintf(
			"%s spans x %.0f-%.0f of %d; the safe area ends at x %.0f (%d%% right margin).",
			label,
			b.x0,
			b.x1,
			st.Width,
			safe.x1,
			pct(w-safe.x1, w),
		), "move the item inside the safe area with item.set_transform x."
	}

	return fmt.Sprintf(
		"%s spans x %.0f-%.0f of %d; the safe area starts at x %.0f (%d%% left margin).",
		label,
		b.x0,
		b.x1,
		st.Width,
		safe.x0,
		pct(safe.x0, w),
	), "move the item inside the safe area with item.set_transform x."
}

// offFrameHint is the hint of an L_OFF_FRAME finding (the catalog keeps the short form, SKILL.md has a size cap): the origin is the top-left
// corner, which an agent that expects a centred origin gets wrong (VV-179).
const offFrameHint = "reposition it with item.set_transform; x and y are the item centre from the top-left corner"

// offFrame reports a visible item whose box lies partly or wholly outside the frame:
// any pixel for text and captions; wholly outside, or more than half outside, for clips
// and shapes, unless the box covers the whole frame (fit "cover") or the item is animated
// in position or size. It needs a Measurer.
func (c *check) offFrame() {
	if c.env.Measurer == nil {
		return
	}

	st := c.doc.Settings
	frame := box{0, 0, float64(st.Width), float64(st.Height)}

	for _, it := range c.sorted() {
		if it.Type == timeline.ItemAudio || !c.visibleTrack(it.Track) {
			continue
		}

		if movesInOrOut(it) {
			continue
		}

		b, ok := c.boxOf(it, frame)
		if !ok {
			continue
		}

		inside := overlapArea(b, frame)
		area := (b.x1 - b.x0) * (b.y1 - b.y0)
		textual := it.Type == timeline.ItemText || it.Type == timeline.ItemCaption

		if textual && b.x0 >= -edgeSlack && b.y0 >= -edgeSlack && b.x1 <= frame.x1+edgeSlack &&
			b.y1 <= frame.y1+edgeSlack {
			continue
		}

		if !textual && (area <= 0 || inside*2 >= area) {
			continue
		}

		// A box that covers the whole frame cuts nothing the viewer would see: this is the
		// overflow of a fit "cover" clip, which is the normal framing, not a mistake (VV-021).
		if !textual && b.x0 <= frame.x0 && b.y0 <= frame.y0 && b.x1 >= frame.x1 && b.y1 >= frame.y1 {
			continue
		}

		c.add(CodeOffFrame, it.ID, itemRange(it),
			fmt.Sprintf("the %s box spans x %.0f-%.0f and y %.0f-%.0f; the frame is %d by %d.",
				it.Type, b.x0, b.x1, b.y0, b.y1, st.Width, st.Height),
			offFrameHint)
	}
}

// movesInOrOut says whether the position or size of an item is animated, by keyframes
// or by a slide transition: the box measured from the stored values is then only one
// moment, and an item that enters from outside the frame is on purpose (VV-021).
func movesInOrOut(it *timeline.Item) bool {
	for _, p := range []string{"x", "y", "scale", "rotation"} {
		if len(it.Keyframes[p]) > 0 {
			return true
		}
	}

	for _, t := range []*timeline.Transition{it.TransitionIn, it.TransitionOut} {
		if t != nil && t.Kind != "fade" {
			return true
		}
	}

	return false
}

func overlapArea(a, b box) float64 {
	w := math.Min(a.x1, b.x1) - math.Max(a.x0, b.x0)
	h := math.Min(a.y1, b.y1) - math.Max(a.y0, b.y0)

	return math.Max(w, 0) * math.Max(h, 0)
}
