package export

import (
	"context"
	"fmt"
	"image"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/compositor"
	"github.com/AlexGalhardo/stuarty/engine/renderhost"
)

// The render of an image document is two stages (SDD-021 section 6.2): every layer is prepared once, on a
// bounded set of workers that each own an instance of the module, and the canvas is then drawn in bands, one
// instance per band, from the prepared pictures that reach it.
const (
	// maxImageWorkers bounds the instances that prepare or draw at once.
	maxImageWorkers = 8
	// imageWorkMemory is the memory the layers being prepared together may take, estimated at eight times the
	// pixels of the largest layer (the picture, its looks and their intermediate copies).
	imageWorkMemory = 2 << 30
	// minBandRows is the fewest rows worth an instance of their own.
	minBandRows = 64
	// rowMargin is the rows added on each side of the rows a layer is computed to reach: the module rounds its
	// own rectangle with its own sine and cosine.
	rowMargin = 2
)

// drawLayer is a layer ready for the draw: the plan layer as a media layer of its prepared picture, the
// picture, and the rows of the canvas that its draw can touch.
type drawLayer struct {
	layer  plan.Layer
	pic    renderhost.Prepared
	y0, y1 int
}

// preparedLayer is the plan layer to draw from the prepared picture of l: a media layer of the same item with
// what the draw reads (position, size, rotation, opacity, blend mode, mask, clip, wipe) and none of the looks,
// which the picture holds (SDD-013 section 4.2.2).
func preparedLayer(l *plan.Layer) plan.Layer {
	return plan.Layer{
		ItemID: l.ItemID, Kind: plan.Media, AssetID: l.ItemID, AssetKind: "image",
		Blend: l.Blend, Mask: l.Mask, Clip: l.Clip, Stencil: l.Stencil, Transition: l.Transition,
		CX: l.CX, CY: l.CY, W: l.W, H: l.H, Rotation: l.Rotation, Scale: l.Scale, Opacity: l.Opacity,
	}
}

// preparationInput is the render input that prepares l: the layer alone, at full opacity, with no clip.
func preparationInput(fp plan.FramePlan, l *plan.Layer) renderhost.RenderInput {
	one := *l
	one.Opacity, one.Clip, one.Stencil, one.Blend, one.Mask = 1, nil, false, "", nil

	return renderhost.RenderInput{
		Output: renderhost.Output{Width: fp.Width, Height: fp.Height, Scale: 1},
		Plan: plan.FramePlan{
			Frame: fp.Frame, Width: fp.Width, Height: fp.Height, Background: fp.Background,
			Transparent: fp.Transparent, Layers: []plan.Layer{one},
		},
	}
}

// rowsOf is the rows [y0, y1) of a canvas of the given height that a prepared layer placed as l can touch: the
// rectangle that draws it, turned by its rotation, with a margin.
func rowsOf(l *plan.Layer, sz renderhost.PreparedSize, height int) (int, int) {
	sin, cos := math.Sincos(l.Rotation * math.Pi / 180)
	cy := l.CY - sz.OffsetY
	ey := sz.GrownW/2*math.Abs(sin) + sz.GrownH/2*math.Abs(cos)

	y0 := int(math.Floor(cy-ey)) - rowMargin
	y1 := int(math.Ceil(cy+ey)) + rowMargin

	return min(max(y0, 0), height), min(max(y1, 0), height)
}

// workerCount is how many instances may work at once: the CPUs, never more than maxImageWorkers, and fewer
// when the layers are so large that their working sets would pass imageWorkMemory together.
func workerCount(layers []plan.Layer) int {
	var largest int64

	for i := range layers {
		l := &layers[i]
		px := int64(math.Min(math.Max(l.W, 1)*math.Max(l.H, 1), 1<<30))

		if l.Kind == plan.Media {
			px = max(px, int64(math.Min(math.Max(l.Crop.W, 1)*math.Max(l.Crop.H, 1), 1<<30)))
		}

		largest = max(largest, px*4*8)
	}

	n := min(runtime.GOMAXPROCS(0), maxImageWorkers)
	if largest > 0 {
		n = min(n, int(max(imageWorkMemory/largest, 1)))
	}

	return max(n, 1)
}

