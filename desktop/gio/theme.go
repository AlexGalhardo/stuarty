package gioapp

import (
	"encoding/json"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"

	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// palette is every colour the window paints. Clip hues stay saturated in both
// themes so a block reads the same; the chrome around them switches.
type palette struct {
	bg, panel, panel2 color.NRGBA
	line, text, muted color.NRGBA
	accent, accentOn  color.NRGBA
	onAccent          color.NRGBA
	canvas, playhead  color.NRGBA
	video, audio      color.NRGBA
	textItem, shape   color.NRGBA
	caption, danger   color.NRGBA
	onClip, lift      color.NRGBA
}

func darkPalette() palette {
	return palette{
		bg: rgb(0x101114), panel: rgb(0x181B20), panel2: rgb(0x22262E),
		line: rgb(0x2E3440), text: rgb(0xF4F7FB), muted: rgb(0x939AAB),
		accent: rgb(0x22D3EE), accentOn: rgb(0x0B3A44), onAccent: rgb(0x050608),
		canvas: rgb(0x050608), playhead: rgb(0xFFFFFF),
		video: rgb(0x3B82F6), audio: rgb(0x22C55E), textItem: rgb(0xA855F7),
		shape: rgb(0xF59E0B), caption: rgb(0xEC4899), danger: rgb(0xF43F5E),
		onClip: rgb(0xFFFFFF), lift: rgb(0xFFFFFF),
	}
}

func lightPalette() palette {
	return palette{
		bg: rgb(0xF3F4F6), panel: rgb(0xFFFFFF), panel2: rgb(0xE6E8EC),
		line: rgb(0xD0D5DD), text: rgb(0x1A1D23), muted: rgb(0x4B5563),
		accent: rgb(0x0E7490), accentOn: rgb(0xCFFAFE), onAccent: rgb(0xFFFFFF),
		canvas: rgb(0xE4E7EC), playhead: rgb(0x111827),
		video: rgb(0x2563EB), audio: rgb(0x15803D), textItem: rgb(0x7E22CE),
		shape: rgb(0xC2410C), caption: rgb(0xBE185D), danger: rgb(0xE11D48),
		onClip: rgb(0xFFFFFF), lift: rgb(0x111827),
	}
}

func (p palette) clip(t timeline.ItemType) color.NRGBA {
	switch t {
	case timeline.ItemAudio:
		return p.audio
	case timeline.ItemText:
		return p.textItem
	case timeline.ItemCaption:
		return p.caption
	case timeline.ItemShape:
		return p.shape
	default:
		return p.video
	}
}

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: byte(c >> 16 & 0xFF), G: byte(c >> 8 & 0xFF), B: byte(c & 0xFF), A: 0xFF}
}

func newMaterial(p palette) *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.TextSize = unit.Sp(13)
	paintTheme(th, p)

	return th
}

func paintTheme(th *material.Theme, p palette) {
	th.Palette = material.Palette{
		Bg: p.panel, Fg: p.text, ContrastBg: p.accent, ContrastFg: p.onAccent,
	}
}

func (ui *UI) vline(gtx layout.Context) layout.Dimensions {
	w := gtx.Dp(unit.Dp(1))
	h := gtx.Constraints.Max.Y
	paint.FillShape(gtx.Ops, ui.pal.line, clip.Rect{Max: image.Pt(w, h)}.Op())

	return layout.Dimensions{Size: image.Pt(w, h)}
}

func (ui *UI) hline(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(unit.Dp(1))
	w := gtx.Constraints.Max.X
	paint.FillShape(gtx.Ops, ui.pal.line, clip.Rect{Max: image.Pt(w, h)}.Op())

	return layout.Dimensions{Size: image.Pt(w, h)}
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

func (ui *UI) button(gtx layout.Context, click *widget.Clickable, txt string, bg, fg color.NRGBA) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		col := bg
		if click.Hovered() || click.Pressed() {
			col = mix(bg, ui.pal.lift, 0.12)
		}

		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				roundRect(gtx.Ops, col, size, gtx.Dp(unit.Dp(4)))

				return layout.Dimensions{Size: size}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.
					Layout(gtx, lbl(ui.th, unit.Sp(12), fg, txt))
			}),
		)
	})
}

func (ui *UI) chip(gtx layout.Context, click *widget.Clickable, txt string, on bool) layout.Dimensions {
	bg, fg := ui.pal.panel2, ui.pal.muted
	if on {
		bg, fg = ui.pal.accentOn, ui.pal.accent
	}

	return ui.button(gtx, click, txt, bg, fg)
}

func (ui *UI) editorBox(gtx layout.Context, ed *widget.Editor, hint string) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			roundRect(gtx.Ops, ui.pal.panel2, size, gtx.Dp(unit.Dp(4)))

			return layout.Dimensions{Size: size}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					e := material.Editor(ui.th, ed, hint)
					e.Color = ui.pal.text
					e.HintColor = ui.pal.muted

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

// gioConfig is the Gio window's own preferences. It is not the Wails shell's
// desktop.json, so a theme choice never turns the update check on.
type gioConfig struct {
	Theme string `json:"theme"`
}

func defaultGioConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "vukem-video", "gio.json"), nil
}

func (ui *UI) themePath() (string, error) {
	if ui.configPath != "" {
		return ui.configPath, nil
	}

	return defaultGioConfigPath()
}

func (ui *UI) loadTheme() {
	ui.light = false
	ui.pal = darkPalette()
	path, err := ui.themePath()
	if err != nil {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var cfg gioConfig
	if json.Unmarshal(data, &cfg) != nil {
		return
	}

	if cfg.Theme == "light" {
		ui.light = true
		ui.pal = lightPalette()
	}
}

func (ui *UI) useTheme(light, save bool) {
	ui.light = light
	ui.pal = darkPalette()

	if light {
		ui.pal = lightPalette()
	}

	if ui.th != nil {
		paintTheme(ui.th, ui.pal)
	}

	if save {
		_ = ui.saveTheme()
	}
}

func (ui *UI) toggleTheme() {
	ui.useTheme(!ui.light, true)
}

func (ui *UI) saveTheme() error {
	path, err := ui.themePath()
	if err != nil {
		return err
	}

	name := "dark"
	if ui.light {
		name = "light"
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, err := json.Marshal(gioConfig{Theme: name})
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)

		return err
	}

	return nil
}

func (ui *UI) themeWord() string {
	if ui.light {
		return "Dark"
	}

	return "Light"
}

// contrast is the WCAG ratio of two opaque colours, used by the theme test.
func contrast(a, b color.NRGBA) float64 {
	la, lb := relLuma(a), relLuma(b)
	if la < lb {
		la, lb = lb, la
	}

	return (la + 0.05) / (lb + 0.05)
}

func relLuma(c color.NRGBA) float64 {
	ch := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}

		return math.Pow((s+0.055)/1.055, 2.4)
	}

	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}
