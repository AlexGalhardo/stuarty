package rasterize

import (
	"bytes"
	"errors"
	"image/png"
	"math"
	"sync"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

func styledForAllEffects() plan.ResolvedStyle {
	st := plainStyle()
	bg := C(0, 0, 0, 160)
	st.BackgroundColor = bg
	st.BackgroundRadius = 12
	st.StrokeWidth = 6
	st.ShadowDX, st.ShadowDY, st.ShadowBlur = 5, 5, 7

	return st
}

func TestRenderIsDeterministic(t *testing.T) {
	t.Parallel()

	spec := textSpec("Same spec, same bytes", styledForAllEffects())
	first := render(t, spec)

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			if got := render(t, spec); !bytes.Equal(got.Pix, first.Pix) {
				t.Error("two renders of the same spec differ")
			}
		})
	}

	wg.Wait()
}

func TestFillOnly(t *testing.T) {
	t.Parallel()

	b := render(t, textSpec("I", plainStyle()))
	if b.Width == 0 || b.Height == 0 {
		t.Fatal("empty bitmap")
	}

	// A white glyph on transparent: every inked pixel is white, with partial
	// alpha at the edges, and the colour is straight (not premultiplied).
	full := 0

	for i := 0; i < len(b.Pix); i += 4 {
		if b.Pix[i+3] == 0 {
			continue
		}

		if b.Pix[i] != 255 || b.Pix[i+1] != 255 || b.Pix[i+2] != 255 {
			t.Fatalf("pixel %d is %v: a partly covered white pixel must stay white (straight alpha)", i/4, b.Pix[i:i+4])
		}

		if b.Pix[i+3] == 255 {
			full++
		}
	}

	if full == 0 {
		t.Error("the glyph has no fully covered pixel")
	}
}

func TestNothingIsClipped(t *testing.T) {
	t.Parallel()

	// Stroke, shadow offset and blur and a tight line height: the outermost
	// row and column of the bitmap stay clear. The tail of the blur may leave
	// an alpha of a few units there, because each pass rounds up; a clipped
	// edge would show an alpha of dozens.
	const tail = 8

	st := plainStyle()
	st.StrokeWidth = 12
	st.ShadowDX, st.ShadowDY, st.ShadowBlur = 20, -14, 9
	st.LineHeight = 0.8

	b := render(t, textSpec("Jgy|Å", st))

	for x := range b.Width {
		if alphaAt(b, x, 0) > tail || alphaAt(b, x, b.Height-1) > tail {
			t.Fatalf("ink on the top or bottom row at x=%d: the bitmap clips", x)
		}
	}

	for y := range b.Height {
		if alphaAt(b, 0, y) > tail || alphaAt(b, b.Width-1, y) > tail {
			t.Fatalf("ink on the left or right column at y=%d: the bitmap clips", y)
		}
	}
}

func TestStrokeGrowsTheInk(t *testing.T) {
	t.Parallel()

	plain := render(t, textSpec("O", plainStyle()))

	st := plainStyle()
	st.StrokeWidth = 10
	stroked := render(t, textSpec("O", st))

	if stroked.Width <= plain.Width+8 || stroked.Height <= plain.Height+8 {
		t.Errorf(
			"stroked %dx%d is not larger than plain %dx%d by the stroke",
			stroked.Width,
			stroked.Height,
			plain.Width,
			plain.Height,
		)
	}

	// The stroke is black, drawn under the white fill: some pixel is black.
	black := false

	for i := 0; i < len(stroked.Pix); i += 4 {
		if stroked.Pix[i+3] == 255 && stroked.Pix[i] == 0 {
			black = true
		}
	}

	if !black {
		t.Error("no opaque black pixel: the stroke is missing")
	}
}

func TestBackgroundBoxCoversTheBlockWithPadding(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	bg := C(10, 20, 30, 255)
	st.BackgroundColor = bg
	st.BackgroundPadding = 20

	b := render(t, textSpec("Box", st))

	// Away from the glyphs, near a corner inside the padding, the box shows.
	o := (23*b.Width + 23) * 4
	if got := b.Pix[o : o+4]; got[0] != 10 || got[1] != 20 || got[2] != 30 || got[3] != 255 {
		t.Errorf("pixel inside the padding = %v, want the box colour", got)
	}

	// With radius 0 the corner of the box is covered. The bitmap has one clear
	// pixel all around, so the box starts at (1, 1).
	if alphaAt(b, 1, 1) != 255 {
		t.Errorf("corner alpha = %d with radius 0, want 255", alphaAt(b, 1, 1))
	}

	st.BackgroundRadius = 20
	round := render(t, textSpec("Box", st))

	if alphaAt(round, 1, 1) > 20 {
		t.Errorf("corner alpha = %d with a radius, want almost 0", alphaAt(round, 1, 1))
	}
}

