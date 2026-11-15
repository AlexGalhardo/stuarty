package mediagen

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
)

// animPalette is transparent, then red, green, blue, white.
var animPalette = color.Palette{
	color.RGBA{},
	color.RGBA{R: 255, A: 255},
	color.RGBA{G: 255, A: 255},
	color.RGBA{B: 255, A: 255},
	color.RGBA{R: 255, G: 255, B: 255, A: 255},
}

// animFrame is frame i of a test animation: a solid colour (red, green, blue, white in turn) with a
// transparent 2x2 corner, so the alpha of every frame is checkable.
func animFrame(w, h, i int) *image.Paletted {
	img := image.NewPaletted(image.Rect(0, 0, w, h), animPalette)
	c := uint8(1 + i%4) //nolint:gosec // i%4 is 0 to 3
	for y := range h {
		for x := range w {
			if x < 2 && y < 2 {
				img.SetColorIndex(x, y, 0)
				continue
			}
			img.SetColorIndex(x, y, c)
		}
	}
	return img
}

// AnimatedGIF is an animated GIF of w x h with one frame per entry of delaysCS, each shown for that many
// hundredths of a second. It is written by the Go encoder, so the delays are exactly as given.
func (g Gen) AnimatedGIF(name string, w, h int, delaysCS []int) (string, error) {
	a := &gif.GIF{LoopCount: 0}
	for i, d := range delaysCS {
		a.Image = append(a.Image, animFrame(w, h, i))
		a.Delay = append(a.Delay, d)
		a.Disposal = append(a.Disposal, gif.DisposalBackground)
	}
	return g.writeGIF(name, a)
}

// BombGIFFrames is an animated GIF of 1x1 frames, declaring n frames.
func (g Gen) BombGIFFrames(name string, n int) (string, error) {
	a := &gif.GIF{}
	img := animFrame(1, 1, 0)
	for range n {
		a.Image = append(a.Image, img)
		a.Delay = append(a.Delay, 2)
	}
	return g.writeGIF(name, a)
}

// BombGIFSize is a GIF of the given number of frames whose logical screen declares w x h pixels over a tiny image.
func (g Gen) BombGIFSize(name string, w, h, frames int) (string, error) {
	a := &gif.GIF{Config: image.Config{ColorModel: animPalette, Width: w, Height: h}}
	for i := range frames {
		a.Image = append(a.Image, animFrame(2, 2, i))
		a.Delay = append(a.Delay, 2)
	}
	return g.writeGIF(name, a)
}

func (g Gen) writeGIF(name string, a *gif.GIF) (string, error) {
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, a); err != nil {
		return "", fmt.Errorf("mediagen %s: %w", name, err)
	}
	p := filepath.Join(g.Dir, name)
	return p, os.WriteFile(p, b.Bytes(), 0o600)
}

// AnimatedAPNG is an animated PNG of w x h with n frames at 15 frames per second, with alpha.
func (g Gen) AnimatedAPNG(ctx context.Context, name string, w, h, n int) (string, error) {
	return g.ff(
		ctx,
		name,
		append(
			[]string{"-f", "lavfi", "-i", fmt.Sprintf("mandelbrot=s=%dx%d:r=15", w, h)},
			"-vf",
			"format=rgba",
			"-frames:v",
			strconv.Itoa(n),
			"-plays",
			"0",
			"-f",
			"apng",
		)...)
}

// AnimatedWebP is an animated WebP of w x h with n frames at 15 frames per second.
func (g Gen) AnimatedWebP(ctx context.Context, name string, w, h, n int) (string, error) {
	return g.ff(
		ctx,
		name,
		append(
			[]string{"-f", "lavfi", "-i", fmt.Sprintf("mandelbrot=s=%dx%d:r=15", w, h)},
			"-vf",
			"format=rgba",
			"-frames:v",
			strconv.Itoa(n),
			"-c:v",
			"libwebp_anim",
			"-loop",
			"0",
			"-lossless",
			"1",
			"-f",
			"webp",
		)...)
}

// StickerPNG is a w x h PNG with a straight alpha: an opaque orange disc whose edge fades to transparent, on a
// fully transparent ground. It is written by the Go encoder, so the bytes are the same on every runner.
func (g Gen) StickerPNG(name string, w, h int) (string, error) {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy, r := float64(w)/2, float64(h)/2, float64(min(w, h))/2
	for y := range h {
		for x := range w {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d := (dx*dx + dy*dy) / (r * r)
			switch {
			case d <= 0.5:
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 140, B: 0, A: 255})
			case d < 1:
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 140, B: 0, A: uint8(255 * (1 - d) * 2)})
			}
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return "", fmt.Errorf("mediagen %s: %w", name, err)
	}
	p := filepath.Join(g.Dir, name)
	return p, os.WriteFile(p, b.Bytes(), 0o600)
}
