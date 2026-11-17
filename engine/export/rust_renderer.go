package export

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"sync"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/engine/bitmap"
	"github.com/AlexGalhardo/vukem-video/engine/compositor"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/gpuhost"
	"github.com/AlexGalhardo/vukem-video/engine/renderhost"
)

// PixelRenderer names what places the pixels of an exported frame (SDD-013 section 4.6, ADR-0029 item 8).
type PixelRenderer string

// The two renderers. The Go compositor stays the default until the byte-for-byte gate of stage C passes on
// Linux, macOS and Windows; the Rust renderer is the same frames drawn by the WebAssembly module.
const (
	PixelRendererGo   PixelRenderer = "go"
	PixelRendererRust PixelRenderer = "rust"
	// PixelRendererGPU draws on the GPU in the child process vukem-render when there is an adapter and a
	// GPU host, and as PixelRendererRust otherwise. The GPU frames are held to the CPU path by SSIM, not byte
	// for byte (SDD-013 section 3.8).
	PixelRendererGPU PixelRenderer = "gpu"
)

// ParsePixelRenderer reads VUKEM_VIDEO_EXPORT_RENDERER: empty is the default, the Go compositor.
func ParsePixelRenderer(s string) (PixelRenderer, error) {
	switch s {
	case "", string(PixelRendererGo):
		return PixelRendererGo, nil
	case string(PixelRendererRust):
		return PixelRendererRust, nil
	case string(PixelRendererGPU):
		return PixelRendererGPU, nil
	}

	return "", fmt.Errorf("export: the renderer %q is not go, rust or gpu", s)
}

// rustBandMinPixels is the frame size under which one instance draws the whole frame: splitting a small frame
// costs more in uploads than it saves. It never changes a pixel.
const rustBandMinPixels = 1 << 14

// rustRenderer draws frames with the core module: one instance per band, each holding the textures and bitmaps
// of the frame and drawing its own rows (SDD-013 section 4.6). The decoders and the bitmap cache are those of
// the Go path, so the module receives the very pixels the Go compositor would.
type rustRenderer struct {
	s       *state
	fs      *frameSource
	bitmaps func(plan.BitmapSpec) (bitmap.Bitmap, error)
	bands   int
	hosts   []*renderhost.Host

	// gpu is the child process of the GPU path, started at the first frame; gpuTried is set once the start
	// was attempted, so a machine without an adapter is asked once.
	gpu      *gpuhost.Host
	gpuTried bool
}

func newRustRenderer(
	s *state,
	fs *frameSource,
	bitmaps func(plan.BitmapSpec) (bitmap.Bitmap, error),
	bands int,
) *rustRenderer {
	if bands <= 0 {
		bands = compositor.DefaultBands()
	}

	return &rustRenderer{s: s, fs: fs, bitmaps: bitmaps, bands: min(bands, compositor.MaxBands)}
}

func (r *rustRenderer) close() {
	if r.gpu != nil {
		_ = r.gpu.Close()
		r.gpu = nil
	}

	for _, h := range r.hosts {
		_ = h.Close(context.WithoutCancel(r.s.ctx))
	}

	r.hosts = nil
}

// opacity is the byte the compositor draws a layer with; a layer at 0 is skipped before its pixels are fetched.
func opacity(l *plan.Layer) int {
	return int(math.Round(min(max(l.Opacity, 0), 1) * 255))
}

type textureUpload struct {
	item   string
	img    compositor.Image
	opaque bool
}

type bitmapUpload struct {
	key string
	bm  bitmap.Bitmap
}

// compose draws the frame plan into dst, scaling every position and size by scale, as Compositor.Compose does.
func (r *rustRenderer) compose(dst *image.RGBA, fp plan.FramePlan, scale float64) error {
	var (
		textures []textureUpload
		bitmaps  []bitmapUpload
		kept     = make([]plan.Layer, 0, len(fp.Layers))
	)

	for i := range fp.Layers {
		l := fp.Layers[i]
		if opacity(&l) == 0 {
			continue
		}

		switch l.Kind {
		case plan.Media:
			img, err := r.fs.media(&l)
			if err != nil {
				return fmt.Errorf("layer %s: %w", l.ItemID, err)
			}

			if img.W <= 0 || img.H <= 0 || len(img.Pix) < img.W*img.H*4 {
				continue
			}

			textures = append(textures, textureUpload{l.ItemID, img, img.Opaque})
		case plan.Bitmap:
			if l.Bitmap == nil {
				return fmt.Errorf("layer %s: %w: no bitmap source", l.ItemID, compositor.ErrSource)
			}

			bm, err := r.bitmaps(*l.Bitmap)
			if err != nil {
				return fmt.Errorf("layer %s: %w", l.ItemID, err)
			}

			if bm.Width <= 0 || bm.Height <= 0 || len(bm.Pix) < bm.Width*bm.Height*4 {
				continue
			}

			bitmaps = append(bitmaps, bitmapUpload{l.Bitmap.Key(), bm})
		default:
			return fmt.Errorf("layer %s: %w: kind %q", l.ItemID, compositor.ErrSource, l.Kind)
		}

		kept = append(kept, l)
	}

	fp.Layers = kept

	luts, err := r.frameLUTs(kept)
	if err != nil {
		return err
	}

	if g := r.gpuHost(); g != nil {
		return rendererError(r.gpuFrame(g, dst, fp, scale, textures, bitmaps, luts))
	}

	w, h := dst.Rect.Dx(), dst.Rect.Dy()

	n := r.bands
	if w*h < rustBandMinPixels {
		n = 1
	}

	n = max(1, min(n, h))

	if err := r.ensureHosts(n); err != nil {
		return err
	}

	errs := make([]error, n)

	var wg sync.WaitGroup

	for i := range n {
		wg.Go(func() {
			errs[i] = r.band(r.hosts[i], dst, fp, scale, textures, bitmaps, luts, h*i/n, h*(i+1)/n)
		})
	}

	wg.Wait()

	return rendererError(errors.Join(errs...))
}

