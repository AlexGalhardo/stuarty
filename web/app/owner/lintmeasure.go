package owner

import (
	"encoding/json"
	"math"

	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
	"github.com/AlexGalhardo/stuarty/service"
)

// lintMeasurer is lint.Measurer over the plan and the rasterizer: the box of an
// item is the layer the plan draws at the item's first frame, sized by the
// rasterizer's own layout, with its rotation applied (SDD-004 section 4.2). The
// lint and the export therefore agree on where a text lands.
type lintMeasurer struct{ workspace string }

var _ lint.Measurer = lintMeasurer{}

var _ lint.DocumentMeasurer = lintMeasurer{}

// Measure plans the frame of the item on its own: the fallback for a document that
// was not prepared with ForDocument.
func (m lintMeasurer) Measure(doc *timeline.Document, it *timeline.Item) (lint.Rect, bool) {
	fp := plan.FrameWith(doc, it.Start, plan.Options{Measure: rasterize.Measure(service.FontSetFor(m.workspace, doc))})

	return boxInPlan(&fp, it.ID)
}

// ForDocument prepares the font set and the frame planner once; the returned
// measurer also remembers each frame and box it computed. It serves one lint run.
func (m lintMeasurer) ForDocument(doc *timeline.Document) lint.Measurer {
	measure := memoMeasure(rasterize.Measure(service.FontSetFor(m.workspace, doc)))

	return &docMeasurer{
		fallback: m,
		doc:      doc,
		planner:  plan.NewPlanner(doc, plan.Options{Measure: measure}),
		frames:   map[int]*plan.FramePlan{},
		boxes:    map[string]boxResult{},
	}
}

// memoMeasure remembers the natural size of each bitmap spec: shaping is the cost of a
// frame plan, and many frames of a lint run hold the same text. The size is a pure
// function of the spec for one font set.
func memoMeasure(inner plan.Measurer) plan.Measurer {
	type size struct{ w, h float64 }

	seen := map[string]size{}

	return func(spec plan.BitmapSpec) (w, h float64) {
		key, err := json.Marshal(spec)
		if err != nil {
			return inner(spec)
		}

		if v, ok := seen[string(key)]; ok {
			return v.w, v.h
		}

		w, h = inner(spec)
		seen[string(key)] = size{w, h}

		return w, h
	}
}

type boxResult struct {
	rect lint.Rect
	ok   bool
}

// docMeasurer is the measurer of one document for one lint run (one goroutine).
type docMeasurer struct {
	fallback lintMeasurer
	doc      *timeline.Document
	planner  *plan.Planner
	frames   map[int]*plan.FramePlan
	boxes    map[string]boxResult
}

func (d *docMeasurer) Measure(doc *timeline.Document, it *timeline.Item) (lint.Rect, bool) {
	if doc != d.doc {
		return d.fallback.Measure(doc, it)
	}

	if r, ok := d.boxes[it.ID]; ok {
		return r.rect, r.ok
	}

	fp, ok := d.frames[it.Start]
	if !ok {
		p := d.planner.Frame(it.Start)
		fp = &p
		d.frames[it.Start] = fp
	}

	rect, found := boxInPlan(fp, it.ID)
	d.boxes[it.ID] = boxResult{rect: rect, ok: found}

	return rect, found
}

// boxInPlan is the box of the layer of an item in a frame plan, rotation applied.
func boxInPlan(fp *plan.FramePlan, itemID string) (lint.Rect, bool) {
	for i := range fp.Layers {
		l := &fp.Layers[i]
		if l.ItemID != itemID {
			continue
		}

		if l.W <= 0 || l.H <= 0 {
			return lint.Rect{}, false
		}

		rad := l.Rotation * math.Pi / 180
		hw := (math.Abs(l.W*math.Cos(rad)) + math.Abs(l.H*math.Sin(rad))) / 2
		hh := (math.Abs(l.W*math.Sin(rad)) + math.Abs(l.H*math.Cos(rad))) / 2

		return lint.Rect{X0: l.CX - hw, Y0: l.CY - hh, X1: l.CX + hw, Y1: l.CY + hh}, true
	}

	return lint.Rect{}, false
}
