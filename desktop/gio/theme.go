package gioapp

import (
	"image"
	"image/color"

	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Colours follow a dark editing surface: near-black panels, a cyan accent on
// the export control, and one hue per item type on the timeline.
var (
	colBg      = rgb(0x101114)
	colPanel   = rgb(0x181B20)
	colPanel2  = rgb(0x22262E)
	colLine    = rgb(0x2E3440)
	colText    = rgb(0xF4F7FB)
	colMuted   = rgb(0x939AAB)
	colAccent  = rgb(0x22D3EE)
	colAccentD = rgb(0x0B3A44)
	colVideo   = rgb(0x3B82F6)
	colAudio   = rgb(0x22C55E)
	colTextIt  = rgb(0xA855F7)
	colShape   = rgb(0xF59E0B)
	colCaption = rgb(0xEC4899)
	colDanger  = rgb(0xF43F5E)
	colBlack   = rgb(0x050608)
	colWhite   = rgb(0xFFFFFF)
)

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: byte(c >> 16 & 0xFF), G: byte(c >> 8 & 0xFF), B: byte(c & 0xFF), A: 0xFF}
}

func theme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{
		Bg:         colPanel,
		Fg:         colText,
		ContrastBg: colAccent,
		ContrastFg: colBlack,
	}
	th.TextSize = unit.Sp(13)

	return th
}

func fill(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X <= 0 {
		size.X = gtx.Constraints.Min.X
	}

	if size.Y <= 0 {
		size.Y = gtx.Constraints.Min.Y
	}

	paint.FillShape(gtx.Ops, c, clip.Rect{Max: size}.Op())

	return layout.Dimensions{Size: size}
}

func roundRect(ops *op.Ops, c color.NRGBA, size image.Point, radius int) {
	rr := clip.RRect{Rect: image.Rectangle{Max: size}, NE: radius, NW: radius, SE: radius, SW: radius}
	paint.FillShape(ops, c, rr.Op(ops))
}

func vline(gtx layout.Context) layout.Dimensions {
	w := gtx.Dp(unit.Dp(1))
	h := gtx.Constraints.Max.Y
	paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: image.Pt(w, h)}.Op())

	return layout.Dimensions{Size: image.Pt(w, h)}
}

func hline(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(unit.Dp(1))
	w := gtx.Constraints.Max.X
	paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: image.Pt(w, h)}.Op())

	return layout.Dimensions{Size: image.Pt(w, h)}
}

func lbl(th *material.Theme, size unit.Sp, col color.NRGBA, txt string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(th, size, txt)
		l.Color = col

		return l.Layout(gtx)
	}
}

func gap(n unit.Dp) layout.FlexChild {
	return layout.Rigid(layout.Spacer{Width: n, Height: n}.Layout)
}

// button is a compact text button. Clicked is reported by the Clickable, which
// the caller checks before drawing.
func button(
	gtx layout.Context,
	th *material.Theme,
	click *widget.Clickable,
	txt string,
	bg, fg color.NRGBA,
) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		col := bg
		if click.Hovered() || click.Pressed() {
			col = mix(bg, colWhite, 0.12)
		}

		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				roundRect(gtx.Ops, col, size, gtx.Dp(unit.Dp(4)))

				return layout.Dimensions{Size: size}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.
					Layout(gtx, lbl(th, unit.Sp(12), fg, txt))
			}),
		)
	})
}

func chip(gtx layout.Context, th *material.Theme, click *widget.Clickable, txt string, on bool) layout.Dimensions {
	bg, fg := colPanel2, colMuted
	if on {
		bg, fg = colAccentD, colAccent
	}

	return button(gtx, th, click, txt, bg, fg)
}

func editorBox(gtx layout.Context, th *material.Theme, ed *widget.Editor, hint string) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			roundRect(gtx.Ops, colPanel2, size, gtx.Dp(unit.Dp(4)))

			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					e := material.Editor(th, ed, hint)
					e.Color = colText
					e.HintColor = colMuted

					return e.Layout(gtx)
				})
		}),
	)
}

func mix(a, b color.NRGBA, t float32) color.NRGBA {
	lerp := func(x, y uint8) uint8 {
		return uint8(float32(x) + (float32(y)-float32(x))*t)
	}

	return color.NRGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: a.A}
}

func ellipsize(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}

	if n < 2 {
		return "…"
	}

	return string(r[:n-1]) + "…"
}