func TestShadowIsOffsetAndOnlyWhenAsked(t *testing.T) {
	t.Parallel()

	none := render(t, textSpec("S", plainStyle()))

	st := plainStyle()
	st.ShadowDX, st.ShadowDY = 12, 12
	st.ShadowColor = C(255, 0, 0, 255)
	shadowed := render(t, textSpec("S", st))

	if inked(shadowed) <= inked(none) {
		t.Error("a shadow added no pixels")
	}

	// A shadow colour with zero offset and blur is not drawn: it would only
	// darken the antialiased edges.
	st.ShadowDX, st.ShadowDY = 0, 0
	if got := render(t, textSpec("S", st)); !bytes.Equal(got.Pix, none.Pix) {
		t.Error("a shadow with no offset and no blur changed the bitmap")
	}

	// A shadow that reaches further on one side does not move the bitmap's centre off the block's: the
	// browser draws a bitmap by its centre and the plan places the block (SDD-004 section 5.1 item 6).
	st.ShadowDX, st.ShadowDY = 40, 0
	moved := render(t, textSpec("S", st))

	if math.Abs(moved.CenterDX) > 0.5 || math.Abs(moved.CenterDY) > 0.5 {
		t.Errorf(
			"centre offset (%.2f, %.2f), want within half a pixel of the block centre",
			moved.CenterDX,
			moved.CenterDY,
		)
	}

	if moved.Width <= none.Width+40 {
		t.Errorf("the bitmap is %d wide, the shadow reaches 40 pixels beyond %d", moved.Width, none.Width)
	}
}

func TestPixelScaleScalesTheBitmap(t *testing.T) {
	t.Parallel()

	st := styledForAllEffects()
	one := render(t, textSpec("Scale", st))

	spec := textSpec("Scale", st)
	spec.PixelScale = 2
	two := render(t, spec)

	if r := float64(two.Width) / float64(one.Width); math.Abs(r-2) > 0.05 {
		t.Errorf("width ratio %.3f, want about 2", r)
	}

	if math.Abs(two.NaturalW-one.NaturalW) > 2.5 || math.Abs(two.NaturalH-one.NaturalH) > 2.5 {
		t.Errorf(
			"natural size %.1fx%.1f at scale 2, %.1fx%.1f at scale 1: want equal",
			two.NaturalW,
			two.NaturalH,
			one.NaturalW,
			one.NaturalH,
		)
	}
}

func TestCaptionStates(t *testing.T) {
	t.Parallel()

	words := []string{"Never", "gonna", "give"}
	st := plainStyle()
	st.HighlightColor = C(255, 0, 0, 255)
	hb := C(0, 0, 255, 255)
	st.HighlightBackground = hb

	none := render(t, captionSpec(words, -1, st))
	second := render(t, captionSpec(words, 1, st))
	third := render(t, captionSpec(words, 2, st))

	if bytes.Equal(none.Pix, second.Pix) || bytes.Equal(second.Pix, third.Pix) {
		t.Fatal("the states of the caption are not distinct bitmaps")
	}

	if len(second.WordBoxes) != 3 {
		t.Fatalf("word boxes = %d, want 3", len(second.WordBoxes))
	}

	// Word boxes are left to right and do not overlap.
	for i := 1; i < 3; i++ {
		prev, cur := second.WordBoxes[i-1], second.WordBoxes[i]
		if cur.X < prev.X+prev.W-0.01 {
			t.Errorf("word %d box %+v overlaps word %d box %+v", i, cur, i-1, prev)
		}
	}

	// The active word has red ink inside its box and nothing red elsewhere.
	box := second.WordBoxes[1]
	redIn, redOut := 0, 0

	for y := range second.Height {
		for x := range second.Width {
			o := (y*second.Width + x) * 4
			if second.Pix[o] != 255 || second.Pix[o+1] != 0 || second.Pix[o+2] != 0 || second.Pix[o+3] != 255 {
				continue
			}

			if float64(x) >= box.X && float64(x) <= box.X+box.W {
				redIn++
			} else {
				redOut++
			}
		}
	}

	if redIn == 0 || redOut != 0 {
		t.Errorf("highlight colour pixels: %d inside the word box, %d outside", redIn, redOut)
	}
}

