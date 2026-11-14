// Command installericon draws the application icon of the installers from the shapes of
// site/public/icon.svg, with the standard library only, and writes vukem-video.png and
// vukem-video.ico. The outputs are committed; run it again only when the icon changes:
//
//	go run ./internal/tools/installericon .config/installers/assets
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// The icon is a 32x32 design: a rounded violet square, a white play triangle and two bars.
type shape struct {
	inside func(x, y float64) bool
	c      color.NRGBA
}

func roundedRect(x0, y0, x1, y1, r float64) func(x, y float64) bool {
	return func(x, y float64) bool {
		if x < x0 || x > x1 || y < y0 || y > y1 {
			return false
		}
		cx, cy := math.Min(math.Max(x, x0+r), x1-r), math.Min(math.Max(y, y0+r), y1-r)
		return math.Hypot(x-cx, y-cy) <= r
	}
}

func triangle(ax, ay, bx, by, cx, cy float64) func(x, y float64) bool {
	sign := func(px, py, qx, qy, rx, ry float64) float64 { return (px-rx)*(qy-ry) - (qx-rx)*(py-ry) }
	return func(x, y float64) bool {
		d1, d2, d3 := sign(x, y, ax, ay, bx, by), sign(x, y, bx, by, cx, cy), sign(x, y, cx, cy, ax, ay)
		neg, pos := d1 < 0 || d2 < 0 || d3 < 0, d1 > 0 || d2 > 0 || d3 > 0
		return !neg || !pos
	}
}

var shapes = []shape{
	{roundedRect(0, 0, 32, 32, 8), color.NRGBA{0x7C, 0x5C, 0xFF, 0xFF}},
	{triangle(11, 7.5, 11, 18.5, 20, 13), color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}},
	{roundedRect(6, 22, 15, 25, 1.5), color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}},
	{roundedRect(17, 22, 26, 25, 1.5), color.NRGBA{0xFF, 0xFF, 0xFF, 0x99}},
}

// render draws the icon at size x size with 4x4 supersampling.
func render(size int) *image.NRGBA {
	const ss = 4
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := 32 / float64(size)
	for py := range size {
		for px := range size {
			var r, g, b, a float64
			for sy := range ss {
				for sx := range ss {
					x, y := (float64(px)+(float64(sx)+0.5)/ss)*scale, (float64(py)+(float64(sy)+0.5)/ss)*scale
					// Source-over of each shape in order, in premultiplied floats.
					var pr, pg, pb, pa float64
					for _, s := range shapes {
						if !s.inside(x, y) {
							continue
						}
						sa := float64(s.c.A) / 255
						pr, pg, pb = float64(
							s.c.R,
						)/255*sa+pr*(1-sa), float64(
							s.c.G,
						)/255*sa+pg*(1-sa), float64(
							s.c.B,
						)/255*sa+pb*(1-sa)
						pa = sa + pa*(1-sa)
					}
					r, g, b, a = r+pr, g+pg, b+pb, a+pa
				}
			}
			n := float64(ss * ss)
			if a > 0 {
				img.SetNRGBA(px, py, color.NRGBA{
					R: uint8(
						math.Round(r / a * 255),
					),
					G: uint8(math.Round(g / a * 255)),
					B: uint8(math.Round(b / a * 255)),
					A: uint8(math.Round(a / n * 255)),
				})
			}
		}
	}
	return img
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ico packs PNG images into one .ico (PNG entries are valid since Windows Vista).
func ico(sizes []int) ([]byte, error) {
	var imgs [][]byte
	for _, s := range sizes {
		b, err := encodePNG(render(s))
		if err != nil {
			return nil, err
		}
		imgs = append(imgs, b)
	}
	var out bytes.Buffer
	le := func(v any) { _ = binary.Write(&out, binary.LittleEndian, v) }
	le(uint16(0))
	le(uint16(1))
	le(uint16(len(sizes) & 0xFFFF))
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		w := byte(s & 0xFF)
		if s >= 256 {
			w = 0
		}
		out.Write([]byte{w, w, 0, 0})
		le(uint16(1))
		le(uint16(32))
		le(uint32(len(imgs[i]) & math.MaxUint32))
		le(uint32(offset & math.MaxUint32))
		offset += len(imgs[i])
	}
	for _, b := range imgs {
		out.Write(b)
	}
	return out.Bytes(), nil
}

// write stores one output file; dir is the argument of a developer tool.
func write(dir, name string, data []byte) error {
	if err := ensureDir(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	return os.WriteFile(path, data, 0o600) //nolint:gosec // G703: the output folder is the argument of a developer tool
}

func ensureDir(dir string) error {
	err := os.MkdirAll(dir, 0o750) //nolint:gosec // G703: the output folder is the argument of a developer tool
	return err
}

func run(dir string) error {
	p, err := encodePNG(render(256))
	if err != nil {
		return err
	}
	if err := write(dir, "vukem-video.png", p); err != nil {
		return err
	}
	i, err := ico([]int{16, 32, 48, 256})
	if err != nil {
		return err
	}
	return write(dir, "vukem-video.ico", i)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: installericon <output directory>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "installericon:", err)
		os.Exit(1)
	}
}
