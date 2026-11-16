package compositor

import (
	"bytes"
	"encoding/binary"
	"hash/fnv"
	"image"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/engine/raster"
)

// The Rust renderer (renderer/, SDD-013 section 3) is held byte for byte to this compositor. These
// vectors are the contract between the two: small compositions with their inputs, and the frame
// this package draws for each. `cargo test` in renderer/ reads the same file and must draw the same
// bytes at one band and at eight. Regenerating the file is a reviewed change of output (rule
// .agents/skills/renderer-parity): VUKEM_WRITE_RENDERER_VECTORS=1 go test ./engine/compositor -run
// RendererVectors.
const vectorsPath = "../../renderer/testdata/compose-vectors.bin"

type vecLayer struct {
	layer  plan.Layer
	pix    []byte // straight alpha for a bitmap, premultiplied for media
	w, h   int
	opaque bool
	bitmap *raster.Bitmap
}

type vecCase struct {
	name   string
	w, h   int
	scale  float64
	bg     string
	layers []vecLayer
}

// softImage is a premultiplied image with a soft alpha edge, so blends and blurs see every alpha.
func softImage(w, h int, seed uint64) (pix []byte) {
	rng := rand.New(rand.NewPCG(seed, 11))
	pix = make([]byte, w*h*4)
	for y := range h {
		for x := range w {
			a := uint32(255)
			if x < 3 || y < 3 || x >= w-3 || y >= h-3 {
				a = uint32(rng.UintN(256))
			}
			i := (y*w + x) * 4
			for k := range 3 {
				pix[i+k] = u8(div255(uint32(rng.UintN(256)) * a))
			}
			pix[i+3] = uint8(a)
		}
	}
	return pix
}

func straightBitmap(w, h int, seed uint64) *raster.Bitmap {
	rng := rand.New(rand.NewPCG(seed, 13))
	pix := make([]byte, w*h*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = uint8(
			rng.UintN(256),
		), uint8(
			rng.UintN(256),
		), uint8(
			rng.UintN(256),
		), uint8(
			rng.UintN(256),
		)
	}
	return &raster.Bitmap{
		Pix:      pix,
		Width:    w,
		Height:   h,
		NaturalW: float64(w),
		NaturalH: float64(h),
		CenterDX: 1.5,
		CenterDY: -2,
	}
}

func vm(id string, img Image, cx, cy, w, h, rot, op float64) vecLayer {
	return vecLayer{
		layer: plan.Layer{ItemID: id, Kind: plan.Media, CX: cx, CY: cy, W: w, H: h, Rotation: rot, Opacity: op},
		pix:   img.Pix, w: img.W, h: img.H, opaque: img.Opaque,
	}
}

func vb(id string, bm *raster.Bitmap, cx, cy, w, h, rot, op float64) vecLayer {
	return vecLayer{
		layer: plan.Layer{
			ItemID:   id,
			Kind:     plan.Bitmap,
			Bitmap:   &plan.BitmapSpec{Kind: "text", Text: id, ActiveWord: -1},
			CX:       cx,
			CY:       cy,
			W:        w,
			H:        h,
			Rotation: rot,
			Opacity:  op,
		},
		bitmap: bm,
	}
}

// greenScreen is a premultiplied image of a green screen with noise, a skin tone with a green cast and two other colours,
// so that the key has pixels to remove, to fade, to despill and to leave.
func greenScreen(seed uint64) Image {
	const w, h = 40, 30

	rng := rand.New(rand.NewPCG(seed, 17))
	pix := make([]byte, w*h*4)

	for y := range h {
		for x := range w {
			i := (y*w + x) * 4
			r, g, b, a := 20+rng.UintN(60), 170+rng.UintN(80), 20+rng.UintN(70), uint32(255)

			switch {
			case x < w/4 && y < h/2:
				r, g, b = 200+rng.UintN(40), 160+rng.UintN(60), 110+rng.UintN(30)
			case x >= 3*w/4 && y < h/2:
				r, g, b = 200+rng.UintN(55), rng.UintN(60), rng.UintN(60)
			case y >= 3*h/4 && x < w/2:
				r, g, b = rng.UintN(60), rng.UintN(60), 190+rng.UintN(60)
			case y >= 3*h/4:
				r, g, b = 90+rng.UintN(80), 200+rng.UintN(50), 60+rng.UintN(80)
			}

			if x < 2 || y < 2 {
				a = uint32(rng.UintN(256))
			}

			pix[i], pix[i+1], pix[i+2], pix[i+3] = u8(
				div255(uint32(r)*a),
			), u8(
				div255(uint32(g)*a),
			), u8(
				div255(uint32(b)*a),
			), uint8(
				a,
			)
		}
	}

	return Image{Pix: pix, W: w, H: h}
}

