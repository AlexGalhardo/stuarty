package raster

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// The Rust rasterizer (renderer/vukem-renderer-text, SDD-013 section 3.11) is held byte for byte to
// this package. These vectors are the contract between the two: the specs and the bitmap this
// package draws for each. `cargo test` in renderer/ reads the same file. Regenerating it is a
// reviewed change of output (rule .agents/skills/renderer-parity):
// VUKEM_WRITE_RENDERER_VECTORS=1 go test ./engine/raster -run RasterVectors.
const rasterVectorsPath = "../../renderer/testdata/raster-vectors.bin"

type vectorCase struct {
	name string
	spec plan.BitmapSpec
}

func shapeVector(name string, scale float64, sh plan.ShapeSpec) vectorCase {
	return vectorCase{name, plan.BitmapSpec{Kind: KindShape, PixelScale: scale, Shape: &sh}}
}

func rasterVectorCases() []vectorCase {
	var out []vectorCase

	for _, c := range goldenCases() {
		if c.spec.Kind == KindShape {
			out = append(out, vectorCase{c.name, c.spec})
		}
	}

	white, black := C(255, 255, 255, 255), C(0, 0, 0, 255)

	return append(out,
		shapeVector("shape-rect-no-stroke", 1, plan.ShapeSpec{
			Shape: ShapeRect, Width: 100, Height: 40, Fill: C(10, 20, 30, 255),
		}),
		shapeVector("shape-rect-scale-2-fractional", 2, plan.ShapeSpec{
			Shape: ShapeRect, Width: 33.3, Height: 17.7, Fill: C(200, 10, 90, 180),
			StrokeColor: white, StrokeWidth: 3.5,
		}),
		shapeVector("shape-rounded-rect-radius-over-half", 1.5, plan.ShapeSpec{
			Shape: ShapeRoundedRect, Width: 80, Height: 30, Radius: 100, Fill: C(30, 144, 255, 230),
			StrokeColor: black, StrokeWidth: 5,
		}),
		shapeVector("shape-rounded-rect-radius-zero", 1, plan.ShapeSpec{
			Shape: ShapeRoundedRect, Width: 64, Height: 64, Radius: 0, Fill: white,
			StrokeColor: C(255, 0, 0, 128), StrokeWidth: 12,
		}),
		shapeVector("shape-ellipse-circle-small", 1, plan.ShapeSpec{
			Shape: ShapeEllipse, Width: 12, Height: 12, Fill: C(255, 255, 0, 255),
			StrokeColor: black, StrokeWidth: 1,
		}),
		shapeVector("shape-ellipse-wide-scale-3", 3, plan.ShapeSpec{
			Shape: ShapeEllipse, Width: 90, Height: 25, Fill: C(0, 128, 128, 255),
			StrokeColor: C(255, 255, 255, 90), StrokeWidth: 7,
		}),
		shapeVector("shape-rounded-rect-tiny", 1, plan.ShapeSpec{
			Shape: ShapeRoundedRect, Width: 5, Height: 3, Radius: 1, Fill: white,
		}),
	)
}

type vectorWriter struct{ bytes.Buffer }

func (w *vectorWriter) u32(v int)       { _ = binary.Write(w, binary.LittleEndian, uint32(v)) }
func (w *vectorWriter) f64(v float64)   { _ = binary.Write(w, binary.LittleEndian, math.Float64bits(v)) }
func (w *vectorWriter) str(s string)    { w.u32(len(s)); w.WriteString(s) }
func (w *vectorWriter) colour(c Colour) { w.Write([]byte{c.R, c.G, c.B, c.A}) }

// encodeRasterVectors is the file: "VRAS", the version, the number of cases, then per case its name,
// the kind (0 shape), the shape (kind index, width, height, radius, stroke width, fill, stroke
// colour, pixel scale) and the expected bitmap (width, height, natural width and height, centre
// offset, pixels).
func encodeRasterVectors(t *testing.T) []byte {
	t.Helper()

	cases := rasterVectorCases()

	var w vectorWriter

	w.WriteString("VRAS")
	w.u32(1)
	w.u32(len(cases))

	for _, c := range cases {
		w.str(c.name)
		w.u32(0)

		sh, err := resolveShape(*c.spec.Shape)
		if err != nil {
			t.Fatal(err)
		}

		kind := map[string]int{ShapeRect: 0, ShapeRoundedRect: 1, ShapeEllipse: 2}[sh.kind]
		w.u32(kind)
		w.f64(sh.width)
		w.f64(sh.height)
		w.f64(sh.radius)
		w.f64(sh.strokeWidth)
		w.colour(sh.fill)
		w.colour(sh.strokeColor)
		w.f64(c.spec.PixelScale)

		bm := render(t, c.spec)
		w.u32(bm.Width)
		w.u32(bm.Height)
		w.f64(bm.NaturalW)
		w.f64(bm.NaturalH)
		w.f64(bm.CenterDX)
		w.f64(bm.CenterDY)
		w.Write(bm.Pix)
	}

	return w.Bytes()
}

// TestRasterVectorsAreCurrent keeps renderer/testdata/raster-vectors.bin equal to what this package
// draws.
func TestRasterVectorsAreCurrent(t *testing.T) {
	got := encodeRasterVectors(t)

	if os.Getenv("VUKEM_WRITE_RENDERER_VECTORS") == "1" {
		if err := os.MkdirAll(filepath.Dir(rasterVectorsPath), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(rasterVectorsPath, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(rasterVectorsPath)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, want) {
		t.Fatal("renderer/testdata/raster-vectors.bin is out of date: regenerate it with " +
			"VUKEM_WRITE_RENDERER_VECTORS=1 and review the change")
	}
}
