package gioapp

import (
	"image"
	"slices"
	"strconv"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

const (
	rulerDp = 22
	rowDp   = 36
	headDp  = 92
	edgeDp  = 6
)

type trackRow struct {
	id, name, kind, order string
	items                 []timeline.Item
}

func (ui *UI) timeline(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(ui.timelineBar),
		layout.Rigid(ui.hline),
		layout.Flexed(1, ui.timelineBody),
	)
}

func (ui *UI) timelineBar(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(unit.Dp(36))
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return fill(gtx, ui.pal.panel)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Left:  unit.Dp(8),
				Right: unit.Dp(8),
			}.Layout(
				gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						ui.tool("split", "Split", ui.split),
						gap(unit.Dp(6)),
						ui.tool("trim-in", "Trim in", func() { ui.trimToPlayhead(true) }),
						gap(unit.Dp(6)),
						ui.tool("trim-out", "Trim out", func() { ui.trimToPlayhead(false) }),
						gap(unit.Dp(6)),
						ui.tool("del", "Delete", ui.deleteSelected),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Min.Y)}
						}),
						layout.Rigid(lbl(ui.th, unit.Sp(12), ui.pal.muted, "Zoom")),
						gap(unit.Dp(6)),
						ui.tool("zoom-out", "−", func() { ui.zoom(0.8) }),
						gap(unit.Dp(4)),
						ui.tool("zoom-in", "+", func() { ui.zoom(1.25) }),
					)
				},
			)
		}),
	)
}

func (ui *UI) tool(id, txt string, fn func()) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if ui.clicked(gtx, "tool-"+id) {
			fn()
		}

		return ui.chip(gtx, ui.click("tool-"+id), txt, false)
	})
}

func (ui *UI) zoom(factor float64) {
	ui.pxPerSec *= factor
	if ui.pxPerSec < minPxPerSec {
		ui.pxPerSec = minPxPerSec
	}

	if ui.pxPerSec > maxPxPerSec {
		ui.pxPerSec = maxPxPerSec
	}
}

func (ui *UI) trimToPlayhead(startEdge bool) {
	it := ui.item(ui.selected)
	if it == nil {
		ui.status = "Select a clip to trim."

		return
	}

	edge := "end"
	to := ui.playhead

	if startEdge {
		edge = "start"
	}

	if startEdge && (to <= it.Start || to >= it.Start+it.Duration) {
		ui.status = "Move the playhead inside the clip."

		return
	}

	if !startEdge && (to <= it.Start || to > it.Start+it.Duration) {
		ui.status = "Move the playhead inside the clip."

		return
	}

	batch, err := operation("item.trim", map[string]any{"item_id": it.ID, "edge": edge, "to": framesArg(to)})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(batch)
}

func (ui *UI) timelineBody(gtx layout.Context) layout.Dimensions {
	rows := ui.trackRows()

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Dp(unit.Dp(headDp))
			gtx.Constraints = layout.Exact(image.Pt(w, gtx.Constraints.Max.Y))

			return ui.trackHeads(gtx, rows)
		}),
		layout.Rigid(ui.vline),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return ui.trackLanes(gtx, rows)
		}),
	)
}

func (ui *UI) trackRows() []trackRow {
	if ui.doc == nil {
		return nil
	}

	rows := make([]trackRow, 0, len(ui.doc.Tracks))

	for id, tr := range ui.doc.Tracks {
		row := trackRow{id: id, name: tr.Name, kind: string(tr.Kind), order: tr.Order}

		for itemID := range ui.doc.Items {
			it := ui.doc.Items[itemID]
			if it.Track == id {
				row.items = append(row.items, it)
			}
		}

		slices.SortFunc(row.items, func(a, b timeline.Item) int { return a.Start - b.Start })
		rows = append(rows, row)
	}

	slices.SortFunc(rows, func(a, b trackRow) int {
		if a.kind != b.kind {
			if a.kind == string(timeline.TrackAudio) {
				return 1
			}

			if b.kind == string(timeline.TrackAudio) {
				return -1
			}
		}

		if a.order < b.order {
			return -1
		}

		if a.order > b.order {
			return 1
		}

		return 0
	})

	return rows
}