// fanOut runs work(i) for i in [0, count) on at most workers goroutines, which stop taking new indices at the
// first error or when ctx ends. It returns the first error once the work in flight is over, or the error of ctx
// as soon as it ends: the work in flight then finishes on its own (an instance of the module cannot be stopped)
// and nobody waits for it.
func fanOut(ctx context.Context, workers, count int, work func(i int) error) error {
	var (
		next   atomic.Int64
		wg     sync.WaitGroup
		once   sync.Once
		first  error
		failed atomic.Bool
	)

	for range min(workers, count) {
		wg.Go(func() {
			for !failed.Load() && ctx.Err() == nil {
				i := int(next.Add(1)) - 1
				if i >= count {
					return
				}

				if err := work(i); err != nil {
					once.Do(func() { first = err })
					failed.Store(true)

					return
				}
			}
		})
	}

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	if first != nil {
		return first
	}

	return ctx.Err()
}

// stage draws the layers over base, the picture of the layers below them, or over the background when base is
// nil, and gives the pixels. The layers are prepared once and drawn in bands; a picture drawn first and the
// layers over it are the same pixels as the layers drawn in one pass (SDD-021 sections 6.1 and 6.2).
func (r *imageRun) stage(base *image.RGBA, layers []plan.Layer) (*image.RGBA, error) {
	for i := range layers {
		if layers[i].Kind == plan.Group {
			// An image document has layers of its own kinds; a group is the plan of nested timelines (SDD-004 §3.16).
			return nil, fmt.Errorf("layer %s: %w: an image has no group", layers[i].ItemID, compositor.ErrSource)
		}
	}

	drawn, err := r.prepareAll(layers)
	if err != nil {
		return nil, err
	}

	return r.drawBands(base, drawn)
}

// prepareAll prepares every layer, in parallel, and gives those that draw something, in plan order.
func (r *imageRun) prepareAll(layers []plan.Layer) ([]drawLayer, error) {
	out := make([]drawLayer, len(layers))
	workers := workerCount(layers)

	// Each goroutine of fanOut takes the next layer; the instance a worker owns is made for the first one it
	// takes and closed when the stage ends.
	hosts := make([]*prepWorker, workers)
	pool := make(chan *prepWorker, workers)

	for i := range hosts {
		hosts[i] = &prepWorker{}
		pool <- hosts[i]
	}

	defer func() {
		// The instances are closed once no worker holds one: those of an abandoned stage are closed by the
		// goroutine that returns them last.
		go func() {
			for range workers {
				w := <-pool
				w.close()
			}
		}()
	}()

	err := fanOut(r.ctx, workers, len(layers), func(i int) error {
		w := <-pool
		defer func() { pool <- w }()

		d, err := r.prepareOne(w, &layers[i])
		out[i] = d

		return err
	})
	if err != nil {
		return nil, err
	}

	kept := out[:0]

	for i := range out {
		if !out[i].pic.Empty() {
			kept = append(kept, out[i])
		}
	}

	return kept, nil
}

// prepWorker is an instance of the module that prepares layers, and the tables it holds.
type prepWorker struct {
	host   *renderhost.Host
	tables map[string]bool
}

func (w *prepWorker) close() {
	if w.host != nil {
		_ = w.host.Close(context.Background())
		w.host = nil
	}
}

// prepareOne makes the prepared picture of one layer on the instance of w: the pixels of the layer, uploaded
// and released, and the preparation of the module.
func (r *imageRun) prepareOne(w *prepWorker, l *plan.Layer) (drawLayer, error) {
	// The instance is not stopped by the context of the render, so that what it finishes is kept.
	ctx := context.WithoutCancel(r.ctx)

	if w.host == nil {
		h, err := renderhost.New(ctx, renderhost.Config{Uncancellable: true})
		if err != nil {
			return drawLayer{}, rendererError(err)
		}

		w.host, w.tables = h, map[string]bool{}
	}

	host := w.host

	var release string

	switch l.Kind {
	case plan.Media:
		img, err := imageTexture(l, r.cfg)
		if err != nil {
			return drawLayer{}, fmt.Errorf("layer %s: %w", l.ItemID, err)
		}

		if err := host.PutTexture(ctx, l.ItemID, img.W, img.H, img.Pix, img.Opaque); err != nil {
			return drawLayer{}, rendererError(err)
		}

		release = l.ItemID
	case plan.Bitmap:
		bm, err := r.raster.Get(*l.Bitmap)
		if err != nil {
			return drawLayer{}, fmt.Errorf("layer %s: %w", l.ItemID, err)
		}

		if bm.Width <= 0 || bm.Height <= 0 {
			return drawLayer{}, nil
		}

		if err := host.PutBitmap(ctx, l.Bitmap.Key(), bm); err != nil {
			return drawLayer{}, rendererError(err)
		}

		release = l.Bitmap.Key()
	default:
		return drawLayer{}, fmt.Errorf("layer %s: %w: an image has no group", l.ItemID, compositor.ErrSource)
	}

	// A table is in the module before the layer that names it is prepared: a layer is never drawn without it.
	if err := putImageLUT(ctx, host, l, w.tables, r.cfg); err != nil {
		return drawLayer{}, err
	}

	pic, err := host.Prepare(ctx, preparationInput(r.plan, l))
	if err != nil {
		return drawLayer{}, rendererError(err)
	}

	// The instance goes on to the next layer: what this one uploaded is not needed again.
	if err := host.Release(ctx, release); err != nil {
		return drawLayer{}, rendererError(err)
	}

	if pic.Empty() {
		return drawLayer{}, nil
	}

	d := drawLayer{layer: preparedLayer(l), pic: pic}
	d.y0, d.y1 = rowsOf(&d.layer, pic.Size(), r.plan.Height)

	return d, nil
}

