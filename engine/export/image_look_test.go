package export

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/plan"
)

// The looks of an image layer (SDD-021 section 3.1, SDD-013 section 3.11) drawn by the export: every new
// operation changes the pixels, and a document that uses all of them exports to the same bytes twice (NFR-062).

func lookOp(name, args string) command.Operation {
	return command.Operation{Op: name, Args: json.RawMessage(args)}
}

// lookBase is the document the looks are applied to: a photo over a dark background, a fill, a bar and a text.
func lookBase() *coreimage.Document {
	d := imageDoc("image_and_group")
	d.Layers = append(d.Layers, coreimage.Layer{
		ID: lyr('p'), Kind: coreimage.KindShape, Name: "Stencil", Visible: true, Opacity: 100, X: 40, Y: 30, W: 120,
		H: 100, Shape: coreimage.ShapeEllipse, Fill: "#FFAA00",
	})
	// A box that is not white, so that every blend mode has something to mix.
	bar, _ := coreimage.Find(d, lyr('c'))
	bar.Fill = "#C0805A"

	coreimage.ReorderAll(d)

	return d
}

func firstLayerOfKind(d *coreimage.Document, kind string) string {
	var id string

	coreimage.Walk(d, func(l, _ *coreimage.Layer, _ int) {
		if id == "" && l.Kind == kind {
			id = l.ID
		}
	})

	return id
}

func applyLook(t *testing.T, d *coreimage.Document, ops ...command.Operation) *coreimage.Document {
	t.Helper()

	res, err := coreimage.ApplyBatch(d, ops, coreimage.Env{})
	if err != nil {
		t.Fatalf("%v: %v", ops, err)
	}

	return res.Doc
}

func renderLook(t *testing.T, d *coreimage.Document) *image.RGBA {
	t.Helper()

	src := sourcePNG(t)

	img, err := RenderImage(
		context.Background(),
		d,
		ImageConfig{AssetPath: func(string) (string, error) { return src, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}

	return img
}

// lookOps is one case per new operation of the epic, on the photo (or the layer the case names).
func lookOps(photo, bar, stencil string) map[string][]command.Operation {
	on := func(id, op, args string) command.Operation {
		return lookOp(op, `{"layer_id": "`+id+`", `+args+`}`)
	}

	return map[string][]command.Operation{
		"adjust_colour": {
			on(photo, "layer.adjust", `"brightness": 20, "contrast": 30, "saturation": -40, "hue": 25`),
		},
		"adjust_exposure": {on(photo, "layer.adjust", `"exposure": 80`)},
		"adjust_levels":   {on(photo, "layer.adjust", `"levels": {"black": 30, "white": 220, "gamma": 150}`)},
		"adjust_curve":    {on(photo, "layer.adjust", `"curve": [{"x": 64, "y": 30}, {"x": 192, "y": 230}]`)},
		"filter_colour":   {on(photo, "layer.filter_add", `"kind": "sepia"`)},
		"filter_blur":     {on(photo, "layer.filter_add", `"kind": "blur", "radius": 20`)},
		"filter_sharpen":  {on(photo, "layer.filter_add", `"kind": "sharpen", "amount": 200, "radius": 15`)},
		"filter_vignette": {on(photo, "layer.filter_add", `"kind": "vignette"`)},
		"filter_grain":    {on(photo, "layer.filter_add", `"kind": "grain", "amount": 60`)},
		"filter_shadow":   {on(bar, "layer.filter_add", `"kind": "shadow", "color": "#000000", "dx": 8, "dy": 8`)},
		"filter_glow":     {on(bar, "layer.filter_add", `"kind": "glow", "radius": 8, "color": "#FFFF00"`)},
		"filter_stroke":   {on(bar, "layer.filter_add", `"kind": "stroke", "color": "#FFFFFF", "width": 3`)},
		"mask_rect":       {on(photo, "layer.mask_set", `"kind": "rect", "x": 100, "w": 600`)},
		"mask_rounded":    {on(photo, "layer.mask_set", `"kind": "rounded_rect", "radius": 24, "feather": 4`)},
		"mask_ellipse":    {on(photo, "layer.mask_set", `"kind": "ellipse", "invert": true`)},
		"mask_layer":      {on(photo, "layer.mask_set", `"kind": "layer", "source": "`+stencil+`"`)},
		"blend_soft":      {on(bar, "layer.blend", `"mode": "soft_light"`)},
		"blend_hard":      {on(bar, "layer.blend", `"mode": "hard_light"`)},
		"blend_darken":    {on(bar, "layer.blend", `"mode": "darken"`)},
		"blend_lighten":   {on(bar, "layer.blend", `"mode": "lighten"`)},
		"blend_dodge":     {on(bar, "layer.blend", `"mode": "color_dodge"`)},
		"blend_burn":      {on(bar, "layer.blend", `"mode": "color_burn"`)},
		"blend_diff":      {on(bar, "layer.blend", `"mode": "difference"`)},
		"blend_multiply":  {on(bar, "layer.blend", `"mode": "multiply"`)},
		"blend_screen":    {on(bar, "layer.blend", `"mode": "screen"`)},
		"blend_overlay":   {on(bar, "layer.blend", `"mode": "overlay"`)},
	}
}

func TestImageLookEveryOperationChangesThePixels(t *testing.T) {
	base := lookBase()
	plain := renderLook(t, base)
	photo, bar, stencil := firstLayerOfKind(base, coreimage.KindImage), lyr('c'), lyr('p')

	for name, ops := range lookOps(photo, bar, stencil) {
		t.Run(name, func(t *testing.T) {
			got := renderLook(t, applyLook(t, base, ops...))
			if bytes.Equal(got.Pix, plain.Pix) {
				t.Fatalf("%s left the picture as it was", name)
			}
		})
	}
}

// lookAll applies every operation of lookOps that can live together on one document.
func lookAll(t *testing.T) *coreimage.Document {
	t.Helper()

	base := lookBase()
	photo, bar, stencil := firstLayerOfKind(base, coreimage.KindImage), lyr('c'), lyr('p')

	var ops []command.Operation

	for _, name := range []string{
		"adjust_colour", "adjust_exposure", "adjust_levels", "adjust_curve", "filter_sharpen", "filter_vignette",
		"filter_grain", "mask_layer", "blend_soft",
	} {
		all := lookOps(photo, bar, stencil)
		ops = append(ops, all[name]...)
	}

	for _, name := range []string{"filter_shadow", "filter_glow", "filter_stroke"} {
		ops = append(ops, lookOps(photo, bar, stencil)[name]...)
	}

	// The stencil is hidden: it is not drawn and still gives the shape the photo shows in.
	ops = append(ops, lookOp("layer.set", `{"layer_id": "`+stencil+`", "visible": false}`))

	return applyLook(t, base, ops...)
}

// TestImageLookRenderIsTheGoldenFileAndDeterministic: a document that uses adjustments, filters, a mask from a
// layer and a blend mode renders to its golden file, and exports to the same bytes twice (SDD-013 section
// 3.11, NFR-062).
func TestImageLookRenderIsTheGoldenFileAndDeterministic(t *testing.T) {
	d := lookAll(t)

	a, b := renderLook(t, d), renderLook(t, d)
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("two renders of the same document differ")
	}

	for _, o := range []ImageEncodeOptions{{Format: ImagePNG}, {Format: ImageJPEG}, {Format: ImagePNG, Width: 100, Height: 100}} {
		one, err := EncodeImage(context.Background(), a, o)
		if err != nil {
			t.Fatal(err)
		}

		two, err := EncodeImage(context.Background(), b, o)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(one, two) {
			t.Errorf("%+v: two exports differ", o)
		}
	}

	got := pngBytes(t, a)
	path := filepath.Join("testdata", "image", "look.png")

	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}

		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("the picture differs from its golden file (%d bytes against %d)", len(got), len(want))
	}
}

