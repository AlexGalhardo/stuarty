package raster

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// The last bit of a variable font's coordinates depends on whether the compiler
// fuses multiply-adds. Rounding to a font unit makes two outlines that differ
// by a float32 ulp come out identical.
func TestOutlineAbsorbsTheLastBitOfFloat32(t *testing.T) {
	t.Parallel()

	outline := func(dx float32) font.GlyphOutline {
		pt := func(x, y float32) ot.SegmentPoint { return ot.SegmentPoint{X: x + dx, Y: y + dx} }

		return font.GlyphOutline{Segments: []ot.Segment{
			{Op: ot.SegmentOpMoveTo, Args: [3]ot.SegmentPoint{pt(345.6639, 1571.9387)}},
			{Op: ot.SegmentOpQuadTo, Args: [3]ot.SegmentPoint{pt(394.08057, 1527.2369), pt(400.25, 1500.75)}},
			{Op: ot.SegmentOpLineTo, Args: [3]ot.SegmentPoint{pt(127.76123, 1118)}},
		}}
	}

	want := glyphPolylines(outline(0), 10, 20, 0.0625)

	for _, dx := range []float32{1.5e-5, -1.5e-5, 6.1e-5, -6.1e-5} {
		if got := glyphPolylines(outline(dx), 10, 20, 0.0625); !reflect.DeepEqual(got, want) {
			t.Fatalf("an outline moved by %g came out different", dx)
		}
	}
}

// golang.org/x/image/vector sums areas with SSE on amd64 and with a Go loop
// elsewhere, in a different order: the packages that draw must not use it
// (internal/scan has the portable loop).
func TestNoVectorRasterizer(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}

		for _, imp := range f.Imports {
			if path, _ := strconv.Unquote(
				imp.Path.Value,
			); slices.Contains(
				[]string{"golang.org/x/image/vector"},
				path,
			) {
				t.Errorf(
					"%s imports %s: use internal/scan, which gives the same bytes on every architecture",
					file,
					path,
				)
			}
		}
	}
}
