package renderhost

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// preparedHeader is the bytes before the pixels of a prepared block (SDD-013 section 4.2.2): width, height,
// flags and a zero as u32, then the grown width and height, the offset of the centre and the scale as f64.
const preparedHeader = 16 + 40

// preparedWindowFlag marks a block that holds a window of its picture; four more u32 follow the header: where the
// window starts in the picture and the size of the picture.
const (
	preparedWindowFlag = 2
	preparedWindowHead = 16
)

// Prepared is a layer after its looks, the block `vr_prepare` gives (SDD-013 section 4.2.2): its pixels, the
// size of the rectangle that draws them and what places it again. It is a plain value: the host that made it
// and any other instance of the module read it the same way, so it can be kept and uploaded again.
type Prepared struct {
	block []byte
}

// PreparedSize is the pixels of a prepared layer and the rectangle that draws them.
type PreparedSize struct {
	// W and H are the pixels of the prepared picture.
	W, H int
	// GrownW and GrownH are the size, in output pixels, of the rectangle that draws it, centred where the layer
	// is (a blur makes it larger than the layer).
	GrownW, GrownH float64
	// OffsetX and OffsetY are what a bitmap's metrics move its centre by; zero for a picture.
	OffsetX, OffsetY float64
	// Scale is the output scale it was prepared at.
	Scale float64
}

// Empty reports that the layer draws nothing.
func (p Prepared) Empty() bool { return len(p.block) == 0 }

// Bytes is the memory the prepared layer takes.
func (p Prepared) Bytes() int { return len(p.block) }

// Size is the geometry of a prepared layer; the zero value for an empty one.
func (p Prepared) Size() PreparedSize {
	if len(p.block) < preparedHeader {
		return PreparedSize{}
	}

	f := func(i int) float64 { return math.Float64frombits(binary.LittleEndian.Uint64(p.block[i:])) }

	return PreparedSize{
		W: int(binary.LittleEndian.Uint32(p.block)), H: int(binary.LittleEndian.Uint32(p.block[4:])),
		GrownW: f(16), GrownH: f(24), OffsetX: f(32), OffsetY: f(40), Scale: f(48),
	}
}

// Prepare runs the preparation of the one layer of the plan (SDD-013 section 4.2.2) at opacity 1 and without
// its clip: its texture or bitmap, and the tables it names, must be stored. A layer that draws nothing gives an
// empty Prepared.
func (h *Host) Prepare(ctx context.Context, in RenderInput) (Prepared, error) {
	if len(in.Plan.Layers) != 1 {
		return Prepared{}, &Error{InvalidArgument, "Prepare takes exactly one layer"}
	}

	text, err := json.Marshal(in)
	if err != nil {
		return Prepared{}, fmt.Errorf("renderhost: encoding the render input: %w", err)
	}

	input, err := h.put(ctx, text)
	if err != nil {
		return Prepared{}, err
	}

	defer h.release(ctx, input)

	if err := h.status(ctx, "vr_prepare", uint64(input.ptr), uint64(input.n)); err != nil {
		return Prepared{}, err
	}

	defer func() {
		if h.dead == nil {
			_, _ = h.call(ctx, "vr_prepared_clear")
		}
	}()

	res, err := h.call(ctx, "vr_prepared_len")
	if err != nil {
		return Prepared{}, err
	}

	n := uint32(res[0]) //nolint:gosec // G115: a u32 result
	if n == 0 {
		return Prepared{}, nil
	}

	res, err = h.call(ctx, "vr_prepared_ptr")
	if err != nil {
		return Prepared{}, err
	}

	b, ok := h.mem.Read(uint32(res[0]), n) //nolint:gosec // G115: an offset into a 32-bit memory
	if !ok || n < preparedHeader {
		return Prepared{}, &Error{OutOfMemory, "reading the prepared layer"}
	}

	return Prepared{block: append([]byte(nil), b...)}, nil
}

// PutPrepared stores a prepared layer as the texture of the item id: a media layer of that item is then drawn
// with no look (SDD-013 section 4.2.2). The module copies the block.
func (h *Host) PutPrepared(ctx context.Context, item string, p Prepared) error {
	if len(item) > maxIDBytes || len(p.block) < preparedHeader {
		return &Error{InvalidArgument, "PutPrepared"}
	}

	id, err := h.put(ctx, []byte(item))
	if err != nil {
		return err
	}

	blk, err := h.put(ctx, p.block)
	if err != nil {
		h.release(ctx, id)

		return err
	}

	defer h.release(ctx, id, blk)

	return h.status(ctx, "vr_put_prepared", uint64(id.ptr), uint64(id.n), uint64(blk.ptr), uint64(blk.n))
}

// Reset forgets every texture, bitmap and lookup table the instance holds, as vr_init does, and keeps the memory
// the instance has grown to.
func (h *Host) Reset(ctx context.Context) error {
	return h.status(ctx, "vr_init", uint64(h.maxTex), uint64(h.maxBM))
}

// MemoryBytes is the memory the instance holds; 0 once it is closed.
func (h *Host) MemoryBytes() int64 {
	if h.dead != nil {
		return 0
	}

	return int64(h.mem.Size())
}

// Window is the prepared layer cut to the pixels [x0, x1) by [y0, y1) of its picture, clamped to it: the module
// draws it as it draws the whole picture, with the same scale, edges and clamps, as long as the window holds
// every pixel the draw reads (SDD-013 section 4.2.2). A window that is the whole picture is the layer itself.
func (p Prepared) Window(x0, y0, x1, y1 int) Prepared {
	sz := p.Size()
	x0, y0, x1, y1 = max(x0, 0), max(y0, 0), min(x1, sz.W), min(y1, sz.H)

	if x0 >= x1 || y0 >= y1 {
		return Prepared{}
	}

	if x0 == 0 && y0 == 0 && x1 == sz.W && y1 == sz.H {
		return p
	}

	out := make([]byte, 0, preparedHeader+preparedWindowHead+(x1-x0)*(y1-y0)*4)
	out = append(out, p.block[:preparedHeader]...)
	binary.LittleEndian.PutUint32(out[0:], uint32(x1-x0)) //nolint:gosec // G115: inside the picture
	binary.LittleEndian.PutUint32(out[4:], uint32(y1-y0)) //nolint:gosec // G115: inside the picture
	binary.LittleEndian.PutUint32(out[8:], binary.LittleEndian.Uint32(p.block[8:])|preparedWindowFlag)

	for _, v := range []int{x0, y0, sz.W, sz.H} {
		out = binary.LittleEndian.AppendUint32(out, uint32(v)) //nolint:gosec // G115: inside the picture
	}

	for y := y0; y < y1; y++ {
		o := preparedHeader + (y*sz.W+x0)*4
		out = append(out, p.block[o:o+(x1-x0)*4]...)
	}

	return Prepared{block: out}
}
