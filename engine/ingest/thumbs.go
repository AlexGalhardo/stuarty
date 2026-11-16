package ingest

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
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

// ErrTooManyTiles means more tile files than the plan allows: the sheet is sized from the count, so
// a count the tool chose must never size an allocation.
var ErrTooManyTiles = errors.New("thumbnails: more tiles than planned")

// BuildSheet decodes the tile files in order, places them row by row and left to right on a
// black canvas of columns * tile_w by rows * tile_h, and encodes it as a JPEG. Standard
// library only. Every tile must decode and have the planned size. Tiles that are PNG files (a
// source with alpha, SDD-011 4.3.5) are placed on a transparent canvas and the sheet is a PNG.
func BuildSheet(tilePaths []string, plan ThumbPlan) ([]byte, ThumbLayout, error) {
	n := len(tilePaths)
	if n == 0 {
		return nil, ThumbLayout{}, errors.New("thumbnails: no tiles")
	}
	if n > ThumbMaxCount || (plan.Planned > 0 && n > plan.Planned+1) {
		return nil, ThumbLayout{}, fmt.Errorf("%w: %d tiles", ErrTooManyTiles, n)
	}
	if plan.TileW < 1 || plan.TileW > ThumbMaxTileWidth || plan.TileH != ThumbHeight {
		return nil, ThumbLayout{}, errors.New("thumbnails: tile size outside the plan")
	}
	l := ThumbLayout{
		TileW:      plan.TileW,
		TileH:      plan.TileH,
		Columns:    min(n, ThumbColumns),
		Count:      n,
		IntervalMS: plan.IntervalS * 1000,
	}
	alpha := strings.EqualFold(filepath.Ext(tilePaths[0]), ".png")
	sheet := image.NewRGBA(image.Rect(0, 0, l.Columns*l.TileW, l.Rows()*l.TileH))
	if !alpha {
		draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	}
	for i, p := range tilePaths {
		img, err := decodeTile(p, alpha)
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
	if alpha {
		if err := png.Encode(&buf, sheet); err != nil {
			return nil, l, err
		}
		return buf.Bytes(), l, nil
	}
	if err := jpeg.Encode(&buf, sheet, &jpeg.Options{Quality: SheetJPEGQuality}); err != nil {
		return nil, l, err
	}
	return buf.Bytes(), l, nil
}

// decodeTile decodes a tile file, a PNG or a JPEG as the sheet needs.
func decodeTile(path string, alpha bool) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	// The header first: the size a file declares must not size an allocation.
	decodeConfig, decode := jpeg.DecodeConfig, jpeg.Decode
	if alpha {
		decodeConfig, decode = png.DecodeConfig, png.Decode
	}
	cfg, err := decodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width > ThumbMaxTileWidth || cfg.Height > ThumbHeight {
		return nil, fmt.Errorf("tile declares %dx%d", cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	return decode(f)
}
