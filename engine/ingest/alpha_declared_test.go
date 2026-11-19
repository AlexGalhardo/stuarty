package ingest

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ingest/mediagen"
)

// gifFile writes a GIF of frames frames of 16x12 pixels, every pixel red. With transparentIndex the palette has a
// transparent colour first, so the Go encoder writes a graphic control extension with the transparent flag, and no
// pixel uses it.
func gifFile(t *testing.T, frames int, transparentIndex bool) string {
	t.Helper()

	palette := color.Palette{color.RGBA{0, 0, 0, 255}, color.RGBA{255, 0, 0, 255}}
	if transparentIndex {
		palette[0] = color.RGBA{}
	}

	a := &gif.GIF{}

	for range frames {
		img := image.NewPaletted(image.Rect(0, 0, 16, 12), palette)
		for i := range img.Pix {
			img.Pix[i] = 1
		}

		a.Image = append(a.Image, img)
		a.Delay = append(a.Delay, 10)
	}

	var b bytes.Buffer
	if err := gif.EncodeAll(&b, a); err != nil {
		t.Fatal(err)
	}

	p := filepath.Join(t.TempDir(), "red.gif")
	if err := os.WriteFile(p, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	return p
}

// webpFile writes the RIFF header and the first chunk of a WebP: only what declaresAlpha reads.
func webpFile(t *testing.T, fourcc string, payload []byte) string {
	t.Helper()

	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(4+8+len(payload)))
	b.WriteString("WEBP" + fourcc)
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(payload)))
	b.Write(payload)

	p := filepath.Join(t.TempDir(), "x.webp")
	if err := os.WriteFile(p, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	return p
}

// The two formats whose decoder reports alpha for every file declare it in the file (SDD-011 section 4.3.6).
func TestAlphaDeclaredByTheFile(t *testing.T) {
	t.Parallel()

	vp8l := func(alpha bool) []byte {
		bits := uint32(15) | uint32(11)<<14 // a 16x12 picture
		if alpha {
			bits |= 1 << 28
		}

		p := []byte{0x2F, 0, 0, 0, 0}
		binary.LittleEndian.PutUint32(p[1:], bits)

		return p
	}

	other := filepath.Join(t.TempDir(), "x.mov")
	if err := os.WriteFile(other, []byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00qt  "), 0o600); err != nil {
		t.Fatal(err)
	}

	for name, c := range map[string]struct {
		path string
		want bool
	}{
		"a GIF with a transparent index":  {gifFile(t, 2, true), true},
		"a GIF without one":               {gifFile(t, 2, false), false},
		"a still GIF without one":         {gifFile(t, 1, false), false},
		"a VP8X WebP with the alpha flag": {webpFile(t, "VP8X", []byte{0x12, 0, 0, 0}), true},
		"a VP8X WebP without it":          {webpFile(t, "VP8X", []byte{0x02, 0, 0, 0}), false},
		"a lossless WebP with alpha":      {webpFile(t, "VP8L", vp8l(true)), true},
		"a lossless WebP without alpha":   {webpFile(t, "VP8L", vp8l(false)), false},
		"a lossy WebP":                    {webpFile(t, "VP8 ", []byte{0, 0, 0, 0}), false},
		"a truncated WebP":                {webpFile(t, "VP8L", nil), false},
		"any other file (pixel format)":   {other, true},
	} {
		if got := declaresAlpha(c.path); got != c.want {
			t.Errorf("%s: declaresAlpha = %v, want %v", name, got, c.want)
		}
	}
}

// An opaque GIF does not declare alpha, although FFmpeg decodes it to bgra: it is not reported as a file that claims
// transparency and has none. One whose palette has a transparent colour that no pixel uses is.
func TestAlphaGIFIsReportedOnlyWhenItDeclaresTransparency(t *testing.T) {
	e := newEnv(t)

	for name, c := range map[string]struct {
		frames      int
		transparent bool
		want        bool
	}{
		"a still opaque GIF":                         {1, false, false},
		"an animated opaque GIF":                     {3, false, false},
		"an animated GIF with an unused transparent": {3, true, true},
	} {
		_, res := e.mustIngest(gifFile(t, c.frames, c.transparent))

		if !res.Facts.HasAlpha {
			t.Fatalf("%s: a GIF is decoded with alpha: %+v", name, res.Facts)
		}

		if res.Facts.AlphaOpaque != c.want || slices.Contains(res.Warnings, WarningAlphaOpaque) != c.want {
			t.Errorf("%s: alpha_opaque %v, warnings %v; want %v", name, res.Facts.AlphaOpaque, res.Warnings, c.want)
		}
	}
}

// An animated image is sampled on every frame: one whose first frame is opaque and whose later frames are
// transparent on the left is not reported.
func TestAlphaAnimatedImageIsSampledOnEveryFrame(t *testing.T) {
	e := newEnv(t)

	for _, in := range []mediagen.AlphaInput{mediagen.AlphaAPNG, mediagen.AlphaWebP} {
		p, err := e.gen.AlphaLater(context.Background(), in, "later-"+string(in)+"."+in.Ext(), alphaW, alphaH, 30, 1, 4)
		_, res := e.mustIngest(mustGen(t, p, err))

		if !res.Facts.HasAlpha || res.Facts.AlphaOpaque || slices.Contains(res.Warnings, WarningAlphaOpaque) {
			t.Errorf("%s: facts %+v, warnings %v", in, res.Facts, res.Warnings)
		}
	}
}
