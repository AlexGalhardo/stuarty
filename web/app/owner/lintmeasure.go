package owner

import (
	"math"

	"github.com/AlexGalhardo/vukem-video/core/lint"
	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/raster"
)

// lintMeasurer is lint.Measurer over the plan and the rasterizer: the box of an
// item is the layer the plan draws at the item's first frame, sized by the
// rasterizer's own layout, with its rotation applied (SDD-004 section 4.2). The
// lint and the export therefore agree on where a text lands.
type lintMeasurer struct{ fonts *raster.FontSet }

var _ lint.Measurer = lintMeasurer{}

func (m lintMeasurer) Measure(doc *timeline.Document, it *timeline.Item) (lint.Rect, bool) {
	fp := plan.FrameWith(doc, it.Start, plan.Options{Measure: raster.Measure(m.fonts)})

	for i := range fp.Layers {
		l := &fp.Layers[i]
		if l.ItemID != it.ID {
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