// TestImageLookHiddenMaskSourceStillMasks: a source that is hidden is not drawn and still gives its shape.
func TestImageLookHiddenMaskSourceStillMasks(t *testing.T) {
	base := lookBase()
	photo, stencil := firstLayerOfKind(base, coreimage.KindImage), lyr('p')
	masked := applyLook(
		t,
		base,
		lookOp("layer.mask_set", `{"layer_id": "`+photo+`", "kind": "layer", "source": "`+stencil+`"}`),
	)
	hidden := applyLook(t, masked, lookOp("layer.set", `{"layer_id": "`+stencil+`", "visible": false}`))

	a, b := renderLook(t, masked), renderLook(t, hidden)
	if bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("hiding the source changed nothing: it should stop drawing the source and keep the mask")
	}

	// The corner of the canvas is outside the stencil, so the photo does not show there in either picture.
	plain := renderLook(t, applyLook(t, base, lookOp("layer.set", `{"layer_id": "`+photo+`", "visible": false}`)))
	if got, want := b.RGBAAt(2, 2), plain.RGBAAt(2, 2); got != want {
		t.Errorf("the masked photo shows outside the stencil: %+v, the background is %+v", got, want)
	}
}

func TestWithoutLostClips(t *testing.T) {
	layers := []plan.Layer{
		{ItemID: "a"},
		{ItemID: "b", Clip: &plan.ClipSpec{Source: "a"}},
		{ItemID: "c", Clip: &plan.ClipSpec{Source: "gone"}},
		{ItemID: "d", Clip: &plan.ClipSpec{Source: "c"}}, // its source is dropped, so it goes too
	}

	got := withoutLostClips(layers)
	if len(got) != 2 || got[0].ItemID != "a" || got[1].ItemID != "b" {
		t.Fatalf("the layers kept are %+v", got)
	}
}
