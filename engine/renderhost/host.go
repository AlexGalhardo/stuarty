// Package renderhost runs the core module of the Rust renderer (renderer/, SDD-013) from Go with wazero: pure
// Go, no cgo, no import for the module to reach anything through. It is the only place that knows the exports
// of SDD-013 section 4.2.
package renderhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/AlexGalhardo/vukem-video/engine/bitmap"
	"github.com/AlexGalhardo/vukem-video/engine/lut"
)

// Default budgets of vr_init (SDD-013 section 7.2).
const (
	DefaultMaxTextureBytes = 512 << 20
	DefaultMaxBitmapBytes  = 128 << 20
)

// maxIDBytes is the longest item id or bitmap key (SDD-013 section 4.2).
const maxIDBytes = 64

// The largest sides, as the module enforces them (SDD-013 section 7.2); the host checks them first so that a
// size never wraps in the 32-bit call.
const (
	maxOutputSide  = 4096
	maxTextureSide = 8192
)

// errorBufBytes is the room the host gives vr_error; a detail names an id or a limit and is shorter.
const errorBufBytes = 256

// Reason is a reason of SDD-013 section 7.1.
type Reason string

// The reasons a call can fail with; RenderTrap has no status, the call itself failed.
const (
	InvalidPlan        Reason = "invalid_plan"
	UnsupportedVersion Reason = "unsupported_version"
	UnsupportedLayer   Reason = "unsupported_layer"
	MissingTexture     Reason = "missing_texture"
	LimitExceeded      Reason = "limit_exceeded"
	OutOfMemory        Reason = "out_of_memory"
	InvalidArgument    Reason = "invalid_argument"
	InvalidSpec        Reason = "invalid_spec"
	FontUnknown        Reason = "font_unknown"
	FontInvalid        Reason = "font_invalid"
	FontNeeded         Reason = "font_needed"
	TextFailed         Reason = "text_failed"
	RenderTrap         Reason = "render_trap"
)

// statusReasons maps the negative statuses of SDD-013 section 4.5 to their reasons.
var statusReasons = map[int32]Reason{
	-1: InvalidPlan, -2: UnsupportedVersion, -3: UnsupportedLayer, -4: MissingTexture,
	-5: LimitExceeded, -6: OutOfMemory, -7: InvalidArgument,
	-8: InvalidSpec, -9: FontUnknown, -10: FontInvalid, -11: FontNeeded, -12: TextFailed,
}

// Error is a failure of the renderer. Detail names an item, a key or a limit, never a pixel or a path.
type Error struct {
	Reason Reason
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return "renderhost: " + string(e.Reason)
	}

	return "renderhost: " + string(e.Reason) + ": " + e.Detail
}

// ErrClosed is returned by a host that was closed or whose instance trapped: a trapped instance is discarded
// and its stored textures are gone.
var ErrClosed = errors.New("renderhost: the instance is closed")

// Config is what New needs; a zero value is the defaults.
type Config struct {
	MaxTextureBytes uint32
	MaxBitmapBytes  uint32
	// Uncancellable runs the instance without the check that stops a call when its context ends, which is
	// about five times faster. Text requests use it: they are bounded by construction (SDD-013 section 4.6).
	Uncancellable bool
}

// Host runs one instance of the core module. It is not safe for concurrent use.
type Host struct {
	mod  api.Module
	mem  api.Memory
	fns  map[string]api.Function
	dead error

	out struct{ ptr, len uint32 }
}

// New instantiates the embedded module, compiled once per process. The module is refused unless its
// digest matches, it has no import, and its vr_api_version is APIVersion.
func New(ctx context.Context, cfg Config) (*Host, error) {
	rt, cm, err := compileFor(ctx, !cfg.Uncancellable)
	if err != nil {
		return nil, err
	}

	mod, err := rt.InstantiateModule(ctx, cm, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return nil, fmt.Errorf("renderhost: instantiating the module: %w", err)
	}

	h := &Host{mod: mod, mem: mod.Memory(), fns: make(map[string]api.Function, len(exports))}
	for _, name := range exports {
		h.fns[name] = mod.ExportedFunction(name)
	}

	res, err := h.call(ctx, "vr_api_version")
	if err != nil {
		_ = mod.Close(ctx)

		return nil, err
	}

	if res[0] != APIVersion {
		_ = mod.Close(ctx)

		return nil, &Error{UnsupportedVersion, fmt.Sprintf("vr_api_version %d", res[0])}
	}

	maxTex, maxBM := cfg.MaxTextureBytes, cfg.MaxBitmapBytes
	if maxTex == 0 {
		maxTex = DefaultMaxTextureBytes
	}

	if maxBM == 0 {
		maxBM = DefaultMaxBitmapBytes
	}

	if err := h.status(ctx, "vr_init", uint64(maxTex), uint64(maxBM)); err != nil {
		_ = mod.Close(ctx)

		return nil, err
	}

	return h, nil
}

