package export

import (
	"context"
	"fmt"
	"image"
	"strings"
	"testing"

	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/plan"
)

const (
	gridCell = 12
	gridCols = 25
)

// gridDoc is a canvas of n small squares in a grid, each its own colour, over a background, so that a layer that
// was not drawn shows in the pixel of its centre. Every fifth square is half transparent, every third is rotated
// and a few use a blend mode, so the passes cross layers that read the picture below them.
func gridDoc(n int) *coreimage.Document {
	d := &coreimage.Document{
		SchemaVersion: coreimage.SchemaVersion, ID: "img_aaaaaaaaaa", Name: "grid",
		Width: gridCols * gridCell, Height: ((n + gridCols - 1) / gridCols) * gridCell, Background: "#101820",
	}

	blends := []string{"multiply", "screen", "overlay", "difference"}

	for i := range n {
		l := coreimage.Layer{
			ID: fmt.Sprintf("lyr_%010d", i), Kind: coreimage.KindShape, Shape: coreimage.ShapeRect, Name: "s",
			Visible: true, Opacity: 100, X: i % gridCols * gridCell, Y: i / gridCols * gridCell,
			W: 10, H: 10, Fill: gridColour(i),
		}

		if i%5 == 0 {
			l.Opacity = 50
		}

		if i%3 == 0 {
			l.Rotation = 15
		}

		if i%11 == 0 {
			l.Blend = blends[(i/11)%len(blends)]
		}

		d.Layers = append(d.Layers, l)
	}

	coreimage.ReorderAll(d)

	return d
}

func gridRGB(i int) [3]byte {
	return [3]byte{byte((i * 37) % 256), byte((i*91 + 40) % 256), byte((i*53 + 120) % 256)}
}

func gridColour(i int) string {
	c := gridRGB(i)

	return fmt.Sprintf("#%02X%02X%02X", c[0], c[1], c[2])
}

// pictureDiff counts the bytes in which two pictures differ, -1 when they are not of the same size.
func pictureDiff(a, b *image.RGBA) int {
	if a.Rect != b.Rect {
		return -1
	}

	return differingBytes(a.Pix, b.Pix)
}

// squareShows tells that the centre of layer i of a gridDoc has the colour the layer was given.
func squareShows(img *image.RGBA, i int) bool {
	o := img.PixOffset(i%gridCols*gridCell+5, i/gridCols*gridCell+5)

	return [3]byte{img.Pix[o], img.Pix[o+1], img.Pix[o+2]} == gridRGB(i)
}

// TestImagePassesAreTheOnePassPixels: a document drawn in two passes, or in many, is the same pixels as the one
// pass the module draws it in when it fits (SDD-021 section 6.1): the picture of the passes below is the bottom
// layer of the next, and the blend modes, opacities and rotations above it read the same pixels.
func TestImagePassesAreTheOnePassPixels(t *testing.T) {
	d := gridDoc(60)

	one, err := renderImagePasses(context.Background(), d, ImageConfig{}, 0, maxPlanLayers)
	if err != nil {
		t.Fatal(err)
	}

	for _, limit := range []int{2, 7, 31, 41} {
		got, err := renderImagePasses(context.Background(), d, ImageConfig{}, 0, limit)
		if err != nil {
			t.Fatalf("passes of %d layers: %v", limit, err)
		}

		if n := pictureDiff(one, got); n != 0 {
			t.Fatalf("passes of %d layers differ from one pass in %d bytes", limit, n)
		}
	}
}

// TestImagePassesKeepTheLooksAndTheMasks: the document of every look, mask and blend mode of the catalogue (a
// hidden layer that masks another included) is the same pixels in passes of a few layers as in one, with the
// cache and without it.
func TestImagePassesKeepTheLooksAndTheMasks(t *testing.T) {
	d := lookAll(t)
	src := sourcePNG(t)
	cfg := ImageConfig{AssetPath: func(string) (string, error) { return src, nil }}

	one, err := renderImagePasses(context.Background(), d, cfg, 0, maxPlanLayers)
	if err != nil {
		t.Fatal(err)
	}

	cachedCfg := cfg
	cachedCfg.Cache, cachedCfg.CacheScope = NewImageCache(0), "u"

	for name, c := range map[string]ImageConfig{"plain": cfg, "cached": cachedCfg} {
		for _, limit := range []int{6, 8, 10} {
			got, err := renderImagePasses(context.Background(), d, c, 0, limit)
			if err != nil {
				t.Fatalf("%s, passes of %d layers: %v", name, limit, err)
			}

			if n := pictureDiff(one, got); n != 0 {
				t.Fatalf("%s, passes of %d layers differ from one pass in %d bytes", name, limit, n)
			}
		}
	}
}

