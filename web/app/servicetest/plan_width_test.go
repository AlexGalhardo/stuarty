package servicetest

import (
	"bytes"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// The plan of a window at an output width hands out bitmaps rasterized at that scale, under keys
// that name the scale (SDD-004 section 10).
func TestPlanWindowWidthRasterizesAtTheOutputScale(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})
	project := m.exportable()
	ctx := m.as(ui)

	plan := func(width int) *service.PlanWindowResult {
		t.Helper()

		w, err := m.svc.PlanWindow(ctx, service.PlanWindowRequest{ProjectID: project, Until: 30, Width: width})
		if err != nil {
			t.Fatal(err)
		}

		return w
	}

	full, same, half := plan(0), plan(1080), plan(540)

	if !slices.Equal(full.Layers[0].Bitmaps.Keys, same.Layers[0].Bitmaps.Keys) {
		t.Errorf("the project width is the default: %v vs %v", full.Layers[0].Bitmaps.Keys, same.Layers[0].Bitmaps.Keys)
	}

	if slices.Equal(full.Layers[0].Bitmaps.Keys, half.Layers[0].Bitmaps.Keys) {
		t.Fatalf("the scale is not in the key: %v", half.Layers[0].Bitmaps.Keys)
	}

	pngWidth := func(key string) int {
		f, err := m.svc.OpenFile(ctx, service.FileRef{Kind: "bitmap", ProjectID: project, ID: key})
		if err != nil {
			t.Fatal(err)
		}

		raw, rerr := os.ReadFile(filepath.Join(m.ws, filepath.FromSlash(f.Path)))
		if rerr != nil {
			t.Fatal(rerr)
		}

		cfg, derr := png.DecodeConfig(bytes.NewReader(raw))
		if derr != nil {
			t.Fatal(derr)
		}

		return cfg.Width
	}

	wf, wh := pngWidth(full.Layers[0].Bitmaps.Keys[0]), pngWidth(half.Layers[0].Bitmaps.Keys[0])
	if wh >= wf || wh*3 < wf {
		t.Errorf("bitmap widths: full %d, half %d; the half plan must be about half as wide", wf, wh)
	}

	for _, width := range []int{-1, 15, 4097} {
		_, err := m.svc.PlanWindow(ctx, service.PlanWindowRequest{ProjectID: project, Until: 30, Width: width})

		var se *service.Error
		if !errors.As(err, &se) || se.Code != command.CodeValidation || se.Path != "width" {
			t.Errorf("width %d: %v", width, err)
		}
	}
}
