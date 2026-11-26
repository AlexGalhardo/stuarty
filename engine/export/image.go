package export

import (
	"context"
	"errors"
	"fmt"
	"image"
	"sync"

	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/compositor"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/lut"
	"github.com/AlexGalhardo/stuarty/engine/rasterize"
	"github.com/AlexGalhardo/stuarty/engine/renderhost"
)

// ImageConfig is what the render of an image document needs from the machine (SDD-021 section 6).
type ImageConfig struct {
	// AssetPath gives the path of the normalized PNG of an image asset, by content hash.
	AssetPath func(hash string) (string, error)
	// LUTPath gives the path of the normalized .cube file of a lookup-table asset (SDD-021 section 3.1), by
	// content hash. It is needed only by a document with a layer that has a table.
	LUTPath func(hash string) (string, error)
	// Fonts is the font set of the text layers; the bundled fonts when nil.
	Fonts *rasterize.FontSet
	// Cache, when set, lets the render start from the picture of the layers that an earlier render of the same
	// document already drew (SDD-021 section 6.1); CacheScope is the owner of those pictures, a user in network
	// mode, and a render never reads a picture of another scope. A custom Fonts set turns the cache off.
	Cache      *ImageCache
	CacheScope string
}

// RenderImage draws an image document at its canvas size with the CPU path of the Rust renderer (the bit-exact
// one, through wazero), as the export of a video does: it builds the one-frame plan, rasterizes each text and
// shape, decodes each image layer, uploads them and takes the pixels. The same document and the same files
// always give the same pixels. The result is premultiplied RGBA, as image.RGBA is.
func RenderImage(ctx context.Context, doc *coreimage.Document, cfg ImageConfig) (*image.RGBA, error) {
	return renderImage(ctx, doc, cfg, 0)
}

// renderImage is renderImagePasses with the layers the module draws in one pass.
func renderImage(ctx context.Context, doc *coreimage.Document, cfg ImageConfig, bands int) (*image.RGBA, error) {
	return renderImagePasses(ctx, doc, cfg, bands, maxPlanLayers)
}

// renderImagePasses is RenderImage with the number of bands the canvas is drawn in; 0 is as many as the work and
// the machine allow (SDD-021 section 6.2), and with the layers one pass draws, passLayers (at most the module's
// maxPlanLayers; a test lowers it to force a document into several passes). The pixels depend on neither.
func renderImagePasses(
	ctx context.Context, doc *coreimage.Document, cfg ImageConfig, bands, passLayers int,
) (*image.RGBA, error) {
	fonts := cfg.Fonts
	if fonts == nil {
		fonts = rasterize.Bundled()
	}

	fp, live := livePlan(doc, fonts)

	// The cache keys layers by what they draw; a custom font set is not part of the key, so it is not cached.
	var chain *layerChain
	if cfg.Cache != nil && cfg.Fonts == nil {
		chain = newLayerChain(fp, live)
	}

	// The run draws and prepares; it also learns which pixels each layer reaches (SDD-021 section 6.3).
	run := &imageRun{ctx: ctx, cfg: cfg, plan: fp, raster: rasterize.NewCache(fonts, 0), bands: bands}

	var (
		prev *lastFrame
		last *image.RGBA
	)

	if chain != nil {
		run.cache = cfg.Cache
		prev, last = cfg.Cache.lastFrameOf(cfg.CacheScope, chain.shape)
		run.inheritBounds(chain, prev)
	}

	start, base := 0, (*image.RGBA)(nil)
	if chain != nil {
		// What the cache keeps was drawn from files the caller could read then, not necessarily now: an asset the
		// caller lost access to must not stay in the picture because it is cached (the export, with no cache, reads
		// every file).
		if err := verifyAccess(cfg, live); err != nil {
			return nil, err
		}

		start, base = cfg.Cache.nearest(cfg.CacheScope, chain)
		// with no layer to draw there is nothing cached (nearest starts at one layer): the canvas is drawn below
		if base != nil && start == len(live) {
			return cloneRGBA(base), nil
		}

		// An edit of some layers of the render before is a window of its picture drawn again.
		img, ok, err := run.renderWindow(live, chain, prev, last, start, base)
		if err != nil {
			return nil, err
		}

		if ok {
			return img, nil
		}
	}

	// A first stage that ends one layer before the top is kept, so that the next edit of the top layer draws
	// that layer over it; the stage after it draws the rest.
	stops := []int{len(live)}
	if chain != nil && len(live)-1 > start && len(live) >= 2 && chain.valid[len(live)-1] {
		stops = []int{len(live) - 1, len(live)}
	}

	// The module draws at most passLayers layers at once, so a longer stack is drawn in passes: each pass draws
	// the picture of the passes below it as its bottom layer (SDD-021 section 6.1, SDD-013 section 7.2).
	stops, err := splitPasses(live, start, base != nil, stops, passLayers)
	if err != nil {
		return nil, err
	}

	// The instances that do not test the context at every loop back edge draw about three times faster
	// (SDD-021 section 13.2); the cancellation is kept by checking the context between the layers and by
	// not waiting for the work in flight (SDD-021 section 6.2).
	for _, stop := range stops {
		img, err := run.stage(base, live[start:stop], start)
		if err != nil {
			return nil, err
		}

		// A cut a clip crosses has no picture of its own, so it is not kept (the passes avoid it when they can).
		if chain != nil && stop < len(live) && chain.valid[stop] {
			cfg.Cache.put(cfg.CacheScope, chain.digest[stop], img)
		}

		base, start = img, stop
	}

	if chain != nil {
		cfg.Cache.put(cfg.CacheScope, chain.digest[len(live)], cloneRGBA(base))
		run.remember(chain, len(live))
	}

	return base, nil
}

