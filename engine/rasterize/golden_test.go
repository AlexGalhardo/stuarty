package rasterize

import (
	"bytes"
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

var update = flag.Bool("update", false, "rewrite testdata/golden from the current output")

// goldenCase is a reference text of the golden suite (P1-RND-003): its bitmap
// bytes must be equal on Linux, macOS and Windows (NFR-010).
type goldenCase struct {
	name string
	spec plan.BitmapSpec
}

var neverWords = []string{"Never", "gonna", "give", "you", "up"}

func goldenStyle(mod func(*plan.ResolvedStyle)) plan.ResolvedStyle {
	st := plainStyle()
	if mod != nil {
		mod(&st)
	}

	return st
}

func goldenCases() []goldenCase {
	box := C(20, 24, 32, 220)
	pink := C(255, 45, 110, 255)

	return []goldenCase{
		{"inter-plain", textSpec("Hello, World!", goldenStyle(nil))},
		{"inter-stroke-shadow-box", textSpec("Stroke, shadow and box", goldenStyle(func(s *plan.ResolvedStyle) {
			s.StrokeWidth, s.ShadowDX, s.ShadowDY, s.ShadowBlur = 6, 4, 6, 8
			s.BackgroundColor, s.BackgroundRadius = box, 14
		}))},
		{"montserrat-uppercase", textSpec("Montserrat 800 uppercase", goldenStyle(func(s *plan.ResolvedStyle) {
			s.FontFamily, s.FontWeight, s.Uppercase, s.Size = "Montserrat", 800, true, 56
		}))},
		{"anton-two-lines", textSpec("ANTON\nTWO LINES", goldenStyle(func(s *plan.ResolvedStyle) {
			s.FontFamily, s.FontWeight, s.LineHeight = "Anton", 400, 1.1
		}))},
		{
			"bebas-right-aligned",
			textSpec("Bebas Neue\nright aligned\nthree lines", goldenStyle(func(s *plan.ResolvedStyle) {
				s.FontFamily, s.FontWeight, s.Align, s.Size = "Bebas Neue", 400, AlignRight, 64
			})),
		},
		{"playfair-accents", textSpec("Café ÅÖ ñ ß — “quotes”", goldenStyle(func(s *plan.ResolvedStyle) {
			s.FontFamily, s.FontWeight, s.Size = "Playfair Display", 700, 60
		}))},
		{
			"jetbrains-mono-code",
			textSpec("func main() {\n    fmt.Println(\"hi\")\n}", goldenStyle(func(s *plan.ResolvedStyle) {
				s.FontFamily, s.FontWeight, s.Size, s.Align = "JetBrains Mono", 500, 40, AlignLeft
			})),
		},
		{"inter-cyrillic-greek", textSpec("Привет, мир — Γειά σου Κόσμε", goldenStyle(func(s *plan.ResolvedStyle) {
			s.FontWeight, s.Size = 600, 48
		}))},
		{"inter-wrapped-paragraph", textSpec(
			"Captions are burned into the picture by the same code that draws the preview, so the two always agree.",
			goldenStyle(func(s *plan.ResolvedStyle) {
				s.FontWeight, s.Size, s.MaxWidth, s.LineHeight, s.Align = 400, 36, 420, 1.5, AlignLeft
			}))},
		{
			"caption-plain",
			captionSpec([]string{"Never", "gonna", "give", "you", "up"}, -1, goldenStyle(func(s *plan.ResolvedStyle) {
				s.Size, s.StrokeWidth = 64, 5
			})),
		},
		{"caption-active-word", func() plan.BitmapSpec {
			sp := captionSpec(
				[]string{"Never", "gonna", "give", "you", "up"},
				2,
				goldenStyle(func(s *plan.ResolvedStyle) {
					s.Size, s.StrokeWidth, s.HighlightBackground = 64, 5, pink
				}),
			)
			sp.PixelScale = 2

			return sp
		}()},
		{
			"emoji-sequence-and-notdef",
			textSpec("Emoji 😀 and 👨‍👩‍👧 draw .notdef", goldenStyle(func(s *plan.ResolvedStyle) {
				s.Size, s.StrokeWidth, s.StrokeColor = 44, 3, C(0, 0, 0, 255)
			})),
		},
		{"right-to-left-mixed", textSpec("abc שלום مرحبا def 123", goldenStyle(func(s *plan.ResolvedStyle) {
			s.Size, s.StrokeWidth, s.StrokeColor = 44, 3, C(0, 0, 0, 255)
		}))},
		{"missing-glyph-no-fallback", textSpec("Latin 你好 สวัสดี text", goldenStyle(func(s *plan.ResolvedStyle) {
			s.Size, s.StrokeWidth, s.StrokeColor = 44, 3, C(0, 0, 0, 255)
		}))},
		{"caption-active-first", captionSpec(neverWords, 0, goldenStyle(func(s *plan.ResolvedStyle) {
			s.Size, s.StrokeWidth = 64, 5
		}))},
		{"caption-active-last-with-box", captionSpec(neverWords, 4, goldenStyle(func(s *plan.ResolvedStyle) {
			s.Size, s.StrokeWidth, s.HighlightBackground = 64, 5, pink
		}))},
		{"shape-rect", plan.BitmapSpec{Kind: KindShape, PixelScale: 1, Shape: &plan.ShapeSpec{
			Shape: ShapeRect, Width: 240, Height: 90, Fill: C(255, 120, 0, 255),
			StrokeColor: C(0, 0, 0, 255), StrokeWidth: 6,
		}}},
		{"shape-ellipse", plan.BitmapSpec{Kind: KindShape, PixelScale: 1, Shape: &plan.ShapeSpec{
			Shape: ShapeEllipse, Width: 240, Height: 120, Fill: C(60, 200, 120, 200),
			StrokeColor: C(255, 255, 255, 255), StrokeWidth: 10,
		}}},
		{"shape-rounded-rect", plan.BitmapSpec{Kind: KindShape, PixelScale: 1, Shape: &plan.ShapeSpec{
			Shape: ShapeRoundedRect, Width: 300, Height: 120, Radius: 30,
			Fill: C(30, 144, 255, 255), StrokeColor: C(255, 255, 255, 255), StrokeWidth: 8,
		}}},
	}
}

const goldenDir = "testdata/golden"

func TestGolden(t *testing.T) {
	t.Parallel()

	cases := goldenCases()
	if len(cases) < 12 {
		t.Fatalf("%d golden cases, want at least 12", len(cases))
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := render(t, c.spec)
			file := filepath.Join(goldenDir, c.name+".png")

			if *update {
				data, err := got.PNG()
				if err != nil {
					t.Fatal(err)
				}

				if err := os.MkdirAll(goldenDir, 0o755); err != nil {
					t.Fatal(err)
				}

				if err := os.WriteFile(file, data, 0o644); err != nil {
					t.Fatal(err)
				}

				return
			}

			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("%v (run: go test ./engine/rasterize -run Golden -update)", err)
			}

			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}

			want, ok := img.(*image.NRGBA)
			if !ok {
				t.Fatalf("%s is a %T, want *image.NRGBA", file, img)
			}

			if want.Bounds().Dx() != got.Width || want.Bounds().Dy() != got.Height {
				t.Fatalf("size %dx%d, golden %dx%d", got.Width, got.Height, want.Bounds().Dx(), want.Bounds().Dy())
			}

			// The pixel bytes are compared, not the PNG bytes: the compressor
			// may change between Go versions, the pixels must not.
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Errorf("%s: pixels differ from %s (sha256 %s)", c.name, file, hashOf(got))
			}
		})
	}
}

// TestDumpGolden writes the golden bitmaps as PNG files to the folder named by
// RASTER_DUMP, for looking at them. It does nothing otherwise.
func TestDumpGolden(t *testing.T) {
	dir := os.Getenv("RASTER_DUMP")
	if dir == "" {
		t.Skip("RASTER_DUMP is not set")
	}

	for _, c := range goldenCases() {
		data, err := render(t, c.spec).PNG()
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, c.name+".png"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
