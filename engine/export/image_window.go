package export

import (
	"context"
	"crypto/sha256"
	"image"
	"runtime"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/renderhost"
)

// The window of an edit (SDD-021 section 6.3). A person edits some layers of a document and the picture they
// see again is the picture of the render before, except where those layers were and where they are now: a
// pixel is drawn from the layers that reach it, in plan order, from its own coordinates, so a pixel no edited
// layer touched, before or after, has the value it had. The render before is kept by the cache; the window is
// drawn again from the layers that reach it, whatever their place in the stack, and is copied over it.

// layerBounds is the rectangle of the canvas that the draw of one layer can touch: its rows and columns, with
// a margin (rowsOf, colsOf). known is false when the render that made it did not draw the layer; empty, that
// the layer draws nothing.
type layerBounds struct {
	known, empty   bool
	x0, y0, x1, y1 int
}

func boundsOfDraw(d drawLayer) layerBounds {
	if d.pic.Empty() || d.x0 >= d.x1 || d.y0 >= d.y1 {
		return layerBounds{known: true, empty: true}
	}

	return layerBounds{known: true, x0: d.x0, y0: d.y0, x1: d.x1, y1: d.y1}
}

func (b layerBounds) reaches(w layerBounds) bool {
	return !b.empty && !w.empty && b.x0 < w.x1 && w.x0 < b.x1 && b.y0 < w.y1 && w.y0 < b.y1
}

// union is the smallest rectangle that holds both; an empty one adds nothing.
func (b layerBounds) union(o layerBounds) layerBounds {
	switch {
	case o.empty:
		return b
	case b.empty:
		return o
	}

	return layerBounds{
		known: true, x0: min(b.x0, o.x0), y0: min(b.y0, o.y0), x1: max(b.x1, o.x1), y1: max(b.y1, o.y1),
	}
}

// noteBounds records, for the layers of a stage that begins at the plan layer first, the pixels each can touch.
func (r *imageRun) noteBounds(first int, each []drawLayer) {
	if r.bounds == nil {
		return
	}

	for i := range each {
		r.bounds[first+i] = boundsOfDraw(each[i])
	}
}

// inheritBounds takes from the render before the bounds of the layers that have not changed since.
func (r *imageRun) inheritBounds(ch *layerChain, prev *lastFrame) {
	r.bounds = make([]layerBounds, len(ch.layer))

	if prev == nil || len(prev.layer) != len(ch.layer) {
		return
	}

	for i := range ch.layer {
		if prev.layer[i] == ch.layer[i] {
			r.bounds[i] = prev.bounds[i]
		}
	}
}

// remember keeps the render that just finished as the start of the next edit.
func (r *imageRun) remember(ch *layerChain, count int) {
	lf := &lastFrame{whole: ch.digest[count], layer: ch.layer, bounds: r.bounds}
	r.cache.setLastFrame(r.cfg.CacheScope, ch.shape, lf)
}

// renderWindow draws the edit of an earlier render as a window over its picture. base, from the layer start on,
// is the picture of the layers below start that the cache holds, or nil. It reports false when the edit is not
// one it can draw that way (nothing earlier to start from, a layer whose bounds are not known, a mask taken from
// another layer, more layers than one draw holds), and the render then draws as usual.
func (r *imageRun) renderWindow(
	live []plan.Layer, ch *layerChain, prev *lastFrame, last *image.RGBA, start int, base *image.RGBA,
) (*image.RGBA, bool, error) {
	if prev == nil || last == nil || len(prev.layer) != len(live) || len(live) == 0 || !windowAllowed(live, ch) {
		return nil, false, nil
	}

	changed, win, ok := editedSince(prev, ch)
	if !ok {
		return nil, false, nil
	}

	// What must be prepared first: the layers that changed (their new bounds), and those past the base whose
	// bounds are not known.
	need := make([]bool, len(live))
	for _, i := range changed {
		need[i] = true
	}

	for i := start; i < len(live); i++ {
		if !r.bounds[i].known {
			need[i] = true
		}
	}

	prepared, err := r.prepareSome(live, need)
	if err != nil {
		return nil, false, err
	}

	for i := range live {
		if need[i] {
			r.bounds[i] = boundsOfDraw(prepared[i])
		}
	}

	for _, i := range changed {
		win = win.union(r.bounds[i])
	}

	if win.empty {
		// the layers that changed drew nothing before and draw nothing now
		r.remember(ch, len(live))
		r.cache.put(r.cfg.CacheScope, ch.digest[len(live)], cloneRGBA(last))
		r.cache.countWindow()

		return cloneRGBA(last), true, nil
	}

	// The layers that reach the window and were not prepared yet (the cache holds most of them).
	more := make([]bool, len(live))

	for i := start; i < len(live); i++ {
		more[i] = !need[i] && r.bounds[i].reaches(win)
	}

	rest, err := r.prepareSome(live, more)
	if err != nil {
		return nil, false, err
	}

	var drawn []drawLayer

	for i := start; i < len(live); i++ {
		switch {
		case need[i] && prepared[i].pic.Empty(), more[i] && rest[i].pic.Empty():
		case need[i] && r.bounds[i].reaches(win):
			drawn = append(drawn, prepared[i])
		case more[i]:
			drawn = append(drawn, rest[i])
		}
	}

	if len(drawn)+boolInt(base != nil) > maxPlanLayers {
		return nil, false, nil
	}

	img, err := r.drawWindow(base, last, drawn, win)
	if err != nil {
		return nil, false, err
	}

	r.remember(ch, len(live))
	r.cache.put(r.cfg.CacheScope, ch.digest[len(live)], cloneRGBA(img))
	r.cache.countWindow()

	return img, true, nil
}