// Close frees the instance.
func (h *Host) Close(ctx context.Context) error {
	if h.dead == nil {
		h.dead = ErrClosed
	}

	return h.mod.Close(ctx)
}

// call runs an export. A failure of the call itself is a trap (or a cancel) and discards the instance:
// a trap on the same input would repeat (SDD-013 section 4.5).
func (h *Host) call(ctx context.Context, name string, args ...uint64) ([]uint64, error) {
	if h.dead != nil {
		return nil, h.dead
	}

	res, err := h.fns[name].Call(ctx, args...)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			h.dead = cerr
			_ = h.mod.Close(context.WithoutCancel(ctx))

			return nil, cerr
		}

		h.dead = &Error{RenderTrap, name + ": " + err.Error()}
		_ = h.mod.Close(context.WithoutCancel(ctx))

		return nil, h.dead
	}

	return res, nil
}

// status runs an export that returns a status and turns a failure into an *Error with its detail.
func (h *Host) status(ctx context.Context, name string, args ...uint64) error {
	res, err := h.call(ctx, name, args...)
	if err != nil {
		return err
	}

	st := int32(res[0]) //nolint:gosec // G115: the export returns an i32
	if st == 0 {
		return nil
	}

	reason, ok := statusReasons[st]
	if !ok {
		reason = RenderTrap
	}

	return &Error{reason, h.detail(ctx)}
}

// detail is the message of the last failed call (vr_error).
func (h *Host) detail(ctx context.Context) string {
	buf, err := h.alloc(ctx, errorBufBytes)
	if err != nil {
		return ""
	}

	defer h.free(ctx, buf, errorBufBytes)

	res, err := h.call(ctx, "vr_error", uint64(buf), errorBufBytes)
	if err != nil {
		return ""
	}

	n := min(uint32(res[0]), errorBufBytes) //nolint:gosec // G115: a u32 result

	b, ok := h.mem.Read(buf, n)
	if !ok {
		return ""
	}

	return string(b)
}

func (h *Host) alloc(ctx context.Context, n uint32) (uint32, error) {
	res, err := h.call(ctx, "vr_alloc", uint64(n))
	if err != nil {
		return 0, err
	}

	if res[0] == 0 {
		return 0, &Error{OutOfMemory, "vr_alloc"}
	}

	return uint32(res[0]), nil //nolint:gosec // G115: an offset into a 32-bit memory
}

func (h *Host) free(ctx context.Context, ptr, n uint32) {
	if h.dead == nil {
		_, _ = h.call(ctx, "vr_free", uint64(ptr), uint64(n))
	}
}

// block is a region of the module's memory that holds a copy of b, until freed.
type block struct{ ptr, n uint32 }

func (h *Host) put(ctx context.Context, b []byte) (block, error) {
	n := uint32(len(b)) //nolint:gosec // G115: callers bound b far below 4 GiB

	ptr, err := h.alloc(ctx, n)
	if err != nil {
		return block{}, err
	}

	if n > 0 && !h.mem.Write(ptr, b) {
		h.free(ctx, ptr, n)

		return block{}, &Error{OutOfMemory, "memory write"}
	}

	return block{ptr, n}, nil
}

func (h *Host) release(ctx context.Context, bs ...block) {
	for _, b := range bs {
		h.free(ctx, b.ptr, b.n)
	}
}

// PutTexture uploads the premultiplied RGBA pixels of a media layer under its item id, replacing the texture
// of that id. The module copies the pixels, so the caller reuses rgba at once.
func (h *Host) PutTexture(ctx context.Context, item string, w, ht int, rgba []byte, opaque bool) error {
	if len(item) > maxIDBytes || w < 0 || ht < 0 {
		return &Error{InvalidArgument, "PutTexture"}
	}

	if w > maxTextureSide || ht > maxTextureSide {
		return &Error{LimitExceeded, "render_texture_side"}
	}

	if len(rgba) < w*ht*4 {
		return &Error{InvalidArgument, "PutTexture: the pixels are shorter than width x height x 4"}
	}

	id, err := h.put(ctx, []byte(item))
	if err != nil {
		return err
	}

	px, err := h.put(ctx, rgba[:w*ht*4])
	if err != nil {
		h.release(ctx, id)

		return err
	}

	defer h.release(ctx, id, px)

	var op uint64
	if opaque {
		op = 1
	}

	return h.status(ctx, "vr_put_texture", uint64(id.ptr), uint64(id.n), uint64(w), uint64(ht), uint64(px.ptr), op)
}

