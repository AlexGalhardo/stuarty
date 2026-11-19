package frame

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/cache"
	"github.com/AlexGalhardo/stuarty/engine/export"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
)

// Frames makes frames and sheets for the documents of one workspace.
type Frames struct {
	Runner       *ffmpeg.Runner
	ArtifactsDir string
	Fonts        *rasterize.FontSet // the bundled fonts when nil
	// Cache holds the JPEGs by project, revision, frame and width. It may be nil: nothing is kept.
	Cache *cache.Store
}

// Ref names the version of the project a document is: the cache key is made of it.
type Ref struct {
	Project  string
	Revision int64
}

// ErrRange is returned for a time or a width the project cannot show.
var ErrRange = errors.New("frame: out of range")

func (f *Frames) renderer(ctx context.Context, doc *timeline.Document) (*export.Renderer, error) {
	var id [6]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	return export.NewRenderer(ctx, export.RenderConfig{
		Runner: f.Runner, Doc: doc, JobID: "frame-" + hex.EncodeToString(id[:]),
		ArtifactsDir: f.ArtifactsDir, Fonts: f.Fonts,
	})
}

func lastFrame(doc *timeline.Document) int {
	end := 0
	for id := range doc.Items {
		end = max(end, doc.Items[id].Start+doc.Items[id].Duration)
	}
	return max(end-1, 0)
}

func encode(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: JPEGQuality}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Still is frame t of the document at the project's own size, as a lossless PNG (VV-037): a
// thumbnail or a proof for a client. It is composed by the same compositor as an export and is not
// cached. A time past the end is the last frame.
func (f *Frames) Still(ctx context.Context, doc *timeline.Document, t int) ([]byte, error) {
	if t < 0 {
		return nil, fmt.Errorf("%w: frame %d", ErrRange, t)
	}

	r, err := f.renderer(ctx, doc)
	if err != nil {
		return nil, err
	}

	defer func() { _ = r.Close() }()

	img, err := r.Render(ctx, min(t, lastFrame(doc)), 1)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

// Frame is frame t of the document at the given width, as a JPEG of quality 80 (4:2:0), from the
// cache when it is there. A time past the end is the last frame.
func (f *Frames) Frame(ctx context.Context, doc *timeline.Document, ref Ref, t, width int) ([]byte, error) {
	return f.FrameSized(ctx, doc, ref, t, width, 0)
}

// FrameSized is Frame with the exact height of the picture, for a caller that took the size from
// ImageSize; a height of zero is the one the width gives, round(width*H/W).
func (f *Frames) FrameSized(
	ctx context.Context, doc *timeline.Document, ref Ref, t, width, height int,
) ([]byte, error) {
	if t < 0 || width < 16 || width > MaxWidth {
		return nil, fmt.Errorf("%w: frame %d at width %d", ErrRange, t, width)
	}
	t = min(t, lastFrame(doc))
	st := doc.Settings
	derived := max(int(math.Round(float64(width)*float64(st.Height)/float64(st.Width))), 1)
	key := cache.FrameKey(ref.Project, ref.Revision, t, width)
	if height <= 0 {
		height = derived
	}
	if height != derived {
		key = cache.FrameKeyHeight(ref.Project, ref.Revision, t, width, height)
	}
	if f.Cache != nil {
		if b, ok := f.Cache.Get(key); ok {
			return b, nil
		}
	}

	r, err := f.renderer(ctx, doc)
	if err != nil {
		return nil, err
	}
	img, err := r.RenderSize(ctx, t, width, height)
	if err != nil {
		_ = r.Close()

		return nil, err
	}
	// The renderer is not needed to encode the pixels: its work folder is removed while the JPEG is
	// made, and the answer waits for both (NFR-004).
	closed := make(chan struct{})

	go func() {
		_ = r.Close()

		close(closed)
	}()

	out, err := encode(img)

	<-closed

	if err != nil {
		return nil, err
	}
	if f.Cache != nil {
		_ = f.Cache.Put(key, out)
	}
	return out, nil
}

// SheetTimes is count frames evenly spaced from from to to, both ends included, each rounded to the
// nearest frame.
func SheetTimes(from, to, count int) []int {
	if count <= 1 {
		return []int{from}
	}
	out := make([]int, count)
	span := to - from
	for i := range count {
		out[i] = from + (2*i*span+(count-1))/(2*(count-1))
	}
	return out
}

// SheetRequest says which frames and how big. Cols zero is ceil(sqrt(count)); Budget zero is the default
// sheet budget of 1,500 tokens.
type SheetRequest struct {
	Times  []int
	Cols   int
	Budget int
}

// Cell is one picture of a sheet.
type Cell struct {
	Index   int // 1-based, as printed in the label
	Frame   int
	Seconds float64
}

// Sheet is a contact sheet: one JPEG and what is in each cell.
type Sheet struct {
	JPEG          []byte
	Width, Height int
	// Key is the cache key of the file, which a caller turns into a path.
	Key    string
	Tokens int
	Cells  []Cell
}

func columns(count int) int {
	c := 1
	for c*c < count {
		c++
	}
	return c
}

// Sheet composes the frames of req.Times into one JPEG (SDD-003 section 3.11): a grid sized by ImageSize with
// the sheet budget, a 2-pixel gutter of the background colour, and the time printed in the top-left
// corner of each cell by the rasterizer.
func (f *Frames) Sheet(ctx context.Context, doc *timeline.Document, ref Ref, req SheetRequest) (*Sheet, error) {
	n := len(req.Times)
	if n < 1 || n > MaxSheetCells {
		return nil, fmt.Errorf("%w: a sheet has 1 to %d cells, not %d", ErrRange, MaxSheetCells, n)
	}
	cols := req.Cols
	if cols <= 0 {
		cols = columns(n)
	}
	cols = min(cols, n)
	rows := (n + cols - 1) / cols
	budget := req.Budget
	if budget <= 0 {
		budget = DefaultSheetBudg
	}
	budget = min(budget, MaxTokens)

	st := doc.Settings
	cw, ch, tokens := ImageSize(cols*st.Width, rows*st.Height, budget)
	times := make([]int, n)
	cells := make([]Cell, n)
	last := lastFrame(doc)
	for i, t := range req.Times {
		if t < 0 {
			return nil, fmt.Errorf("%w: frame %d", ErrRange, t)
		}
		times[i] = min(t, last)
		cells[i] = Cell{
			Index: i + 1, Frame: times[i],
			Seconds: float64(times[i]) * float64(st.FPS.Den) / float64(st.FPS.Num),
		}
	}
	out := &Sheet{Width: cw, Height: ch, Tokens: tokens, Cells: cells}

	sum := sha256.Sum256([]byte(fmt.Sprint(times, cols, budget)))
	key := cache.SheetKey(ref.Project, ref.Revision, hex.EncodeToString(sum[:]), cw)
	out.Key = key
	if f.Cache != nil {
		if b, ok := f.Cache.Get(key); ok {
			out.JPEG = b
			return out, nil
		}
	}

	canvas := image.NewRGBA(image.Rect(0, 0, cw, ch))
	bg := background(st.Background)
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)

	r, err := f.renderer(ctx, doc)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()

	const gutter = 2
	cellW, cellH := cw/cols, ch/rows
	fonts := f.Fonts
	if fonts == nil {
		fonts = rasterize.Bundled()
	}
	for i, c := range cells {
		x0, y0 := (i%cols)*cellW, (i/cols)*cellH
		img, err := r.Render(ctx, c.Frame, float64(cellW-gutter)/float64(st.Width))
		if err != nil {
			return nil, err
		}
		area := image.Rect(x0, y0, x0+cellW-gutter, y0+cellH-gutter)
		draw.Draw(canvas, area, img, image.Point{}, draw.Src)
		if err := label(canvas, area, fonts, c); err != nil {
			return nil, err
		}
	}

	if out.JPEG, err = encode(canvas); err != nil {
		return nil, err
	}
	if f.Cache != nil {
		_ = f.Cache.Put(key, out.JPEG)
	}
	return out, nil
}

