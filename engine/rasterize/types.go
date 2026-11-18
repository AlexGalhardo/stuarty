package rasterize

import (
	"errors"

	"github.com/AlexGalhardo/vukem-video/engine/bitmap"
)

// Bitmap is a rendered bitmap and its metrics.
type Bitmap = bitmap.Bitmap

// Kinds of plan.BitmapSpec.
const (
	KindText    = "text"
	KindCaption = "caption"
	KindShape   = "shape"
)

// Shapes of plan.ShapeSpec (SDD-001 section 3.6).
const (
	ShapeRect        = "rect"
	ShapeRoundedRect = "rounded_rect"
	ShapeEllipse     = "ellipse"
)

// Alignments of plan.ResolvedStyle.
const (
	AlignLeft   = "left"
	AlignCenter = "center"
	AlignRight  = "right"
)

// ErrInvalidSpec is returned for a spec the rasterizer cannot draw.
var ErrInvalidSpec = errors.New("raster: invalid spec")

// MaxPixels bounds the size of a bitmap: a 4x oversampled 1080 by 1920 frame
// is 33 million pixels, which is also the largest bitmap a text can need.
const MaxPixels = 4096 * 8192