// PutBitmap uploads a bitmap of the rasterizer under its key (plan.BitmapSpec.Key). The module premultiplies it.
func (h *Host) PutBitmap(ctx context.Context, key string, bm bitmap.Bitmap) error {
	if len(key) > maxIDBytes || bm.Width < 0 || bm.Height < 0 {
		return &Error{InvalidArgument, "PutBitmap"}
	}

	if bm.Width > maxTextureSide || bm.Height > maxTextureSide {
		return &Error{LimitExceeded, "render_texture_side"}
	}

	n := bm.Width * bm.Height * 4
	if len(bm.Pix) < n {
		return &Error{InvalidArgument, "PutBitmap: the pixels are shorter than width x height x 4"}
	}

	id, err := h.put(ctx, []byte(key))
	if err != nil {
		return err
	}

	px, err := h.put(ctx, bm.Pix[:n])
	if err != nil {
		h.release(ctx, id)

		return err
	}

	defer h.release(ctx, id, px)

	return h.status(ctx, "vr_put_bitmap",
		uint64(id.ptr), uint64(id.n), uint64(bm.Width), uint64(bm.Height),
		api.EncodeF64(bm.NaturalW), api.EncodeF64(bm.NaturalH), api.EncodeF64(bm.CenterDX), api.EncodeF64(bm.CenterDY),
		uint64(px.ptr))
}

// PutLUT uploads the lookup table of a lut asset under the asset id, replacing the table of that id (SDD-004
// section 3.6): the entries are the little-endian 16-bit values of the table (lut.Table.Bytes).
func (h *Host) PutLUT(ctx context.Context, asset string, t *lut.Table) error {
	if len(asset) > maxIDBytes || t == nil || t.Size < lut.MinSize || t.Size > lut.MaxSize ||
		len(t.Data) != 3*t.Size*t.Size*t.Size {
		return &Error{InvalidArgument, "PutLUT"}
	}

	id, err := h.put(ctx, []byte(asset))
	if err != nil {
		return err
	}

	data, err := h.put(ctx, t.Bytes())
	if err != nil {
		h.release(ctx, id)

		return err
	}

	defer h.release(ctx, id, data)

	return h.status(ctx, "vr_put_lut", uint64(id.ptr), uint64(id.n), uint64(t.Size), uint64(data.ptr), uint64(data.n))
}

// Release forgets the texture, bitmap or lookup table with that id or key; an unknown one is not an error.
func (h *Host) Release(ctx context.Context, id string) error {
	if len(id) > maxIDBytes {
		return &Error{InvalidArgument, "Release"}
	}

	b, err := h.put(ctx, []byte(id))
	if err != nil {
		return err
	}

	defer h.release(ctx, b)

	return h.status(ctx, "vr_release", uint64(b.ptr), uint64(b.n))
}

// Render draws the frame into dst: RGBA, no stride padding, exactly Output.Width by Output.Height. With a band
// (Output.Y1 > Output.Y0) only those rows of dst are written.
func (h *Host) Render(ctx context.Context, in RenderInput, dst *image.RGBA) error {
	w, ht := in.Output.Width, in.Output.Height
	if dst.Rect.Min != (image.Point{}) || dst.Rect.Dx() != w || dst.Rect.Dy() != ht || dst.Stride != 4*w {
		return &Error{InvalidArgument, "dst is not the output size"}
	}

	text, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("renderhost: encoding the render input: %w", err)
	}

	y0, y1 := 0, ht
	if in.Output.Y1 > in.Output.Y0 {
		y0, y1 = in.Output.Y0, in.Output.Y1
	}

	return h.renderText(ctx, text, dst, y0, y1)
}

// renderText runs vr_render on an encoded render input and copies rows [y0, y1) of the frame into dst.
func (h *Host) renderText(ctx context.Context, text []byte, dst *image.RGBA, y0, y1 int) error {
	w, ht := dst.Rect.Dx(), dst.Rect.Dy()

	if w > maxOutputSide || ht > maxOutputSide {
		return &Error{LimitExceeded, "render_frame_side"}
	}

	frame := uint32(w * ht * 4) //nolint:gosec // G115: the module refuses sides above 4096 before drawing
	if frame != h.out.len {
		if h.out.len != 0 {
			h.free(ctx, h.out.ptr, h.out.len)
			h.out.ptr, h.out.len = 0, 0
		}

		ptr, err := h.alloc(ctx, frame)
		if err != nil {
			return err
		}

		h.out.ptr, h.out.len = ptr, frame
	}

	input, err := h.put(ctx, text)
	if err != nil {
		return err
	}

	defer h.release(ctx, input)

	if err := h.status(
		ctx,
		"vr_render",
		uint64(input.ptr),
		uint64(input.n),
		uint64(h.out.ptr),
		uint64(h.out.len),
	); err != nil {
		return err
	}

	rows, ok := h.mem.Read(h.out.ptr+uint32(y0*w*4), uint32((y1-y0)*w*4)) //nolint:gosec // G115: inside the frame
	if !ok {
		return &Error{OutOfMemory, "reading the frame"}
	}

	copy(dst.Pix[y0*w*4:y1*w*4], rows)

	return nil
}
