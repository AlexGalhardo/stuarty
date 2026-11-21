package export

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

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
	// imageWorkMemory bounds the memory of a render: the layers being prepared together, estimated at eight times
	// the pixels of the largest layer (the picture, its looks and their intermediate copies); the prepared
	// pictures the render holds; and what the instances of its bands hold of them.
	imageWorkMemory = 2 << 30
	// imageTextureBytes is the textures one instance may hold. A prepared layer is larger than its source when a
	// look grew it, and a bitmap counts as a texture once prepared, so the budget of a plain instance would refuse
	// a document that a render drawn from the sources accepts. It stays under the 1 GiB of the module, which also
	// holds the copy of the block being stored.
	imageTextureBytes = 800 << 20
	// maxPlanLayers is the layers of one plan the module draws (`render_layers`, SDD-013 section 7.2).
	maxPlanLayers = 64
	// minBandRows is the fewest rows worth an instance of their own.
	minBandRows = 64
	// rowMargin is the rows added on each side of the rows a layer is computed to reach: the module rounds its
	// own rectangle with its own sine and cosine.
	rowMargin = 2
)

// imagePool keeps the instances that prepare and draw an image document for the next render (SDD-021 section
// 6.2): up to maxImageWorkers of them, a gigabyte of their memory, for half a minute.
var imagePool = renderhost.NewPool(
	renderhost.Config{Uncancellable: true, MaxTextureBytes: imageTextureBytes},
	maxImageWorkers,
	1<<30,
	30*time.Second,
)

// imageSlots bounds the instances at work in the process, whatever the number of renders: a frame given up by a
// fast edit keeps working until its layer is done, and without a bound each edit would add its workers to the
// ones it replaced. A slot is held for one layer or one band, so a holder never waits for another slot.
var imageSlots = make(chan struct{}, maxImageWorkers)

