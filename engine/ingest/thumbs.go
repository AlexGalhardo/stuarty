package ingest

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
)

// ThumbLayout is the layout stored in artifacts.meta of the sheet and returned as strip
// (SDD-011 4.5).
type ThumbLayout struct {
	TileW      int `json:"tile_w"`
	TileH      int `json:"tile_h"`
	Columns    int `json:"columns"`
	Count      int `json:"count"`
	IntervalMS int `json:"interval_ms"`
}

// Rows is ceil(count / columns).
func (l ThumbLayout) Rows() int { return (l.Count + l.Columns - 1) / l.Columns }

// SheetJPEGQuality is the quality of the sheet (SDD-011 4.5).
const SheetJPEGQuality = 80

// BuildSheet decodes the tile files in order, places them row by row and left to right on a
// black canvas of columns * tile_w by rows * tile_h, and encodes it as a JPEG. Standard
// library only. Every tile must decode and have the planned size.
func BuildSheet(tilePaths []string, plan ThumbPlan) ([]byte, ThumbLayout, error) {
	n := len(tilePaths)
	if n == 0 {
		return nil, ThumbLayout{}, errors.New("thumbnails: no tiles")
	}
	l := ThumbLayout{
		TileW:      plan.TileW,
		TileH:      plan.TileH,
		Columns:    min(n, ThumbColumns),
		Count:      n,
		IntervalMS: plan.IntervalS * 1000,
	}
	sheet := image.NewRGBA(image.Rect(0, 0, l.Columns*l.TileW, l.Rows()*l.TileH))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	for i, p := range tilePaths {
		img, err := decodeJPEG(p)
		if err != nil {
			return nil, l, fmt.Errorf("thumbnails: tile %d: %w", i, err)
		}
		if b := img.Bounds(); b.Dx() != l.TileW || b.Dy() != l.TileH {
			return nil, l, fmt.Errorf("thumbnails: tile %d is %dx%d, want %dx%d", i, b.Dx(), b.Dy(), l.TileW, l.TileH)
		}
		x, y := (i%l.Columns)*l.TileW, (i/l.Columns)*l.TileH
		draw.Draw(sheet, image.Rect(x, y, x+l.TileW, y+l.TileH), img, img.Bounds().Min, draw.Src)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, sheet, &jpeg.Options{Quality: SheetJPEGQuality}); err != nil {
		return nil, l, err
	}
	return buf.Bytes(), l, nil
}

func decodeJPEG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return jpeg.Decode(f)
}
