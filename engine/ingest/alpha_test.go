package ingest

import (
	"context"
	"image"
	"image/png"
	"os"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ingest/mediagen"
)

const (
	alphaW, alphaH = 160, 90
	alphaName      = "alpha-"
)

func alphaFixture(t *testing.T, e *env, in mediagen.AlphaInput, opaque bool) string {
	t.Helper()

	name := alphaName + string(in) + "." + in.Ext()
	if opaque {
		name = alphaName + "opaque-" + name[len(alphaName):]
	}

	gen := e.gen.AlphaEdge
	if opaque {
		gen = e.gen.AlphaOpaque
	}

	p, err := gen(context.Background(), in, name, alphaW, alphaH, 30, 1, 4)

	return mustGen(t, p, err)
}

func readPNG(t *testing.T, path string) image.Image {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}

	return img
}

// straight returns the colour and the alpha of the pixel of img at the fraction of its width and height, as 8-bit
// straight (not premultiplied) values.
func straight(img image.Image, fx, fy float64) (r, g, b, a int) {
	bounds := img.Bounds()
	x := bounds.Min.X + int(fx*float64(bounds.Dx()))
	y := bounds.Min.Y + int(fy*float64(bounds.Dy()))

	c := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	c.Set(0, 0, img.At(x, y))

	p := c.Pix

	return int(p[0]), int(p[1]), int(p[2]), int(p[3])
}

// pictureOf is the PNG that shows an asset with its alpha: the normalized picture of an image, and for a video the
// poster cut from its alpha proxy.
func pictureOf(t *testing.T, e *env, hash string, res *Result, video bool) image.Image {
	t.Helper()

	if !video {
		return readPNG(t, e.publishedPath(hash, res.find("normalized", "")))
	}

	return readPNG(t, e.publishedPath(hash, res.find("thumbnails", "w320")))
}

// TestAlphaInputsKeepAStraightSoftEdge is SDD-011 section 4.3.6: each accepted input is ingested as an asset with
// alpha, and the soft edge of its fixture (transparent on the left, rising to opaque at the half, red everywhere)
// comes out straight: half transparent pixels keep the full red, so that a decoder that premultiplied would fail.
func TestAlphaInputsKeepAStraightSoftEdge(t *testing.T) {
	for _, in := range mediagen.AlphaInputs() {
		t.Run(string(in), func(t *testing.T) {
			e := newEnv(t)
			hash, res := e.mustIngest(alphaFixture(t, e, in, false))

			if !res.Facts.HasAlpha || res.Facts.AlphaOpaque {
				t.Fatalf("facts %+v: an input with a soft edge has alpha and is not opaque", res.Facts)
			}

			if slices.Contains(res.Warnings, WarningAlphaOpaque) {
				t.Errorf("warnings %v", res.Warnings)
			}

			img := pictureOf(t, e, hash, res, in.Video())

			// A lossy encoder (VP9 at 4:2:0) moves the edge by a few levels; the straight/premultiplied
			// difference at the middle of the ramp is about 128 levels of red.
			tol := 24
			if in == mediagen.AlphaVP9 {
				tol = 40
			}

			_, _, _, left := straight(img, 0.12, 0.5)
			r, g, b, mid := straight(img, 0.375, 0.5)
			_, _, _, right := straight(img, 0.88, 0.5)

			if left > tol || right < 255-tol || mid < 128-tol || mid > 128+tol {
				t.Errorf("alpha left %d, middle %d, right %d", left, mid, right)
			}

			if r < 255-tol || g > tol || b > tol {
				t.Errorf("the half transparent pixel is (%d,%d,%d): a straight alpha keeps the red whole", r, g, b)
			}
		})
	}
}