func (ui *UI) trackHeads(gtx layout.Context, rows []trackRow) layout.Dimensions {
	fill(gtx, ui.pal.panel)
	ruler := gtx.Dp(unit.Dp(rulerDp))
	rowH := gtx.Dp(unit.Dp(rowDp))
	y := ruler - ui.scrollY

	visualN, audioN := 0, 0

	for i, row := range rows {
		label := row.name
		prefix := "V"
		visualN++
		n := visualN

		if row.kind == string(timeline.TrackAudio) {
			prefix = "A"
			audioN++
			n = audioN
			visualN--
		}

		if label == "" {
			label = "Video"
			if prefix == "A" {
				label = "Audio"
			}
		}

		label = prefix + strconv.Itoa(n) + "  " + label
		top := y + i*rowH

		if top+rowH < ruler || top > gtx.Constraints.Max.Y {
			continue
		}

		off := op.Offset(image.Pt(gtx.Dp(unit.Dp(8)), top+gtx.Dp(unit.Dp(8)))).Push(gtx.Ops)
		lbl(ui.th, unit.Sp(11), ui.pal.muted, ellipsize(label, 12))(gtx)
		off.Pop()
	}

	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (ui *UI) trackLanes(gtx layout.Context, rows []trackRow) layout.Dimensions {
	size := gtx.Constraints.Max
	ruler := gtx.Dp(unit.Dp(rulerDp))
	rowH := gtx.Dp(unit.Dp(rowDp))
	edge := gtx.Dp(unit.Dp(edgeDp))

	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &ui.timeTag)
	ui.paintTracks(gtx, rows, ruler, rowH, ui.scrollY)
	ui.paintClips(gtx, rows, ruler, rowH)
	ui.paintPlayhead(gtx)
	ui.paintRuler(gtx, ruler)
	ui.laneEvents(gtx, rows, size, ruler, rowH, edge)

	return layout.Dimensions{Size: size}
}

func (ui *UI) paintTracks(gtx layout.Context, rows []trackRow, ruler, rowH, scrollY int) {
	paint.FillShape(gtx.Ops, ui.pal.bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	width := gtx.Constraints.Max.X

	for i := range rows {
		y := ruler + i*rowH - scrollY
		col := ui.pal.panel
		if i%2 == 1 {
			col = ui.pal.panel2
		}

		paint.FillShape(gtx.Ops, col, clip.Rect{
			Min: image.Pt(0, y), Max: image.Pt(width, y+rowH-1),
		}.Op())
	}
}

func (ui *UI) paintClips(gtx layout.Context, rows []trackRow, ruler, rowH int) {
	fps := ui.fps()
	assets := ui.assetMap()

	for i, row := range rows {
		y := ruler + i*rowH - ui.scrollY + 4
		h := rowH - 8

		for j := range row.items {
			it := row.items[j]
			start, dur := ui.displaySpan(it)
			x := frameToX(start, fps, ui.pxPerSec) - ui.scrollX
			w := frameToX(start+dur, fps, ui.pxPerSec) - frameToX(start, fps, ui.pxPerSec)

			w = max(w, 2)

			if x+w < 0 || x > gtx.Constraints.Max.X {
				continue
			}

			col := ui.pal.clip(it.Type)
			if it.ID == ui.selected {
				col = mix(col, ui.pal.lift, 0.25)
			}

			off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			roundRect(gtx.Ops, col, image.Pt(w, h), gtx.Dp(unit.Dp(3)))
			off.Pop()

			if w > gtx.Dp(unit.Dp(36)) {
				off = op.Offset(image.Pt(x+gtx.Dp(unit.Dp(6)), y+gtx.Dp(unit.Dp(6)))).Push(gtx.Ops)
				lbl(ui.th, unit.Sp(11), ui.pal.onClip, ellipsize(itemLabel(it, assets), 18))(gtx)
				off.Pop()
			}
		}
	}
}

func (ui *UI) paintPlayhead(gtx layout.Context) {
	x := frameToX(ui.playhead, ui.fps(), ui.pxPerSec) - ui.scrollX
	if x < 0 || x > gtx.Constraints.Max.X {
		return
	}

	paint.FillShape(gtx.Ops, ui.pal.playhead, clip.Rect{
		Min: image.Pt(x, 0), Max: image.Pt(x+gtx.Dp(unit.Dp(1)), gtx.Constraints.Max.Y),
	}.Op())
}

func (ui *UI) paintRuler(gtx layout.Context, ruler int) {
	paint.FillShape(gtx.Ops, ui.pal.panel, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, ruler)}.Op())
	fps := ui.fps()
	if fps.Num <= 0 {
		return
	}

	secPx := max(frameToX(int(fps.Num), fps, ui.pxPerSec), 8)

	for x := -ui.scrollX % secPx; x < gtx.Constraints.Max.X; x += secPx {
		paint.FillShape(gtx.Ops, ui.pal.line, clip.Rect{
			Min: image.Pt(x, ruler-gtx.Dp(unit.Dp(8))), Max: image.Pt(x+1, ruler),
		}.Op())
	}
}

