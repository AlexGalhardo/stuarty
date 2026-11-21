package export

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	coreimage "github.com/AlexGalhardo/stuarty/core/image"
)

const testHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func lyr(c byte) string { return "lyr_" + string(bytes.Repeat([]byte{c}, 10)) }

// sourcePNG writes a 64 by 48 picture with a horizontal ramp and a vertical one to a file and gives its path.
func sourcePNG(t *testing.T) string {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 64, 48))
	for y := range 48 {
		for x := range 64 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 5), B: 128, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	p := filepath.Join(t.TempDir(), "src.png")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	return p
}

func imageDoc(name string) *coreimage.Document {
	d := &coreimage.Document{
		SchemaVersion: coreimage.SchemaVersion, ID: "img_aaaaaaaaaa", Name: name, Width: 320, Height: 180,
		Background: "#101820",
		Assets:     map[string]coreimage.AssetRef{testHash: {Hash: testHash, Name: "src.png", Width: 64, Height: 48}},
	}

	switch name {
	case "shapes":
		d.Layers = []coreimage.Layer{
			{
				ID:      lyr('a'),
				Kind:    coreimage.KindFill,
				Name:    "Fill",
				Visible: true,
				Opacity: 100,
				W:       320,
				H:       180,
				Fill:    "#203040",
			},
			{
				ID: lyr(
					'b',
				),
				Kind:        coreimage.KindShape,
				Name:        "Pill",
				Visible:     true,
				Opacity:     100,
				X:           20,
				Y:           20,
				W:           160,
				H:           60,
				Shape:       coreimage.ShapeRoundedRect,
				Radius:      20,
				Fill:        "#E63946",
				StrokeColor: "#FFFFFF",
				StrokeWidth: 4,
			},
			{
				ID: lyr(
					'c',
				),
				Kind:     coreimage.KindShape,
				Name:     "Dot",
				Visible:  true,
				Opacity:  60,
				X:        200,
				Y:        60,
				W:        90,
				H:        90,
				Shape:    coreimage.ShapeEllipse,
				Fill:     "#FFE600",
				Rotation: 20,
			},
		}
	case "text":
		d.Layers = []coreimage.Layer{
			{
				ID: lyr(
					'a',
				),
				Kind:    coreimage.KindText,
				Name:    "Title",
				Visible: true,
				Opacity: 100,
				X:       20,
				Y:       20,
				W:       280,
				H:       100,
				Text:    "HOW IT WORKS",
			},
		}
	case "image_and_group":
		d.Layers = []coreimage.Layer{
			{
				ID: lyr('a'), Kind: coreimage.KindImage, Name: "Photo", Visible: true, Opacity: 100, W: 320, H: 180,
				Asset: testHash, Crop: &coreimage.Crop{X: 0, Y: 0, W: 64, H: 36},
			},
			{
				ID: lyr('b'), Kind: coreimage.KindGroup, Name: "Badge", Visible: true, Opacity: 50, X: 200, Y: 100,
				Children: []coreimage.Layer{{
					ID: lyr('c'), Kind: coreimage.KindShape, Name: "Box", Visible: true, Opacity: 100, W: 80, H: 50,
					Shape: coreimage.ShapeRect, Fill: "#FFFFFF",
				}},
			},
		}
	case "transparent":
		d.Background = coreimage.Transparent
		d.Layers = []coreimage.Layer{
			{
				ID: lyr(
					'a',
				),
				Kind:    coreimage.KindShape,
				Name:    "Dot",
				Visible: true,
				Opacity: 100,
				X:       100,
				Y:       40,
				W:       100,
				H:       100,
				Shape:   coreimage.ShapeEllipse,
				Fill:    "#00FF00",
			},
		}
	}

	coreimage.ReorderAll(d)

	return d
}

func pngBytes(t *testing.T, img *image.RGBA) []byte {
	t.Helper()

	var buf bytes.Buffer

	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func TestImageRender(t *testing.T) {
	src := sourcePNG(t)
	cfg := ImageConfig{AssetPath: func(string) (string, error) { return src, nil }}

	for _, name := range []string{"shapes", "text", "image_and_group", "transparent"} {
		t.Run(name, func(t *testing.T) {
			img, err := RenderImage(context.Background(), imageDoc(name), cfg)
			if err != nil {
				t.Fatal(err)
			}

			if img.Rect.Dx() != 320 || img.Rect.Dy() != 180 {
				t.Fatalf("the size is %v", img.Rect)
			}

			got := pngBytes(t, img)
			path := filepath.Join("testdata", "image", name+".png")

			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
					t.Fatal(err)
				}

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
				t.Fatalf("%s differs from its golden file (%d bytes against %d)", name, len(got), len(want))
			}
		})
	}
}

func TestImageRenderShowsWhatWasAsked(t *testing.T) {
	img, err := RenderImage(context.Background(), imageDoc("transparent"), ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}

	if a := img.RGBAAt(5, 5).A; a != 0 {
		t.Errorf("a transparent corner has alpha %d", a)
	}

	if c := img.RGBAAt(150, 90); c.G != 255 || c.A != 255 {
		t.Errorf("the dot is green and opaque, got %+v", c)
	}
}

func TestImageRenderMissingFile(t *testing.T) {
	_, err := RenderImage(context.Background(), imageDoc("image_and_group"), ImageConfig{})
	if err == nil {
		t.Fatal("an image layer with no file to read must fail")
	}
}

func TestRenderImageStopsWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := RenderImage(ctx, imageDoc("shapes"), ImageConfig{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled context must stop the render before it draws, got %v", err)
	}
}