// acquireSlot waits for a slot, or for the end of the context of the render.
func (r *imageRun) acquireSlot() (func(), error) {
	select {
	case imageSlots <- struct{}{}:
		return func() { <-imageSlots }, nil
	case <-r.ctx.Done():
		return nil, r.ctx.Err()
	}
}

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

	limit := r.preparedLimit
	if limit <= 0 {
		limit = imageWorkMemory
	}

	// The costliest layers first, so that the last one a worker takes is a small one and not a photo with a
	// sharpen that every other worker waits for.
	order := make([]int, len(layers))
	for i := range order {
		order[i] = i
	}

	slices.SortStableFunc(
		order,
		func(a, b int) int { return cmp.Compare(prepareCost(&layers[b]), prepareCost(&layers[a])) },
	)

	// The prepared pictures stay in memory until the draw, so what they add up to is bounded here: a document has
	// up to 500 layers and a picture can be hundreds of megabytes.
	var held atomic.Int64

	err := fanOut(r.ctx, workers, len(layers), func(n int) error {
		i := order[n]

		d, err := r.prepareOne(&layers[i])
		if err != nil {
			return err
		}

		if held.Add(int64(d.pic.Bytes())) > limit {
			return rendererError(&renderhost.Error{Reason: renderhost.LimitExceeded, Detail: "render_texture_bytes"})
		}

		out[i] = d

		return nil
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

// prepareCost is a measure of how long a layer takes to prepare, for ordering only: its pixels, once for the
// picture and once for each look that walks them.
func prepareCost(l *plan.Layer) int64 {
	px := int64(math.Max(l.W, 1) * math.Max(l.H, 1))
	if l.Kind == plan.Media {
		px = max(px, int64(math.Max(l.Crop.W, 1)*math.Max(l.Crop.H, 1)))
	}

	looks := int64(1 + len(l.Filters) + len(l.Effects))
	if l.Tone != nil || l.ColorMatrix != nil || l.Blur > 0 || l.Shadow != nil || l.Outline != nil {
		looks++
	}

	return px * looks
}

// prepareOne makes the prepared picture of one layer on an instance of the pool: the pixels of the layer,
// uploaded, and the preparation of the module. The instance goes back cleared, so a layer never sees what
// another one stored.
func (r *imageRun) prepareOne(l *plan.Layer) (drawLayer, error) {
	// The instance is not stopped by the context of the render, so that what it finishes is kept.
	ctx := context.WithoutCancel(r.ctx)

	key, keyed := [sha256.Size]byte{}, false
	if r.cache != nil {
		key, keyed = preparedKey(r.plan, l)
	}

	if keyed {
		if pic, ok := r.cache.getPrepared(r.cfg.CacheScope, key); ok {
			return r.layerFor(l, pic), nil
		}
	}

	release, err := r.acquireSlot()
	if err != nil {
		return drawLayer{}, err
	}

	defer release()

	// While this waited for a slot, the frame this one replaced may have finished the same layer.
	if keyed {
		if pic, ok := r.cache.peekPrepared(r.cfg.CacheScope, key); ok {
			return r.layerFor(l, pic), nil
		}
	}

	host, err := imagePool.Get(ctx)
	if err != nil {
		return drawLayer{}, rendererError(err)
	}

	defer imagePool.Put(host)

	switch l.Kind {
	case plan.Media:
		img, err := imageTexture(l, r.cfg)
		if err != nil {
			return drawLayer{}, fmt.Errorf("layer %s: %w", l.ItemID, err)
		}

		if err := host.PutTexture(ctx, l.ItemID, img.W, img.H, img.Pix, img.Opaque); err != nil {
			return drawLayer{}, rendererError(err)
		}
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
	default:
		return drawLayer{}, fmt.Errorf("layer %s: %w: an image has no group", l.ItemID, compositor.ErrSource)
	}

	// A table is in the module before the layer that names it is prepared: a layer is never drawn without it.
	if err := r.putLUT(ctx, host, l); err != nil {
		return drawLayer{}, err
	}

	pic, err := host.Prepare(ctx, preparationInput(r.plan, l))
	if err != nil {
		return drawLayer{}, rendererError(err)
	}

	if keyed {
		r.cache.putPrepared(r.cfg.CacheScope, key, pic)
	}

	return r.layerFor(l, pic), nil
}

// layerFor is the layer to draw from the prepared picture of l; a picture that draws nothing makes an empty one.
func (r *imageRun) layerFor(l *plan.Layer, pic renderhost.Prepared) drawLayer {
	if pic.Empty() {
		return drawLayer{}
	}

	d := drawLayer{layer: preparedLayer(l), pic: pic}
	d.y0, d.y1 = rowsOf(&d.layer, pic.Size(), r.plan.Height)

	return d
}

// preparedKey is the key of the prepared picture of l in an ImageCache: a digest of the canvas size and of the
// plan layer without what the preparation does not read, that is its opacity, blend mode, mask, clip and
// position (SDD-021 section 6.2). A centre that is not a multiple of half a pixel stays in the key, because the
// rectangle that draws the picture is then not the same sum.
func preparedKey(fp plan.FramePlan, l *plan.Layer) ([sha256.Size]byte, bool) {
	one := *l
	one.Opacity, one.Blend, one.Mask, one.Clip, one.Stencil = 0, "", nil, nil, false

	if isHalfPixel(l.CX) && isHalfPixel(l.CY) {
		one.CX, one.CY = 0, 0
	}

	body, err := json.Marshal(&one)
	if err != nil {
		return [sha256.Size]byte{}, false
	}

	h := sha256.New()
	_, _ = fmt.Fprintf(h, "stuarty prepared layer v1 %d %d %d ", fp.Width, fp.Height, fp.Frame)
	_, _ = h.Write(body)

	var sum [sha256.Size]byte

	h.Sum(sum[:0])

	return sum, true
}

func isHalfPixel(v float64) bool { return v*2 == math.Round(v*2) }

// drawBands draws the prepared layers over base, or over the background when base is nil, in bands of rows on
// instances of their own, and joins them. Only the rows the layers reach are drawn when there is a base; the
// rest of the result is the base.
func (r *imageRun) drawBands(base *image.RGBA, drawn []drawLayer) (*image.RGBA, error) {
	fp := r.plan
	lo, hi := 0, fp.Height

	// The limit of the module is on the draw, not on a band: counted per band, whether a document can be drawn
	// would depend on how many CPUs cut it.
	count := len(drawn)
	if base != nil {
		count++
	}

	if count > maxPlanLayers {
		return nil, rendererError(&renderhost.Error{Reason: renderhost.LimitExceeded, Detail: "render_layers"})
	}

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

	bands := r.bands
	if bands <= 0 {
		want := min(maxImageWorkers, runtime.GOMAXPROCS(0), max((hi-lo)/minBandRows, 1))
		bands = bandCount(drawn, lo, hi, want, imageWorkMemory)
	}

	// A mask taken from another layer needs that layer in the same draw: one band holds them all, the source
	// included when it reaches no row (a source off the canvas still masks, and an inverted mask then shows the
	// whole layer).
	clipped := slices.ContainsFunc(drawn, func(d drawLayer) bool { return d.layer.Clip != nil || d.layer.Stencil })
	if clipped {
		bands = 1
	}

	step := (hi - lo + bands - 1) / bands

	err := fanOut(r.ctx, bands, bands, func(b int) error {
		y0, y1 := lo+b*step, min(lo+(b+1)*step, hi)
		if y0 >= y1 {
			return nil
		}

		return r.drawBand(base, drawn, dst, y0, y1, clipped)
	})
	if err != nil {
		return nil, err
	}

	return dst, nil
}

// bandCount is the most bands, up to want, whose instances together hold at most budget bytes of prepared
// pictures: each instance holds every picture that reaches its rows, so a stack of large layers would otherwise
// be held once per band.
func bandCount(drawn []drawLayer, lo, hi, want int, budget int64) int {
	for n := want; n > 1; n-- {
		if bandsHold(drawn, lo, hi, n) <= budget {
			return n
		}
	}

	return 1
}

// bandsHold is the bytes of prepared pictures that n bands of the rows [lo, hi) hold together.
func bandsHold(drawn []drawLayer, lo, hi, n int) int64 {
	step := (hi - lo + n - 1) / n

	var held int64

	for b := range n {
		y0, y1 := lo+b*step, min(lo+(b+1)*step, hi)

		for i := range drawn {
			if drawn[i].y0 < y1 && drawn[i].y1 > y0 {
				held += int64(drawn[i].pic.Bytes())
			}
		}
	}

	return held
}

// drawBand draws the rows [y0, y1) of dst on an instance of its own: the rows of base, when there is one, as a
// texture of that band alone, and the prepared pictures of the layers that reach the band, or of all of them
// when keepAll says that a layer may need another one as its mask.
func (r *imageRun) drawBand(base *image.RGBA, drawn []drawLayer, dst *image.RGBA, y0, y1 int, keepAll bool) error {
	ctx := context.WithoutCancel(r.ctx)
	fp := r.plan

	release, err := r.acquireSlot()
	if err != nil {
		return err
	}

	defer release()

	host, err := imagePool.Get(ctx)
	if err != nil {
		return rendererError(err)
	}

	defer imagePool.Put(host)

	kept := make([]plan.Layer, 0, len(drawn)+1)

	if base != nil {
		rows := y1 - y0
		stride := base.Stride

		if err := host.PutTexture(
			ctx,
			prefixItem,
			fp.Width,
			rows,
			base.Pix[y0*stride:y1*stride],
			!fp.Transparent,
		); err != nil {
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
		if !keepAll && (d.y1 <= y0 || d.y0 >= y1) {
			continue
		}

		if err := host.PutPrepared(ctx, d.layer.ItemID, d.pic); err != nil {
			return rendererError(err)
		}

		kept = append(kept, d.layer)
	}

	in := fp
	in.Layers = withoutLostClips(kept)

	// The instance draws into a view of the rows of its band, so it holds those rows and not the frame.
	view, ok := dst.SubImage(image.Rect(0, y0, fp.Width, y1)).(*image.RGBA)
	if !ok {
		return rendererError(&renderhost.Error{Reason: renderhost.InvalidArgument, Detail: "the band is not a view"})
	}

	out := renderhost.Output{Width: fp.Width, Height: fp.Height, Scale: 1, Y0: y0, Y1: y1}
	if err := host.Render(ctx, renderhost.RenderInput{Output: out, Plan: in}, view); err != nil {
		return rendererError(err)
	}

	return nil
}