func softMedia(w, h int, seed uint64) Image {
	return Image{Pix: softImage(w, h, seed), W: w, H: h}
}

func rendererVectorCases() []vecCase {
	opaque := pattern(16, 12, 1)
	big := pattern(40, 30, 2)
	soft := softMedia(20, 14, 3)
	sepia := &[12]float64{0.393, 0.769, 0.189, 0, 0.349, 0.686, 0.168, 0, 0.272, 0.534, 0.131, 0}
	invert := &[12]float64{-1, 0, 0, 1, 0, -1, 0, 1, 0, 0, -1, 1}
	bright := &[12]float64{1.2, 0, 0, 0.05, 0, 1.2, 0, 0.05, 0, 0, 1.2, 0.05}

	with := func(l vecLayer, f func(*plan.Layer)) vecLayer { f(&l.layer); return l }

	cases := []vecCase{
		{
			name:   "copy-opaque",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#102030",
			layers: []vecLayer{vm("a", opaque, 24, 18, 16, 12, 0, 1)},
		},
		{
			name:   "copy-clipped",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#ffffff",
			layers: []vecLayer{vm("a", opaque, 4, 3, 16, 12, 0, 1), vm("b", opaque, 46, 34, 16, 12, 0, 1)},
		},
		{
			name:   "copy-opacity",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#336699",
			layers: []vecLayer{vm("a", soft, 24, 18, 20, 14, 0, 0.5)},
		},
		{
			name:   "copy-alpha",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#cc3300",
			layers: []vecLayer{vm("a", soft, 20, 16, 20, 14, 0, 1)},
		},
		{
			name:   "scale-up",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#000000",
			layers: []vecLayer{vm("a", opaque, 24, 18, 40, 30, 0, 1)},
		},
		{
			name:   "scale-down",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#000000",
			layers: []vecLayer{vm("a", big, 24, 18, 13, 9.5, 0, 1)},
		},
		{
			name:   "out-scale",
			w:      24,
			h:      18,
			scale:  0.5,
			bg:     "#101010",
			layers: []vecLayer{vm("a", big, 24, 18, 40, 30, 0, 0.8)},
		},
		{
			name:   "rotate-15",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#202020",
			layers: []vecLayer{vm("a", opaque, 24, 18, 20, 15, 15, 1)},
		},
		{
			name:   "rotate-90",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#202020",
			layers: []vecLayer{vm("a", opaque, 24, 18, 20, 12, 90, 1)},
		},
		{
			name:   "rotate-neg-37.5-alpha",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#202020",
			layers: []vecLayer{vm("a", soft, 22, 20, 30, 21, -37.5, 0.7)},
		},
		{
			name:   "rotate-720.25",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#202020",
			layers: []vecLayer{vm("a", opaque, 24, 18, 18, 14, 720.25, 1)},
		},
		{
			name:  "colour-matrix",
			w:     48,
			h:     36,
			scale: 1,
			bg:    "#000000",
			layers: []vecLayer{
				with(vm("a", soft, 24, 18, 20, 14, 0, 1), func(l *plan.Layer) { l.ColorMatrix = bright }),
			},
		},
		{
			name:   "filter-sepia",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#000000",
			layers: []vecLayer{with(vm("a", opaque, 24, 18, 16, 12, 0, 1), func(l *plan.Layer) { l.Filter = sepia })},
		},
		{
			name:  "colour-then-filter-invert",
			w:     48,
			h:     36,
			scale: 1,
			bg:    "#101010",
			layers: []vecLayer{
				with(
					vm("a", soft, 24, 18, 20, 14, 10, 0.9),
					func(l *plan.Layer) { l.ColorMatrix = bright; l.Filter = invert },
				),
			},
		},
		{
			name:   "blur-2",
			w:      64,
			h:      48,
			scale:  1,
			bg:     "#000000",
			layers: []vecLayer{with(vm("a", soft, 32, 24, 20, 14, 0, 1), func(l *plan.Layer) { l.Blur = 2 })},
		},
		{
			name:   "blur-6",
			w:      64,
			h:      48,
			scale:  1,
			bg:     "#101820",
			layers: []vecLayer{with(vm("a", soft, 32, 24, 20, 14, 0, 1), func(l *plan.Layer) { l.Blur = 6 })},
		},
		{
			name:   "blur-16-rotated",
			w:      96,
			h:      72,
			scale:  1,
			bg:     "#101820",
			layers: []vecLayer{with(vm("a", soft, 48, 36, 20, 14, 20, 0.8), func(l *plan.Layer) { l.Blur = 16 })},
		},
		{
			name:   "blur-scaled-up",
			w:      48,
			h:      36,
			scale:  1,
			bg:     "#000000",
			layers: []vecLayer{with(vm("a", big, 24, 18, 30, 24, 0, 1), func(l *plan.Layer) { l.Blur = 3 })},
		},
		{
			name:   "bitmap-metrics",
			w:      64,
			h:      48,
			scale:  1,
			bg:     "#202020",
			layers: []vecLayer{vb("t1", straightBitmap(24, 16, 5), 32, 24, 24, 16, 0, 1)},
		},
		{
			name:   "bitmap-shrunk",
			w:      32,
			h:      24,
			scale:  1,
			bg:     "#202020",
			layers: []vecLayer{vb("t2", straightBitmap(96, 64, 6), 16, 12, 24, 16, 0, 1)},
		},
		{
			name:  "bitmap-rotated-blur",
			w:     64,
			h:     48,
			scale: 0.5,
			bg:    "#202020",
			layers: []vecLayer{
				with(vb("t3", straightBitmap(48, 32, 7), 64, 48, 48, 32, 25, 0.9), func(l *plan.Layer) { l.Blur = 8 }),
			},
		},
		{
			name:  "chroma-hard",
			w:     64,
			h:     48,
			scale: 1,
			bg:    "#101820",
			layers: []vecLayer{with(vm("a", greenScreen(21), 32, 24, 40, 30, 0, 1), func(l *plan.Layer) {
				l.ChromaKey = &plan.ChromaKeySpec{Color: "#00C81E", Similarity: 0.3}
			})},
		},
		{
			name:  "chroma-soft-spill",
			w:     64,
			h:     48,
			scale: 1,
			bg:    "#402010",
			layers: []vecLayer{with(vm("a", greenScreen(22), 32, 24, 40, 30, 0, 1), func(l *plan.Layer) {
				l.ChromaKey = &plan.ChromaKeySpec{Color: "#00FF00", Similarity: 0.25, Smoothness: 0.3, Spill: 0.7}
			})},
		},
		{
			name:  "chroma-alpha-rotated-matrix-blur",
			w:     96,
			h:     72,
			scale: 1,
			bg:    "#202020",
			layers: []vecLayer{with(vm("a", greenScreen(23), 48, 36, 50, 38, 21, 0.85), func(l *plan.Layer) {
				l.ChromaKey = &plan.ChromaKeySpec{Color: "#2EBE32", Similarity: 0.3, Smoothness: 0.15, Spill: 1}
				l.ColorMatrix = bright
				l.Blur = 3
			})},
		},
		{name: "chroma-grey-key-and-bitmap", w: 64, h: 48, scale: 1, bg: "#303030", layers: []vecLayer{
			with(vm("a", greenScreen(24), 24, 24, 40, 30, 0, 1), func(l *plan.Layer) {
				l.ChromaKey = &plan.ChromaKeySpec{Color: "#808080", Similarity: 0.2, Smoothness: 0.2, Spill: 1}
			}),
			with(vb("t4", straightBitmap(24, 16, 9), 44, 30, 24, 16, 0, 1), func(l *plan.Layer) {
				l.ChromaKey = &plan.ChromaKeySpec{Color: "#FF0000", Similarity: 0.4, Smoothness: 0.1, Spill: 0.5}
			}),
		}},
		{name: "stack", w: 64, h: 48, scale: 1, bg: "#0a0a0a", layers: []vecLayer{
			vm("a", big, 32, 24, 64, 48, 0, 1),
			vm("b", soft, 20, 20, 20, 14, 12, 0.6),
			vb("t", straightBitmap(24, 16, 8), 40, 30, 24, 16, 0, 1),
			vm("c", opaque, 50, 12, 16, 12, 0, 0),
		}},
	}
	return cases
}