// drawBands draws the prepared layers over base, or over the background when base is nil, in bands of rows on
// instances of their own, and joins them. Only the rows the layers reach are drawn when there is a base; the
// rest of the result is the base.
func (r *imageRun) drawBands(base *image.RGBA, drawn []drawLayer) (*image.RGBA, error) {
	fp := r.plan
	lo, hi := 0, fp.Height

	if base != nil {
		lo, hi = fp.Height, 0

		for i := range drawn {
			lo, hi = min(lo, drawn[i].y0), max(hi, drawn[i].y1)
		}

		if lo >= hi {
			return cloneRGBA(base), nil
		}
	}

	var dst *image.RGBA
	if base != nil {
		dst = cloneRGBA(base)
	} else {
		dst = image.NewRGBA(image.Rect(0, 0, fp.Width, fp.Height))
	}

	// A mask taken from another layer needs that layer in the same draw: one band holds them all.
	bands := r.bands
	if bands <= 0 {
		bands = min(maxImageWorkers, runtime.GOMAXPROCS(0), max((hi-lo)/minBandRows, 1))
	}

	if slices.ContainsFunc(drawn, func(d drawLayer) bool { return d.layer.Clip != nil || d.layer.Stencil }) {
		bands = 1
	}

	step := (hi - lo + bands - 1) / bands

	err := fanOut(r.ctx, bands, bands, func(b int) error {
		y0, y1 := lo+b*step, min(lo+(b+1)*step, hi)
		if y0 >= y1 {
			return nil
		}

		return r.drawBand(base, drawn, dst, y0, y1)
	})
	if err != nil {
		return nil, err
	}

	return dst, nil
}

// drawBand draws the rows [y0, y1) of dst on an instance of its own: the rows of base, when there is one, as a
// texture of that band alone, and the prepared pictures of the layers that reach the band.
func (r *imageRun) drawBand(base *image.RGBA, drawn []drawLayer, dst *image.RGBA, y0, y1 int) error {
	ctx := context.WithoutCancel(r.ctx)
	fp := r.plan

	host, err := renderhost.New(ctx, renderhost.Config{Uncancellable: true})
	if err != nil {
		return rendererError(err)
	}

	defer func() { _ = host.Close(ctx) }()

	kept := make([]plan.Layer, 0, len(drawn)+1)

	if base != nil {
		rows := y1 - y0
		stride := base.Stride

		if err := host.PutTexture(ctx, prefixItem, fp.Width, rows, base.Pix[y0*stride:y1*stride], !fp.Transparent); err != nil {
			return rendererError(err)
		}

		kept = append(kept, plan.Layer{
			ItemID: prefixItem, Kind: plan.Media, AssetID: prefixItem, AssetKind: "image",
			Crop: plan.Rect{W: float64(fp.Width), H: float64(rows)}, CX: float64(fp.Width) / 2,
			CY: float64(y0) + float64(rows)/2, W: float64(fp.Width), H: float64(rows), Scale: 1, Opacity: 1,
		})
	}

	for i := range drawn {
		d := &drawn[i]
		if d.y1 <= y0 || d.y0 >= y1 {
			continue
		}

		if err := host.PutPrepared(ctx, d.layer.ItemID, d.pic); err != nil {
			return rendererError(err)
		}

		kept = append(kept, d.layer)
	}

	in := fp
	in.Layers = withoutLostClips(kept)

	out := renderhost.Output{Width: fp.Width, Height: fp.Height, Scale: 1, Y0: y0, Y1: y1}
	if err := host.Render(ctx, renderhost.RenderInput{Output: out, Plan: in}, dst); err != nil {
		return rendererError(err)
	}

	return nil
}
