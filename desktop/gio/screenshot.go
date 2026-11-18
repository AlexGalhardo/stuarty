package gioapp

import (
	"image"
	"image/png"
	"os"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// RenderPNG draws one frame of the editor without a window.
func RenderPNG(ui *UI, path string, w, h int) error {
	win, err := headless.NewWindow(w, h)
	if err != nil {
		return err
	}

	defer win.Release()

	var ops op.Ops

	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, h)),
		Now:         time.Now(),
	}
	ui.Layout(gtx)

	if err := win.Frame(&ops); err != nil {
		return err
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := win.Screenshot(img); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	defer f.Close()

	return png.Encode(f, img)
}