// windowAllowed tells that no layer reads another one (a clip or a stencil) or is a group, and that every layer
// has a digest.
func windowAllowed(live []plan.Layer, ch *layerChain) bool {
	for i := range live {
		l := &live[i]
		if l.Clip != nil || l.Stencil || l.Kind == plan.Group || ch.layer[i] == [sha256.Size]byte{} {
			return false
		}
	}

	return true
}

// editedSince gives the layers that differ from the render before, and the rectangle they reached there; false
// when there is none or one of them has no bounds from that render.
func editedSince(prev *lastFrame, ch *layerChain) ([]int, layerBounds, bool) {
	var changed []int

	win := layerBounds{known: true, empty: true}

	for i := range ch.layer {
		if prev.layer[i] == ch.layer[i] {
			continue
		}

		if !prev.bounds[i].known {
			return nil, win, false
		}

		changed = append(changed, i)
		win = win.union(prev.bounds[i])
	}

	return changed, win, len(changed) > 0
}

// prepareSome prepares the layers of live that need says, and gives one entry per layer of live (the zero value
// for those it did not prepare). The pictures come from the cache when it holds them.
func (r *imageRun) prepareSome(live []plan.Layer, need []bool) ([]drawLayer, error) {
	idx := make([]int, 0, len(live))
	sub := make([]plan.Layer, 0, len(live))

	for i := range live {
		if need[i] {
			idx = append(idx, i)
			sub = append(sub, live[i])
		}
	}

	each, err := r.prepareEach(sub)
	if err != nil {
		return nil, err
	}

	out := make([]drawLayer, len(live))
	for k, i := range idx {
		out[i] = each[k]
	}

	return out, nil
}

// drawWindow draws the layers over base inside the window of a copy of last, in bands of rows on instances of
// their own, and gives the copy. Outside the window the pixels are those of last.
func (r *imageRun) drawWindow(base, last *image.RGBA, drawn []drawLayer, win layerBounds) (*image.RGBA, error) {
	dst := cloneRGBA(last)

	bands := r.bands
	if bands <= 0 {
		want := min(maxImageWorkers, runtime.GOMAXPROCS(0), max((win.y1-win.y0)/minBandRows, 1))
		bands = bandCount(drawn, win.x1-win.x0, win.y0, win.y1, want, imageWorkMemory)
	}

	edges := bandEdges(drawn, win.x1-win.x0, win.y0, win.y1, bands)

	err := fanOut(r.ctx, bands, bands, func(b int) error {
		y0, y1 := edges[b], edges[b+1]
		if y0 >= y1 {
			return nil
		}

		return r.drawWindowBand(base, drawn, dst, win, y0, y1)
	})
	if err != nil {
		return nil, err
	}

	return dst, nil
}

// drawWindowBand draws the rows [y0, y1) of the window on an instance of its own: the window of base as a
// texture, then the prepared pictures of the layers that reach the band.
func (r *imageRun) drawWindowBand(
	base *image.RGBA, drawn []drawLayer, dst *image.RGBA, win layerBounds, y0, y1 int,
) error {
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
	rows, cols := y1-y0, win.x1-win.x0

	if base != nil {
		piece := make([]byte, 0, rows*cols*4)

		for y := y0; y < y1; y++ {
			o := y*base.Stride + win.x0*4
			piece = append(piece, base.Pix[o:o+cols*4]...)
		}

		if err := host.PutTexture(ctx, prefixItem, cols, rows, piece, !fp.Transparent); err != nil {
			return rendererError(err)
		}

		kept = append(kept, plan.Layer{
			ItemID: prefixItem, Kind: plan.Media, AssetID: prefixItem, AssetKind: "image",
			Crop: plan.Rect{W: float64(cols), H: float64(rows)}, CX: float64(win.x0) + float64(cols)/2,
			CY: float64(y0) + float64(rows)/2, W: float64(cols), H: float64(rows), Scale: 1, Opacity: 1,
		})
	}

	for i := range drawn {
		d := &drawn[i]
		if d.y1 <= y0 || d.y0 >= y1 || d.x1 <= win.x0 || d.x0 >= win.x1 {
			continue
		}

		if err := host.PutPrepared(ctx, d.layer.ItemID, d.pic); err != nil {
			return rendererError(err)
		}

		kept = append(kept, d.layer)
	}

	in := fp
	in.Layers = kept

	view, ok := dst.SubImage(image.Rect(0, y0, fp.Width, y1)).(*image.RGBA)
	if !ok {
		return rendererError(&renderhost.Error{Reason: renderhost.InvalidArgument, Detail: "the band is not a view"})
	}

	out := renderhost.Output{Width: fp.Width, Height: fp.Height, Scale: 1, Y0: y0, Y1: y1, X0: win.x0, X1: win.x1}
	if err := host.Render(ctx, renderhost.RenderInput{Output: out, Plan: in}, view); err != nil {
		return rendererError(err)
	}

	return nil
}
