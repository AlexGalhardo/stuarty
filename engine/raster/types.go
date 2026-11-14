package raster

import "github.com/AlexGalhardo/vukem-video/core/plan"

// Version is bumped on any change that alters output bytes (SDD-004 section 5).
// It must equal plan.BitmapVersion, which is part of every bitmap key:
// TestVersionMatchesThePlan fails when they differ.
const Version = 1

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

// Colour is a straight (non-premultiplied) RGBA colour.
type Colour struct {
	R, G, B, A uint8
}

// style is the resolved style of a text or caption: the fields of
// plan.ResolvedStyle with the colours parsed, so drawing needs no parsing.
type style struct {
	fontFamily string
	fontWeight int
	size       float64
	color      Colour

	strokeColor Colour
	strokeWidth float64

	shadowColor Colour
	shadowDX    float64
	shadowDY    float64
	shadowBlur  float64

	// background is nil when there is no box behind the text.
	background        *Colour
	backgroundPadding float64
	backgroundRadius  float64

	align      string
	lineHeight float64
	// maxWidth is in pixels: lines wrap at this width.
	maxWidth  float64
	uppercase bool

	// highlight is false when the style says "none": the active word is then
	// drawn like the others.
	highlight           bool
	highlightColor      Colour
	highlightBackground *Colour
}

// shape is plan.ShapeSpec with the colours parsed.
type shape struct {
	kind        string
	width       float64
	height      float64
	radius      float64
	fill        Colour
	strokeColor Colour
	strokeWidth float64
}

// Bitmap is a rendered bitmap and its metrics.
type Bitmap struct {
	// Pix is RGBA, 8 bits per channel, straight (non-premultiplied) alpha,
	// Width*Height*4 bytes. It is shared by the cache: do not modify it.
	Pix           []byte
	Width, Height int
	// NaturalW and NaturalH are the size of the bitmap at PixelScale 1, in
	// frame pixels.
	NaturalW, NaturalH float64
	// CenterDX and CenterDY are the offset, at PixelScale 1, from the centre
	// of the bitmap to the centre of the text block (or shape). The item's
	// position names the centre of the block, so a shadow that reaches further
	// on one side moves the bitmap, not the block.
	CenterDX, CenterDY float64
	// WordBoxes is the box of each word of a caption at PixelScale 1, in the
	// coordinates of the bitmap, for the lint and the UI.
	WordBoxes []plan.Rect
}
