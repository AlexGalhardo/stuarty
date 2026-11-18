package gioapp

import (
	"image"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func (ui *UI) preview(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, ui.canvas),
		layout.Rigid(ui.hline),
		layout.Rigid(ui.transport),
	)
}

func (ui *UI) canvas(gtx layout.Context) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return fill(gtx, ui.pal.canvas)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				box := gtx.Constraints.Max
				box.X -= gtx.Dp(unit.Dp(32))
				box.Y -= gtx.Dp(unit.Dp(32))

				if box.X < 1 {
					box.X = 1
				}

				if box.Y < 1 {
					box.Y = 1
				}

				frame := image.Pt(1920, 1080)
				if ui.doc != nil && ui.doc.Settings.Width > 0 && ui.doc.Settings.Height > 0 {
					frame = image.Pt(ui.doc.Settings.Width, ui.doc.Settings.Height)
				}

				size := fitBox(box, frame)

				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min = size
						paint.FillShape(gtx.Ops, ui.pal.panel, clip.Rect{Max: size}.Op())

						return layout.Dimensions{Size: size}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						if ui.frame.img != nil {
							return drawFitted(gtx, ui.frame.img, size)
						}

						hint := "Preview"
						if ui.frame.err != "" {
							hint = ellipsize(ui.frame.err, 48)
						}

						return layout.Center.Layout(gtx, lbl(ui.th, unit.Sp(13), ui.pal.muted, hint))
					}),
				)
			})
		}),
	)
}

func fitBox(box, frame image.Point) image.Point {
	if frame.X <= 0 || frame.Y <= 0 {
		return box
	}

	sx := float64(box.X) / float64(frame.X)
	sy := float64(box.Y) / float64(frame.Y)

	if sy < sx {
		sx = sy
	}

	w := int(float64(frame.X) * sx)
	h := int(float64(frame.Y) * sx)

	if w < 1 {
		w = 1
	}

	if h < 1 {
		h = 1
	}

	return image.Pt(w, h)
}

func drawFitted(gtx layout.Context, img image.Image, box image.Point) layout.Dimensions {
	b := img.Bounds().Size()
	if b.X < 1 || b.Y < 1 {
		return layout.Dimensions{Size: box}
	}

	sx := float32(box.X) / float32(b.X)
	sy := float32(box.Y) / float32(b.Y)

	if sy < sx {
		sx = sy
	}

	opImg := paint.NewImageOp(img)
	opImg.Filter = paint.FilterLinear
	defer op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(sx, sx))).Push(gtx.Ops).Pop()
	opImg.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	return layout.Dimensions{Size: box}
}

func (ui *UI) transport(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(unit.Dp(64))
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return fill(gtx, ui.pal.panel)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Left:  unit.Dp(12),
				Right: unit.Dp(12),
			}.Layout(
				gtx,
				func(gtx layout.Context) layout.Dimensions {
					play := "Play"
					if ui.playing {
						play = "Pause"
					}

					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if ui.clicked(gtx, "play") {
										ui.togglePlay()
									}

									return ui.button(gtx, ui.click("play"), play, ui.pal.accent, ui.pal.onAccent)
								}),
								gap(unit.Dp(12)),
								layout.Rigid(lbl(ui.th, unit.Sp(13), ui.pal.text, ui.tc(ui.playhead))),
								gap(unit.Dp(8)),
								layout.Rigid(lbl(ui.th, unit.Sp(12), ui.pal.muted, "/  "+ui.tc(ui.endFrame()))),
							)
						}),
						gap(unit.Dp(6)),
						layout.Rigid(ui.scrubber),
					)
				},
			)
		}),
	)
}

func (ui *UI) scrubber(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(unit.Dp(8))
	w := gtx.Constraints.Max.X
	size := image.Pt(w, h)

	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &ui.scrubTag)
	paint.FillShape(gtx.Ops, ui.pal.panel2, clip.Rect{Max: size}.Op())

	end := max(ui.endFrame(), 1)
	pos := min(w*ui.playhead/end, w)

	paint.FillShape(gtx.Ops, ui.pal.accent, clip.Rect{Max: image.Pt(pos, h)}.Op())

	ui.scrubEvents(gtx, w, end)

	return layout.Dimensions{Size: size}
}

func (ui *UI) scrubEvents(gtx layout.Context, width, end int) {
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &ui.scrubTag,
			Kinds:  pointer.Press | pointer.Drag,
		})
		if !ok {
			break
		}

		e, ok := ev.(pointer.Event)
		if !ok || width <= 0 {
			continue
		}

		x := min(max(int(e.Position.X), 0), width)

		ui.seek(x * end / width)
	}
}