// label prints "<index> <seconds>s f<frame>" in the top-left corner of the cell: white Inter 700 of
// max(12, cell height / 28) pixels on a black box at 70% opacity, drawn by the rasterizer.
func label(canvas *image.RGBA, area image.Rectangle, fonts *rasterize.FontSet, c Cell) error {
	size := float64(max(12, area.Dy()/28))
	spec := plan.BitmapSpec{
		Kind: rasterize.KindText, ActiveWord: -1, PixelScale: 1,
		Text: fmt.Sprintf("%d %ss f%d", c.Index, secondsText(c.Seconds), c.Frame),
		Style: plan.ResolvedStyle{
			FontFamily:        "Inter",
			FontWeight:        700,
			Size:              size,
			Color:             "#FFFFFF",
			StrokeColor:       "#000000",
			ShadowColor:       "#00000000",
			BackgroundColor:   "#000000B3",
			BackgroundPadding: size / 4,
			Align:             "left",
			LineHeight:        1.2,
			MaxWidth:          float64(area.Dx()),
		},
	}
	bm, err := rasterize.Render(spec, fonts)
	if err != nil {
		return fmt.Errorf("sheet label: %w", err)
	}
	clip := area.Intersect(image.Rect(area.Min.X, area.Min.Y, area.Min.X+bm.Width, area.Min.Y+bm.Height))
	for y := clip.Min.Y; y < clip.Max.Y; y++ {
		for x := clip.Min.X; x < clip.Max.X; x++ {
			s := bm.Pix[((y-area.Min.Y)*bm.Width+(x-area.Min.X))*4:]
			a := uint32(s[3])
			if a == 0 {
				continue
			}
			d := canvas.Pix[canvas.PixOffset(x, y):]
			for k := range 3 {
				d[k] = uint8((uint32(s[k])*a + uint32(d[k])*(255-a) + 127) / 255) //nolint:gosec // at most 255
			}
		}
	}
	return nil
}

// secondsText is a time with at most two decimals: 1 is "1", 0.5 is "0.5", 1.0333 is "1.03".
func secondsText(sec float64) string {
	return strconv.FormatFloat(math.Round(sec*100)/100, 'f', -1, 64)
}

// background is the opaque colour of #RRGGBB (black for anything else).
func background(c timeline.Color) color.RGBA {
	out := color.RGBA{A: 255}
	str := string(c)
	if len(str) < 7 || str[0] != '#' {
		return out
	}
	b, err := hex.DecodeString(str[1:7])
	if err != nil {
		return out
	}
	out.R, out.G, out.B = b[0], b[1], b[2]
	return out
}
