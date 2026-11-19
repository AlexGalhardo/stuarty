package rasterize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// C is a colour in the form a plan style holds it.
func C(r, g, b, a uint8) string { return fmt.Sprintf("#%02X%02X%02X%02X", r, g, b, a) }

var (
	white = C(255, 255, 255, 255)
	black = C(0, 0, 0, 255)
)

// plainStyle is a style with no stroke, no shadow and no box: only the fill
// shows, which keeps the assertions simple.
func plainStyle() plan.ResolvedStyle {
	return plan.ResolvedStyle{
		FontFamily: "Inter", FontWeight: 700, Size: 72, Color: white,
		StrokeColor: black, ShadowColor: C(0, 0, 0, 128),
		BackgroundPadding: 16, Align: AlignCenter, LineHeight: 1.2, MaxWidth: 900,
		HighlightColor: C(255, 230, 0, 255),
	}
}

func textSpec(text string, st plan.ResolvedStyle) plan.BitmapSpec {
	return plan.BitmapSpec{Kind: KindText, Text: text, Style: st, PixelScale: 1, ActiveWord: -1}
}

func captionSpec(words []string, active int, st plan.ResolvedStyle) plan.BitmapSpec {
	return plan.BitmapSpec{Kind: KindCaption, Words: words, ActiveWord: active, Style: st, PixelScale: 1}
}

func render(t *testing.T, spec plan.BitmapSpec) Bitmap {
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
func alphaAt(b Bitmap, x, y int) uint8 { return b.Pix[(y*b.Width+x)*4+3] }

// inked counts the pixels with any alpha.
func inked(b Bitmap) int {
	n := 0

	for i := 3; i < len(b.Pix); i += 4 {
		if b.Pix[i] > 0 {
			n++
		}
	}

	return n
}
