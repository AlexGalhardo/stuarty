package raster

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

var (
	white = Colour{255, 255, 255, 255}
	black = Colour{0, 0, 0, 255}
)

// plainStyle is a style with no stroke, no shadow and no box: only the fill
// shows, which keeps the assertions simple.
func plainStyle() ResolvedStyle {
	return ResolvedStyle{
		FontFamily: "Inter", FontWeight: 700, Size: 72, Color: white,
		StrokeColor: black, ShadowColor: Colour{0, 0, 0, 128},
		BackgroundPadding: 16, Align: AlignCenter, LineHeight: 1.2, MaxWidth: 900,
		HighlightColor: Colour{255, 230, 0, 255},
	}
}

func textSpec(text string, st ResolvedStyle) BitmapSpec {
	return BitmapSpec{Kind: KindText, Text: text, Style: st, PixelScale: 1, ActiveWord: -1}
}

func captionSpec(words []string, active int, st ResolvedStyle) BitmapSpec {
	return BitmapSpec{Kind: KindCaption, Words: words, ActiveWord: active, Style: st, PixelScale: 1}
}

func render(t *testing.T, spec BitmapSpec) Bitmap {
	t.Helper()

	b, err := Render(spec, Bundled())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	return b
}

func hashOf(b Bitmap) string {
	h := sha256.Sum256(b.Pix)

	return hex.EncodeToString(h[:])
}

// alphaAt returns the alpha of a pixel.
func (b Bitmap) alphaAt(x, y int) uint8 { return b.Pix[(y*b.Width+x)*4+3] }

// inked counts the pixels with any alpha.
func (b Bitmap) inked() int {
	n := 0

	for i := 3; i < len(b.Pix); i += 4 {
		if b.Pix[i] > 0 {
			n++
		}
	}

	return n
}