// gpuHost is the child process of the GPU path, or nil when the export draws on the CPU path: the job did not ask
// for the GPU, has no GPU host, the runner is sandboxed, or the child could not start (no adapter, a missing
// binary). The choice is made once, before the first frame, so the frames of one export are never a mix of both.
func (r *rustRenderer) gpuHost() *gpuhost.Host {
	job := r.s.job
	if r.gpuTried || job.PixelRenderer != PixelRendererGPU || job.GPUHost == "" || r.s.single {
		return r.gpu
	}

	r.gpuTried = true

	g, err := gpuhost.Start(r.s.ctx, job.Runner, gpuhost.Config{
		Path: job.GPUHost, Dir: r.s.dir, Deadline: r.s.deadline,
	})
	if err == nil {
		r.gpu = g
	}

	return r.gpu
}

// gpuFrame draws the whole frame in the child and releases what it uploaded.
func (r *rustRenderer) gpuFrame(
	g *gpuhost.Host, dst *image.RGBA, fp plan.FramePlan, scale float64,
	textures []textureUpload, bitmaps []bitmapUpload, luts []lutUpload,
) error {
	var ids []string

	defer func() {
		for _, id := range ids {
			_ = g.Release(id)
		}
	}()

	for i := range textures {
		t := &textures[i]
		ids = append(ids, t.item)

		if err := g.PutTexture(t.item, t.img.W, t.img.H, t.img.Pix, t.opaque); err != nil {
			return err
		}
	}

	for i := range bitmaps {
		ids = append(ids, bitmaps[i].key)

		if err := g.PutBitmap(bitmaps[i].key, bitmaps[i].bm); err != nil {
			return err
		}
	}

	for i := range luts {
		ids = append(ids, luts[i].asset)

		if err := g.PutLUT(luts[i].asset, luts[i].table); err != nil {
			return err
		}
	}

	out := renderhost.Output{Width: dst.Rect.Dx(), Height: dst.Rect.Dy(), Scale: scale}

	return g.Render(renderhost.RenderInput{Output: out, Plan: fp}, dst)
}

func (r *rustRenderer) ensureHosts(n int) error {
	for len(r.hosts) < n {
		h, err := renderhost.New(r.s.ctx, renderhost.Config{})
		if err != nil {
			return rendererError(err)
		}

		r.hosts = append(r.hosts, h)
	}

	return nil
}

// band draws rows [y0, y1) of the frame on one instance and releases what it uploaded.
func (r *rustRenderer) band(
	h *renderhost.Host, dst *image.RGBA, fp plan.FramePlan, scale float64,
	textures []textureUpload, bitmaps []bitmapUpload, luts []lutUpload, y0, y1 int,
) error {
	ctx := r.s.ctx

	var ids []string

	defer func() {
		for _, id := range ids {
			_ = h.Release(context.WithoutCancel(ctx), id)
		}
	}()

	for i := range textures {
		t := &textures[i]
		ids = append(ids, t.item)

		if err := h.PutTexture(ctx, t.item, t.img.W, t.img.H, t.img.Pix, t.opaque); err != nil {
			return err
		}
	}

	for i := range bitmaps {
		ids = append(ids, bitmaps[i].key)

		if err := h.PutBitmap(ctx, bitmaps[i].key, bitmaps[i].bm); err != nil {
			return err
		}
	}

	for i := range luts {
		ids = append(ids, luts[i].asset)

		if err := h.PutLUT(ctx, luts[i].asset, luts[i].table); err != nil {
			return err
		}
	}

	out := renderhost.Output{Width: dst.Rect.Dx(), Height: dst.Rect.Dy(), Scale: scale, Y0: y0, Y1: y1}

	return h.Render(ctx, renderhost.RenderInput{Output: out, Plan: fp}, dst)
}

// rendererError turns a failure of the renderer into the error of the export (SDD-013 section 7.3):
// E_LIMIT_EXCEEDED for a limit, E_EXPORT_FAILED with reason render_failed and the reason of the renderer for
// the rest. A cancelled context passes through, a trap discards the instance and fails the export once.
func rendererError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	re, ok := errors.AsType[*renderhost.Error](err)
	if !ok {
		return exportErr(ReasonRender, "A frame of the project could not be drawn.", err, "")
	}

	if re.Reason == renderhost.LimitExceeded {
		return &ffmpeg.Error{
			Code:    ffmpeg.CodeLimitExceeded,
			Reason:  "limit_exceeded",
			Message: "The project is too large to draw at once.",
			Hint:    "Use smaller clips, or fewer large layers at the same time.",
			Details: map[string]any{"limit": re.Detail},
			Err:     err,
		}
	}

	e := exportErr(ReasonRender, "A frame of the project could not be drawn.", err, "")
	e.Details["renderer_reason"] = string(re.Reason)

	if re.Reason == renderhost.MissingTexture {
		e.Details["item"] = re.Detail
	}

	return e
}