// splitPasses cuts the passes of stops into passes of at most limit layers, where a pass that starts from a
// picture counts that picture as one of its layers. A cut is the latest one under the limit that no clip
// crosses (a clip needs its source in the same draw); when a clip holds more layers than a pass draws together,
// there is no cut and the module's own limit is reported.
func splitPasses(live []plan.Layer, start int, haveBase bool, stops []int, limit int) ([]int, error) {
	var position map[string]int

	out := make([]int, 0, len(stops))

	for _, stop := range stops {
		for stop-start > limit-boolInt(haveBase) {
			if position == nil {
				position = layerPositions(live)
			}

			cut := start
			for k := start + limit - boolInt(haveBase); k > start; k-- {
				if cutIsClean(live, position, k) {
					cut = k

					break
				}
			}

			if cut == start {
				return nil, noCutError(live, position, start+limit-boolInt(haveBase), limit)
			}

			out = append(out, cut)
			start, haveBase = cut, true
		}

		out = append(out, stop)
		start, haveBase = stop, true
	}

	return out, nil
}

// noCutError is the refusal of a stack that has no cut a pass can end at: the masks taken from other layers
// cross every cut of a pass of limit layers (one layer 63 or more layers from its mask source, or pairs that
// overlap one another over that many layers). It names a layer whose mask crosses the last cut tried, k.
func noCutError(live []plan.Layer, position map[string]int, k, limit int) error {
	layer, source := "", ""

	for i := range live {
		c := live[i].Clip
		if c == nil {
			continue
		}

		if src, ok := position[c.Source]; ok && (i < k) != (src < k) {
			layer, source = live[i].ItemID, c.Source

			break
		}
	}

	return &ffmpeg.Error{
		Code:   ffmpeg.CodeLimitExceeded,
		Reason: "limit_exceeded",
		Message: fmt.Sprintf("The image cannot be drawn: its masks taken from other layers reach across more "+
			"than the %d layers the renderer draws at once.", limit),
		Hint: fmt.Sprintf("Move layer %s closer to %s, the layer it takes its mask from, or give it a mask of "+
			"its own.", layer, source),
		Details: map[string]any{"limit": "render_layers", "ceiling": limit, "layer": layer, "source": source},
		Err:     &renderhost.Error{Reason: renderhost.LimitExceeded, Detail: "render_layers"},
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}

	return 0
}

// livePlan is the one-frame plan of a document and the layers of it that draw something, in plan order.
func livePlan(doc *coreimage.Document, fonts *rasterize.FontSet) (plan.FramePlan, []plan.Layer) {
	fp := coreimage.Plan(doc, plan.Options{Measure: rasterize.Measure(fonts), OutputScale: 1})

	live := make([]plan.Layer, 0, len(fp.Layers))

	for i := range fp.Layers {
		if opacity(&fp.Layers[i]) != 0 {
			live = append(live, fp.Layers[i])
		}
	}

	return fp, live
}

// CheckImageAccess asks cfg for the file of every image and lookup table of the layers a render of doc would
// draw, and reads none of them: the error is the one the render would give. A caller that answers from a copy of
// a render (the frame of the editor, SDD-021 section 6.1) calls it first, so that what a person lost access to is
// refused whether or not a copy exists.
func CheckImageAccess(doc *coreimage.Document, cfg ImageConfig) error {
	fonts := cfg.Fonts
	if fonts == nil {
		fonts = rasterize.Bundled()
	}

	_, live := livePlan(doc, fonts)

	return verifyAccess(cfg, live)
}

