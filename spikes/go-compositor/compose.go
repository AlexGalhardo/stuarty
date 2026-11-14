package main

import (
	"image"
	"image/color"
	"math"
	"sync"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
)

const (
	W, H      = 1080, 1920
	FPS       = 30
	Seconds   = 30
	Frames    = FPS * Seconds
	BmpW      = 900
	BmpH      = 180
	TextY     = 1310
	FadeSec   = 0.25
	TextX0    = 40.0
	TextSpeed = 60.0

	PipW, PipH   = 432, 243 // C3 sprite size
	LandW, LandH = 1920, 1080
	ScrW, ScrH   = 640, 440 // C4 scratch (bounding box of the rotated sprite at scale 0.3)
	PipWeight    = 205      // 80% opacity as n/256
)

// ---- text rasterization (same as the export-pipeline spike) ----

func renderTexts() []*image.NRGBA {
	f, err := opentype.Parse(goregular.TTF)
	check(err)
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 100, DPI: 72, Hinting: font.HintingFull})
	check(err)
	bmps := make([]*image.NRGBA, Seconds)
	for k := range bmps {
		img := image.NewNRGBA(image.Rect(0, 0, BmpW, BmpH))
		for _, layer := range []struct {
			c  color.NRGBA
			dx int
		}{{color.NRGBA{0, 0, 0, 160}, 5}, {color.NRGBA{255, 255, 255, 255}, 0}} {
			d := &font.Drawer{Dst: img, Src: image.NewUniform(layer.c), Face: face,
				Dot: fixed.P(20+layer.dx, 130+layer.dx)}
			d.DrawString(textFor(k))
		}
		bmps[k] = img
	}
	return bmps
}

func fade(u float64) float64 {
	return math.Max(0, math.Min(1, math.Min(u/FadeSec, (1-u)/FadeSec)))
}

// ---- composer ----

type composer struct {
	caseName string
	bands    int
	interp   draw.Transformer // C4 sampler
	bmps     []*image.NRGBA
	cm       [12]int32 // colour matrix, Q10: 3 rows x (r,g,b,offset)
	scratch  *image.RGBA
}

func newComposer(caseName string, bands int, approx bool) *composer {
	c := &composer{caseName: caseName, bands: bands, bmps: renderTexts()}
	if approx {
		c.interp = draw.ApproxBiLinear
	} else {
		c.interp = draw.BiLinear
	}
	c.scratch = image.NewRGBA(image.Rect(0, 0, ScrW, ScrH))
	c.cm = colourMatrix(0.05, 1.10, 1.20)
	return c
}

// colourMatrix builds a 3x4 matrix (Q10 fixed point) = brightness + contrast + saturation on 0..255 values.
func colourMatrix(brightness, contrast, saturation float64) [12]int32 {
	lum := [3]float64{0.2126, 0.7152, 0.0722}
	var m [12]int32
	for r := 0; r < 3; r++ {
		for k := 0; k < 3; k++ {
			v := (1 - saturation) * lum[k]
			if r == k {
				v += saturation
			}
			m[r*4+k] = int32(math.Round(v * contrast * 1024))
		}
		m[r*4+3] = int32(math.Round(255 * (0.5 - 0.5*contrast + brightness) * 1024))
	}
	return m
}

func (c *composer) compose(n int, bg, pip, out []byte) {
	var phase1 func(y0, y1 int)
	if c.caseName == "c4" {
		phase1 = func(y0, y1 int) { c.transformBand(n, pip, y0, y1) }
		runBands(c.bands, ScrH, phase1)
	}
	runBands(c.bands, H, func(y0, y1 int) { c.band(n, bg, pip, out, y0, y1) })
}

func runBands(bands, rows int, fn func(y0, y1 int)) {
	if bands <= 1 {
		fn(0, rows)
		return
	}
	var wg sync.WaitGroup
	for i := 0; i < bands; i++ {
		y0, y1 := i*rows/bands, (i+1)*rows/bands
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(y0, y1)
		}()
	}
	wg.Wait()
}

func pipPos(n int) (int, int) { return 40 + (n*13)%568, 300 + (n*7)%900 }
func scrPos(n int) (int, int) { return 40 + (n*13)%360, 300 + (n*7)%900 }