// The poster of an image with alpha keeps the alpha (VV-242): the preview draws a still from its poster, and a poster
// without alpha shows the transparent area as a black box over the layers below.
func TestAlphaImagePostersKeepTheAlpha(t *testing.T) {
	for _, in := range mediagen.AlphaInputs() {
		if in.Video() {
			continue
		}

		t.Run(string(in), func(t *testing.T) {
			e := newEnv(t)
			hash, res := e.mustIngest(alphaFixture(t, e, in, false))

			for _, variant := range []string{"w160", "w320", "w640"} {
				art := res.find("thumbnails", variant)
				if art.Ext != "png" {
					t.Fatalf("poster %s is a .%s file, want .png", variant, art.Ext)
				}

				img := readPNG(t, e.publishedPath(hash, art))
				_, _, _, left := straight(img, 0.12, 0.5)
				r, _, _, right := straight(img, 0.88, 0.5)

				if left > 24 || right < 255-24 || r < 255-24 {
					t.Errorf("poster %s: alpha left %d (want transparent), right %d (want opaque), red %d", variant, left, right, r)
				}
			}
		})
	}
}

// The animated GIF has the alpha of a transparent index: on or off, no soft edge (SDD-011 section 4.3.6).
func TestAlphaAnimatedGIFKeepsItsTransparentIndex(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.AnimatedGIF("alpha.gif", 16, 12, []int{7, 3, 10})
	hash, res := e.mustIngest(mustGen(t, p, err))

	if !res.Facts.HasAlpha || res.Facts.AlphaOpaque {
		t.Fatalf("facts %+v", res.Facts)
	}

	img := readPNG(t, e.publishedPath(hash, res.find("normalized", "")))

	if _, _, _, a := straight(img, 0.02, 0.02); a != 0 {
		t.Errorf("the transparent corner has alpha %d", a)
	}

	if _, _, _, a := straight(img, 0.6, 0.6); a != 255 {
		t.Errorf("the body has alpha %d", a)
	}
}

// A file that claims alpha and has none is reported, not turned into a silent opaque clip: the asset keeps
// has_alpha, the normalized artifact records alpha_opaque and the job warns (SDD-011 section 4.3.6). libwebp writes
// no alpha flag for an opaque picture, so the opaque WebP declares none, although FFmpeg decodes a lossless WebP to
// argb: it is not reported (TestAlphaDeclaredByTheFile covers a WebP that declares alpha).
func TestAlphaClaimedButOpaqueIsReported(t *testing.T) {
	for _, in := range mediagen.AlphaInputs() {
		t.Run(string(in), func(t *testing.T) {
			e := newEnv(t)
			_, res := e.mustIngest(alphaFixture(t, e, in, true))

			if in == mediagen.AlphaWebP {
				if !res.Facts.HasAlpha || res.Facts.AlphaOpaque || slices.Contains(res.Warnings, WarningAlphaOpaque) {
					t.Errorf("facts %+v, warnings %v: an opaque WebP declares no alpha", res.Facts, res.Warnings)
				}

				return
			}

			if !res.Facts.HasAlpha || !res.Facts.AlphaOpaque {
				t.Fatalf("facts %+v: the file declares alpha and every picture is opaque", res.Facts)
			}

			if !slices.Contains(res.Warnings, WarningAlphaOpaque) {
				t.Errorf("warnings %v", res.Warnings)
			}

			n := res.find("normalized", "")
			if f, ok := n.Meta.(Facts); !ok || !f.AlphaOpaque {
				t.Errorf("the normalized artifact does not record it: %+v", n.Meta)
			}
		})
	}
}

// An opaque file that does not claim alpha is not reported.
func TestAlphaOpaqueFileWithoutAlphaIsNotReported(t *testing.T) {
	e := newEnv(t)
	p, err := e.gen.Image(context.Background())
	_, res := e.mustIngest(mustGen(t, p, err))

	if res.Facts.HasAlpha || res.Facts.AlphaOpaque || slices.Contains(res.Warnings, WarningAlphaOpaque) {
		t.Errorf("facts %+v warnings %v", res.Facts, res.Warnings)
	}
}