func (c vecCase) compose(t *testing.T, bands int) []byte {
	t.Helper()
	imgs := map[string]Image{}
	bms := map[string]raster.Bitmap{}
	var layers []plan.Layer
	for i := range c.layers {
		l := &c.layers[i]
		layers = append(layers, l.layer)
		if l.layer.Kind == plan.Bitmap {
			bms[l.layer.Bitmap.Key()] = *l.bitmap
		} else {
			imgs[l.layer.ItemID] = Image{Pix: l.pix, W: l.w, H: l.h, Opaque: l.opaque}
		}
	}
	comp := &Compositor{
		Bands:   bands,
		Media:   func(l *plan.Layer) (Image, error) { return imgs[l.ItemID], nil },
		Bitmaps: func(s plan.BitmapSpec) (raster.Bitmap, error) { return bms[s.Key()], nil },
	}
	dst := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
	if err := comp.Compose(dst, plan.FramePlan{Background: c.bg, Layers: layers}, c.scale); err != nil {
		t.Fatal(err)
	}
	return dst.Pix
}

// sincosHash is the FNV-1a 64 of the bits of sincos over 10,001 angles in [-1000, 1000] and over the
// multiples of 15 degrees from -720 to 720; the Rust test computes the same hash.
func sincosHash() uint64 {
	h := fnv.New64a()
	put := func(x float64) {
		s, c := sincos(x)
		var b [16]byte
		binary.LittleEndian.PutUint64(b[:8], math.Float64bits(s))
		binary.LittleEndian.PutUint64(b[8:], math.Float64bits(c))
		_, _ = h.Write(b[:])
	}
	for i := range 10001 {
		put(-1000 + 2000*float64(i)/10000)
	}
	for d := -720; d <= 720; d += 15 {
		put(float64(d) * (math.Pi / 180))
	}
	return h.Sum64()
}