// VV-360: a document of more layers than the module draws in a plan exports and previews (SDD-021 accepts 500).
func TestImageMoreLayersThanOnePlanExportAndPreview(t *testing.T) {
	for _, n := range []int{maxPlanLayers + 1, 70, 126, 500} {
		t.Run(fmt.Sprintf("%d layers", n), func(t *testing.T) {
			d := gridDoc(n)

			exported, err := RenderImage(context.Background(), d, ImageConfig{})
			if err != nil {
				t.Fatalf("export: %v", err)
			}

			cache := NewImageCache(0)
			cfg := ImageConfig{Cache: cache, CacheScope: "u"}

			previewed, err := RenderImage(context.Background(), d, cfg)
			if err != nil {
				t.Fatalf("preview: %v", err)
			}

			if x := pictureDiff(exported, previewed); x != 0 {
				t.Fatalf("the preview differs from the export in %d bytes", x)
			}

			// every plain square, from the bottom pass to the top one, is on the canvas
			for i := range n {
				if i%5 != 0 && i%3 != 0 && i%11 != 0 && !squareShows(exported, i) {
					t.Fatalf("layer %d of %d is not drawn", i, n)
				}
			}

			// the edit of the top layer starts from the cache again
			edited := withOpacity(t, d, layerIDs(d)[n-1], 40)

			want, err := RenderImage(context.Background(), edited, ImageConfig{})
			if err != nil {
				t.Fatal(err)
			}

			got, err := RenderImage(context.Background(), edited, cfg)
			if err != nil {
				t.Fatal(err)
			}

			if x := pictureDiff(want, got); x != 0 {
				t.Fatalf("the edited preview differs from the export in %d bytes", x)
			}

			if cache.Stats().Hits == 0 {
				t.Fatalf("the edit never started from the cache: %+v", cache.Stats())
			}
		})
	}
}

// Whatever the passes are cut at, the pixels are the same: 70 layers in the passes the module takes and in
// passes of three layers.
func TestImageSeventyLayersAreTheSamePixelsInAnyPasses(t *testing.T) {
	d := gridDoc(70)

	a, err := RenderImage(context.Background(), d, ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}

	b, err := renderImagePasses(context.Background(), d, ImageConfig{}, 0, 3)
	if err != nil {
		t.Fatal(err)
	}

	if n := pictureDiff(a, b); n != 0 {
		t.Fatalf("differ in %d bytes", n)
	}
}

func TestSplitPassesCountsThePictureBelowAndAvoidsAClip(t *testing.T) {
	mk := func(n int) []plan.Layer {
		out := make([]plan.Layer, n)
		for i := range out {
			out[i] = plan.Layer{ItemID: fmt.Sprintf("l%d", i), Kind: plan.Bitmap, Opacity: 1}
		}

		return out
	}

	cases := []struct {
		name  string
		n     int
		start int
		base  bool
		stops []int
		want  string
	}{
		{"fits", 64, 0, false, []int{64}, "[64]"},
		{"one over", 65, 0, false, []int{65}, "[64 65]"},
		{"a pass over a picture draws one less", 200, 0, false, []int{200}, "[64 127 190 200]"},
		{"from a cached picture", 70, 5, true, []int{70}, "[68 70]"},
		{"the stops of the cache are kept", 70, 0, false, []int{69, 70}, "[64 69 70]"},
	}

	for _, c := range cases {
		got, err := splitPasses(mk(c.n), c.start, c.base, c.stops, maxPlanLayers)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: %v, %v; want %s", c.name, got, err, c.want)
		}
	}

	// a clip source and its layer stay in one pass: the cut falls below both
	layers := mk(70)
	layers[60].Clip = &plan.ClipSpec{Source: "l66"}

	got, err := splitPasses(layers, 0, false, []int{70}, maxPlanLayers)
	if err != nil || fmt.Sprint(got) != "[60 70]" {
		t.Errorf("a clip across the cut: %v, %v; want [60 70]", got, err)
	}

	// a clip that holds more layers than a pass draws has no cut
	layers = mk(130)
	layers[1].Clip = &plan.ClipSpec{Source: "l120"}

	_, err = splitPasses(layers, 0, false, []int{130}, maxPlanLayers)
	if err == nil || !strings.Contains(err.Error(), "render_layers") {
		t.Errorf("a clip across 120 layers: err = %v, want the limit of layers", err)
	}
}