// verifyAccess asks for the file of every image and lookup table the layers name, without reading it, so that a
// render that starts from the cache refuses what the export would refuse.
func verifyAccess(cfg ImageConfig, layers []plan.Layer) error {
	seen := map[string]bool{}

	for i := range layers {
		l := &layers[i]

		if l.Kind == plan.Media && !seen[l.AssetID] {
			seen[l.AssetID] = true

			if cfg.AssetPath == nil {
				return errors.New("no image files are available")
			}

			if _, err := cfg.AssetPath(l.AssetID); err != nil {
				return fmt.Errorf("layer %s: %w", l.ItemID, err)
			}
		}

		if l.Lut != nil && !seen["lut:"+l.Lut.Asset] {
			seen["lut:"+l.Lut.Asset] = true

			if cfg.LUTPath == nil {
				return exportErr(ReasonRender, "No lookup table files are available.", errors.New("no lut source"), "")
			}

			if _, err := cfg.LUTPath("sha256:" + l.Lut.Asset); err != nil {
				return exportErr(
					ReasonRender,
					"A lookup table of the image is not available.",
					fmt.Errorf("layer %s: %w", l.ItemID, err),
					"",
				)
			}
		}
	}

	return nil
}

// prefixItem is the id of the texture that holds the picture of the layers already drawn (SDD-021 section 6.1).
const prefixItem = "stuarty-prefix"

// imageRun is one render of a document: the plan, the rasterizer cache and the context; the instances of the
// module that prepare and draw it are made by the stages (image_bands.go).
type imageRun struct {
	ctx    context.Context
	cfg    ImageConfig
	plan   plan.FramePlan
	raster *rasterize.Cache
	bands  int
	// cache keeps the prepared layers, when the render may use one (SDD-021 section 6.2).
	cache *ImageCache
	// bounds is, for each live layer, the pixels its draw can touch, when the render knows them.
	bounds []layerBounds
	// preparedLimit bounds the bytes of the prepared pictures the render holds; 0 is imageWorkMemory.
	preparedLimit int64

	lutMu sync.Mutex
	luts  map[string]*lut.Table

	imgMu sync.Mutex
	imgs  map[string]*decodedImage
}

type decodedImage struct {
	once sync.Once
	img  compositor.Image
	err  error
}

// loadDecodedImage decodes path at most once per imageRun; concurrent layer preparations share the single decode.
func (r *imageRun) loadDecodedImage(path string) (compositor.Image, error) {
	r.imgMu.Lock()
	if r.imgs == nil {
		r.imgs = map[string]*decodedImage{}
	}

	entry, ok := r.imgs[path]
	if !ok {
		entry = &decodedImage{}
		r.imgs[path] = entry
	}
	r.imgMu.Unlock()

	entry.once.Do(func() {
		entry.img, entry.err = loadImage(path)
	})

	return entry.img, entry.err
}

// withoutLostClips drops the layers whose mask source did not make it to the renderer (a text with no pixels, an
// opacity that rounds to none): a layer clipped to nothing draws nothing, and the renderer refuses a clip whose
// source it does not hold (SDD-013 section 3.11.4).
func withoutLostClips(layers []plan.Layer) []plan.Layer {
	for {
		have := make(map[string]bool, len(layers))
		for i := range layers {
			have[layers[i].ItemID] = true
		}

		kept := layers[:0]

		for i := range layers {
			if c := layers[i].Clip; c != nil && !have[c.Source] {
				continue
			}

			kept = append(kept, layers[i])
		}

		if len(kept) == len(layers) {
			return kept
		}

		layers = kept
	}
}

// imageTexture is the pixels of an image layer: the part of the source the layer shows, scaled down to the box
// when the box is smaller, as the video export does, so a large photo is averaged and not point sampled.
func (r *imageRun) imageTexture(l *plan.Layer) (compositor.Image, error) {
	if r.cfg.AssetPath == nil {
		return compositor.Image{}, errors.New("no image files are available")
	}

	path, err := r.cfg.AssetPath(l.AssetID)
	if err != nil {
		return compositor.Image{}, err
	}

	src, err := r.loadDecodedImage(path)
	if err != nil {
		return compositor.Image{}, fmt.Errorf("image %s: %w", l.AssetID, err)
	}

	x, y := min(max(round(l.Crop.X), 0), max(src.W-1, 0)), min(max(round(l.Crop.Y), 0), max(src.H-1, 0))
	w, h := min(max(round(l.Crop.W), 1), src.W-x), min(max(round(l.Crop.H), 1), src.H-y)
	img := cropImage(src, x, y, w, h)

	if tw, th := max(round(l.W), 1), max(round(l.H), 1); l.Rotation == 0 && tw < w && th < h {
		img = compositor.Resize(img, tw, th)
	}

	return img, nil
}