func encodeVectors(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := func(v any) {
		if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	buf.WriteString("VVCV")
	w(uint32(2))
	w(sincosHash())
	cases := rendererVectorCases()
	w(uint32(len(cases)))
	for _, c := range cases {
		w(uint8(len(c.name)))
		buf.WriteString(c.name)
		w(uint32(c.w))
		w(uint32(c.h))
		w(c.scale)
		bg := parseColour(c.bg)
		buf.Write(bg[:])
		w(uint32(len(c.layers)))
		for i := range c.layers {
			l := &c.layers[i]
			pl := l.layer
			if pl.Kind == plan.Bitmap {
				w(uint8(1))
				w(uint32(l.bitmap.Width))
				w(uint32(l.bitmap.Height))
				w(uint8(0))
				buf.Write(l.bitmap.Pix)
				w(l.bitmap.NaturalW)
				w(l.bitmap.CenterDX)
				w(l.bitmap.CenterDY)
			} else {
				w(uint8(0))
				w(uint32(l.w))
				w(uint32(l.h))
				op := uint8(0)
				if l.opaque {
					op = 1
				}
				w(op)
				buf.Write(l.pix)
			}
			for _, v := range []float64{pl.CX, pl.CY, pl.W, pl.H, pl.Rotation, pl.Opacity, pl.Blur} {
				w(v)
			}
			for _, m := range []*[12]float64{pl.ColorMatrix, pl.Filter} {
				if m == nil {
					w(uint8(0))
					continue
				}
				w(uint8(1))
				w(*m)
			}

			if k := pl.ChromaKey; k == nil {
				w(uint8(0))
			} else {
				w(uint8(1))
				w(uint8(len(k.Color)))
				buf.WriteString(k.Color)

				for _, v := range []float64{k.Similarity, k.Smoothness, k.Spill} {
					w(v)
				}
			}
		}
		buf.Write(c.compose(t, 1))
	}
	return buf.Bytes()
}

// TestRendererVectorsAreCurrent keeps renderer/testdata/compose-vectors.bin equal to what this
// compositor draws, and the bands of every vector equal to each other.
func TestRendererVectorsAreCurrent(t *testing.T) {
	for _, c := range rendererVectorCases() {
		one, eight := c.compose(t, 1), c.compose(t, 8)
		if !bytes.Equal(one, eight) {
			t.Errorf("%s: one band and eight bands differ", c.name)
		}
	}

	got := encodeVectors(t)
	if os.Getenv("VUKEM_WRITE_RENDERER_VECTORS") == "1" {
		if err := os.MkdirAll(filepath.Dir(vectorsPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(vectorsPath, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal(
			"renderer/testdata/compose-vectors.bin is out of date: regenerate it with VUKEM_WRITE_RENDERER_VECTORS=1 and review the change",
		)
	}
}