func TestShapes(t *testing.T) {
	t.Parallel()

	spec := plan.BitmapSpec{
		Kind: KindShape, PixelScale: 1,
		Shape: &plan.ShapeSpec{Shape: ShapeRect, Width: 100, Height: 50, Fill: C(255, 0, 0, 255)},
	}
	rect := render(t, spec)

	if rect.Width != 100 || rect.Height != 50 {
		t.Fatalf("rect bitmap %dx%d, want 100x50", rect.Width, rect.Height)
	}

	if inked(rect) != 100*50 {
		t.Errorf("rect pixels = %d, want %d", inked(rect), 100*50)
	}

	spec.Shape.Shape = ShapeEllipse
	ell := render(t, spec)

	if got, want := float64(inked(ell)), math.Pi*50*25; math.Abs(got-want)/want > 0.06 {
		t.Errorf("ellipse area %v, want about %v", got, want)
	}

	spec.Shape.Shape = ShapeRoundedRect
	spec.Shape.Radius = 25
	spec.Shape.StrokeWidth = 10
	spec.Shape.StrokeColor = C(0, 0, 255, 255)
	stroked := render(t, spec)

	if stroked.Width != 110 || stroked.Height != 60 {
		t.Errorf("stroked bitmap %dx%d, want 110x60 (the stroke is centred on the edge)", stroked.Width, stroked.Height)
	}

	if alphaAt(stroked, 0, 0) != 0 {
		t.Error("the rounded corner of the stroke is not transparent")
	}
}

func TestRenderRejectsBadSpecs(t *testing.T) {
	t.Parallel()

	bad := map[string]plan.BitmapSpec{
		"no scale":   {Kind: KindText, Text: "x", Style: plainStyle()},
		"huge scale": {Kind: KindText, Text: "x", Style: plainStyle(), PixelScale: 100},
		"no kind":    {PixelScale: 1},
		"no shape":   {Kind: KindShape, PixelScale: 1},
		"no words":   {Kind: KindCaption, Style: plainStyle(), PixelScale: 1},
		"bad active": captionSpec([]string{"a"}, 3, plainStyle()),
		"zero size": textSpec(
			"x",
			plan.ResolvedStyle{FontFamily: "Inter", FontWeight: 400, LineHeight: 1, MaxWidth: 10},
		),
		"zero width": {Kind: KindShape, Shape: &plan.ShapeSpec{Shape: ShapeRect, Height: 5}, PixelScale: 1},
		"bad shape":  {Kind: KindShape, Shape: &plan.ShapeSpec{Shape: "star", Width: 5, Height: 5}, PixelScale: 1},
		"huge bitmap": {
			Kind:       KindShape,
			Shape:      &plan.ShapeSpec{Shape: ShapeRect, Width: 1e5, Height: 1e5},
			PixelScale: 1,
		},
	}

	for name, spec := range bad {
		if _, err := Render(spec, Bundled()); !errors.Is(err, ErrInvalidSpec) {
			t.Errorf("%s: error = %v, want ErrInvalidSpec", name, err)
		}
	}
}

func TestRenderUnknownFont(t *testing.T) {
	t.Parallel()

	st := plainStyle()
	st.FontFamily = "Papyrus"

	_, err := Render(textSpec("x", st), Bundled())

	var fe *FontError
	if !errors.As(err, &fe) || fe.Code != CodeFontUnknown {
		t.Errorf("error = %v, want E_FONT_UNKNOWN", err)
	}
}

func TestKey(t *testing.T) {
	t.Parallel()

	spec := textSpec("Key", plainStyle())
	key := spec.Key()

	if len(key) != 64 || key != spec.Key() {
		t.Fatalf("key %q is not a stable SHA-256 hex", key)
	}

	size := spec
	size.Style.Size++

	scale := spec
	scale.PixelScale = 2

	text := spec
	text.Text = "Key."

	bg := spec
	bg.Style.BackgroundColor = C(1, 2, 3, 4)

	for name, s := range map[string]plan.BitmapSpec{"size": size, "scale": scale, "text": text, "background": bg} {
		if s.Key() == key {
			t.Errorf("changing the %s did not change the key", name)
		}
	}
}

func TestPNGRoundTrip(t *testing.T) {
	t.Parallel()

	b := render(t, textSpec("PNG", styledForAllEffects()))

	data, err := b.PNG()
	if err != nil {
		t.Fatal(err)
	}

	again, _ := b.PNG()
	if !bytes.Equal(data, again) {
		t.Error("the PNG encoding is not deterministic")
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	if img.Bounds().Dx() != b.Width || img.Bounds().Dy() != b.Height {
		t.Errorf("decoded %v, want %dx%d", img.Bounds(), b.Width, b.Height)
	}
}
