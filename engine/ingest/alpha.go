package ingest

import (
	"image"
	"image/png"
	"os"
)

// WarningAlphaOpaque is the warning of an ingest whose asset declares an alpha channel but whose sampled pictures
// are all opaque (SDD-011 section 4.3.6). The lint rule L_ALPHA_MISSING reports the clips that use it.
const WarningAlphaOpaque = "alpha_opaque"

// pngAlphaOpaque reports whether every pixel of the PNG at path, or of its first count tiles when a layout is
// given, is fully opaque. A picture it cannot decode is not called opaque: the verification of the ingest has
// already decoded it, so an error here leaves the fact false rather than inventing a finding.
func pngAlphaOpaque(path string, layout *ThumbLayout) (opaque bool) {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	img, err := png.Decode(f)
	if err != nil {
		return false
	}

	b := img.Bounds()
	if layout == nil {
		return rectOpaque(img, b)
	}

	for i := range layout.Count {
		x0, y0 := b.Min.X+(i%layout.Columns)*layout.TileW, b.Min.Y+(i/layout.Columns)*layout.TileH
		if !rectOpaque(img, image.Rect(x0, y0, x0+layout.TileW, y0+layout.TileH).Intersect(b)) {
			return false
		}
	}

	return true
}

// rectOpaque reads the alpha of a rectangle; the Pix of the two layouts the PNG decoder makes for 8-bit colour is
// read in place, any other layout through At.
func rectOpaque(img image.Image, r image.Rectangle) bool {
	switch p := img.(type) {
	case *image.NRGBA:
		return planeOpaque(p.Pix, p.Stride, r, p.PixOffset(r.Min.X, r.Min.Y), 4, 3)
	case *image.RGBA:
		return planeOpaque(p.Pix, p.Stride, r, p.PixOffset(r.Min.X, r.Min.Y), 4, 3)
	}

	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xFFFF {
				return false
			}
		}
	}

	return true
}

func planeOpaque(pix []uint8, stride int, r image.Rectangle, start, step, alpha int) bool {
	for y := range r.Dy() {
		row := start + y*stride
		for x := range r.Dx() {
			if pix[row+x*step+alpha] != 0xFF {
				return false
			}
		}
	}

	return true
}

// markAlphaOpaque sets Facts.AlphaOpaque and warns, from the picture the ingest made of an asset that has alpha.
func (j *job) markAlphaOpaque(path string, layout *ThumbLayout) {
	if !j.res.Facts.HasAlpha || !pngAlphaOpaque(path, layout) {
		return
	}

	j.res.Facts.AlphaOpaque = true
	j.res.Warnings = append(j.res.Warnings, WarningAlphaOpaque)
}