func (c *composer) band(n int, bg, pip, out []byte, y0, y1 int) {
	// 1. video layer into the output buffer
	if c.caseName == "c5" {
		colourCopy(out[y0*W*4:y1*W*4], bg[y0*W*4:y1*W*4], &c.cm)
	} else {
		copy(out[y0*W*4:y1*W*4], bg[y0*W*4:y1*W*4])
	}
	// 2. picture-in-picture
	switch c.caseName {
	case "c3":
		px, py := pipPos(n)
		for y := max(y0, py); y < min(y1, py+PipH); y++ {
			blendOpaque(out[(y*W+px)*4:(y*W+px+PipW)*4], pip[(y-py)*PipW*4:(y-py+1)*PipW*4])
		}
	case "c4":
		sx, sy := scrPos(n)
		for y := max(y0, sy); y < min(y1, sy+ScrH); y++ {
			s := c.scratch.Pix[(y-sy)*c.scratch.Stride : (y-sy)*c.scratch.Stride+ScrW*4]
			blendOpaque(out[(y*W+sx)*4:(y*W+sx+ScrW)*4], s)
		}
	}
	// 3. text
	k, u := n/FPS, float64(n%FPS)/FPS
	tx := int(TextX0 + TextSpeed*u)
	f := int(fade(u)*256 + 0.5)
	b := c.bmps[k]
	for y := max(y0, TextY); y < min(y1, TextY+BmpH); y++ {
		blendText(out[(y*W+tx)*4:(y*W+tx+BmpW)*4], b.Pix[(y-TextY)*b.Stride:(y-TextY)*b.Stride+BmpW*4], f)
	}
}

// blendText: straight-alpha NRGBA row over opaque dst row; f is an extra alpha factor in 0..256.
func blendText(d, s []byte, f int) {
	for i := 0; i+3 < len(s); i += 4 {
		a := int(s[i+3]) * f >> 8
		if a == 0 {
			continue
		}
		if a >= 255 {
			d[i], d[i+1], d[i+2] = s[i], s[i+1], s[i+2]
			continue
		}
		ia := 255 - a
		d[i] = div255(int(s[i])*a + int(d[i])*ia)
		d[i+1] = div255(int(s[i+1])*a + int(d[i+1])*ia)
		d[i+2] = div255(int(s[i+2])*a + int(d[i+2])*ia)
	}
}

func div255(v int) byte {
	v += 128
	return byte((v + v>>8) >> 8)
}

// blendOpaque: 80% blend of s over d; pixels with s alpha 0 are skipped (C4 scratch outside the sprite).
func blendOpaque(d, s []byte) {
	for i := 0; i+3 < len(s); i += 4 {
		if s[i+3] == 0 {
			continue
		}
		d[i] = byte((int(s[i])*PipWeight + int(d[i])*(256-PipWeight) + 128) >> 8)
		d[i+1] = byte((int(s[i+1])*PipWeight + int(d[i+1])*(256-PipWeight) + 128) >> 8)
		d[i+2] = byte((int(s[i+2])*PipWeight + int(d[i+2])*(256-PipWeight) + 128) >> 8)
	}
}

func clamp255(v int32) byte {
	if uint32(v) > 255 {
		if v < 0 {
			return 0
		}
		return 255
	}
	return byte(v)
}

func colourCopy(d, s []byte, m *[12]int32) {
	for i := 0; i+3 < len(s); i += 4 {
		r, g, b := int32(s[i]), int32(s[i+1]), int32(s[i+2])
		d[i] = clamp255((m[0]*r + m[1]*g + m[2]*b + m[3] + 512) >> 10)
		d[i+1] = clamp255((m[4]*r + m[5]*g + m[6]*b + m[7] + 512) >> 10)
		d[i+2] = clamp255((m[8]*r + m[9]*g + m[10]*b + m[11] + 512) >> 10)
		d[i+3] = 255
	}
}

// transformBand renders the scaled+rotated landscape frame into scratch rows [y0,y1).
func (c *composer) transformBand(n int, pip []byte, y0, y1 int) {
	rect := image.Rect(0, y0, ScrW, y1)
	clear(c.scratch.Pix[y0*c.scratch.Stride : y1*c.scratch.Stride])
	src := &image.RGBA{Pix: pip, Stride: LandW * 4, Rect: image.Rect(0, 0, LandW, LandH)}
	scale := 0.2 + 0.1*float64(n)/float64(Frames-1)
	sn, cs := math.Sincos(10 * math.Pi / 180)
	a, b, cc, d := scale*cs, -scale*sn, scale*sn, scale*cs
	tx := ScrW/2 - (a*LandW/2 + b*LandH/2)
	ty := ScrH/2 - (cc*LandW/2 + d*LandH/2)
	m := f64.Aff3{a, b, tx, cc, d, ty}
	c.interp.Transform(c.scratch.SubImage(rect).(*image.RGBA), m, src, src.Bounds(), draw.Src, nil)
}