func (ui *UI) laneEvents(gtx layout.Context, rows []trackRow, size image.Point, ruler, rowH, edge int) {
	clips := ui.placed(rows)

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &ui.timeTag,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Scroll,
		})
		if !ok {
			break
		}

		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}

		switch e.Kind {
		case pointer.Scroll:
			ui.scrollX -= int(e.Scroll.X)
			ui.scrollY -= int(e.Scroll.Y)
			ui.clampScroll(rows, size, ruler, rowH)
		case pointer.Press:
			ui.pressLane(e, clips, ruler, rowH, edge)
			gtx.Execute(pointer.GrabCmd{Tag: &ui.timeTag, ID: e.PointerID})
		case pointer.Drag:
			ui.dragLane(e)
		case pointer.Release:
			ui.commitDrag()
		default:
		}
	}
}

func (ui *UI) placed(rows []trackRow) []placedClip {
	fps := ui.fps()
	clips := make([]placedClip, 0)

	for i, row := range rows {
		for j := range row.items {
			it := row.items[j]
			start, dur := ui.displaySpan(it)
			x := frameToX(start, fps, ui.pxPerSec) - ui.scrollX
			w := frameToX(start+dur, fps, ui.pxPerSec) - frameToX(start, fps, ui.pxPerSec)

			clips = append(clips, placedClip{id: it.ID, start: x, dur: w, row: i})
		}
	}

	return clips
}

func (ui *UI) pressLane(e pointer.Event, clips []placedClip, ruler, rowH, edge int) {
	x := int(e.Position.X)
	y := int(e.Position.Y)
	frame := xToFrame(x+ui.scrollX, ui.fps(), ui.pxPerSec)

	if y < ruler {
		ui.drag = dragState{}
		ui.seek(frame)

		return
	}

	h := hitTimeline(clips, x, y-ruler+ui.scrollY, 0, rowH, edge, func(int) int { return frame })
	ui.seek(frame)

	if h.kind == hitEmpty || h.itemID == "" {
		ui.selected = ""
		ui.drag = dragState{}

		return
	}

	ui.selected = h.itemID
	it := ui.item(h.itemID)
	ui.drag = dragState{kind: h.kind, item: h.itemID, active: true, originFrame: frame}

	if it != nil {
		ui.drag.originStart = it.Start
		ui.drag.originDur = it.Duration
	}
}

func (ui *UI) dragLane(e pointer.Event) {
	if !ui.drag.active {
		ui.seek(xToFrame(int(e.Position.X)+ui.scrollX, ui.fps(), ui.pxPerSec))

		return
	}

	frame := xToFrame(int(e.Position.X)+ui.scrollX, ui.fps(), ui.pxPerSec)
	ui.drag.delta = frame - ui.drag.originFrame
}

func (ui *UI) clampScroll(rows []trackRow, size image.Point, ruler, rowH int) {
	content := frameToX(ui.endFrame()+int(ui.fps().Num), ui.fps(), ui.pxPerSec)
	maxX := max(content-size.X, 0)
	ui.scrollX = min(max(ui.scrollX, 0), maxX)

	contentY := ruler + len(rows)*rowH
	maxY := max(contentY-size.Y, 0)
	ui.scrollY = min(max(ui.scrollY, 0), maxY)
}
