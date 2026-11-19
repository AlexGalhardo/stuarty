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
	"strconv"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/bitmap"
	"github.com/AlexGalhardo/stuarty/engine/lut"
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
	bitmap *bitmap.Bitmap
	// table is the lookup table that layer.Lut names.
	table *lut.Table
}

type vecCase struct {
	name   string
	w, h   int
	scale  float64
	bg     string
	layers []vecLayer
	// frame, fw and fh are the plan's frame number and the project's size (the grain and the whip use them).
	frame, fw, fh int
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

func straightBitmap(w, h int, seed uint64) *bitmap.Bitmap {
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
	return &bitmap.Bitmap{
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

func vb(id string, bm *bitmap.Bitmap, cx, cy, w, h, rot, op float64) vecLayer {
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
	cube := colourCube()
	table5, table17 := mixTable(5), mixTable(17)
	hsl := []timeline.HslBand{
		{Range: "red", Hue: 0.5, Saturation: 0.4, Lightness: -0.2},
		{Range: "yellow", Hue: -0.8, Lightness: 0.5},
		{Range: "green", Saturation: -0.7},
		{Range: "blue", Hue: 1, Saturation: 1, Lightness: 1},
		{Range: "magenta", Hue: -1, Saturation: -1, Lightness: -1},
	}
	wheels := &timeline.Wheels{
		Shadows:    &timeline.Wheel{Hue: 220, Amount: 0.6, Luma: -0.2},
		Midtones:   &timeline.Wheel{Hue: 40.5, Amount: 0.3, Luma: 0.1},
		Highlights: &timeline.Wheel{Hue: 359.9, Amount: 0.8, Luma: 0.3},
	}
	curves := &timeline.Curves{
		Master: []timeline.CurvePoint{{X: 0, Y: 0.1}, {X: 0.3, Y: 0.2}, {X: 0.7, Y: 0.9}, {X: 1, Y: 1}},
		Red:    []timeline.CurvePoint{{X: 0, Y: 0}, {X: 0.5, Y: 0.7}, {X: 1, Y: 1}},
		Green:  []timeline.CurvePoint{{X: 0.2, Y: 0.2}, {X: 0.4, Y: 0.2}, {X: 0.8, Y: 0.6}},
		Blue:   []timeline.CurvePoint{{X: 0, Y: 1}, {X: 1, Y: 0}},
	}

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
		{name: "blend-multiply-screen", w: 64, h: 48, scale: 1, bg: "#506070", layers: []vecLayer{
			vm("a", big, 32, 24, 64, 48, 0, 1),
			with(vm("b", opaque, 24, 24, 16, 12, 0, 1), func(l *plan.Layer) { l.Blend = "multiply" }),
			with(vm("c", soft, 40, 24, 20, 14, 0, 1), func(l *plan.Layer) { l.Blend = "screen" }),
			with(vm("d", soft, 32, 36, 20, 14, 10, 0.5), func(l *plan.Layer) { l.Blend = "multiply" }),
		}},
		{name: "blend-overlay-add", w: 64, h: 48, scale: 1, bg: "#808080", layers: []vecLayer{
			vm("a", big, 32, 24, 64, 48, 0, 1),
			with(vm("b", opaque, 24, 24, 16, 12, 0, 1), func(l *plan.Layer) { l.Blend = "overlay" }),
			with(vm("c", soft, 40, 24, 20, 14, 0, 0.7), func(l *plan.Layer) { l.Blend = "overlay" }),
			with(vm("d", soft, 24, 36, 20, 14, 0, 1), func(l *plan.Layer) { l.Blend = "add" }),
			with(vm("e", opaque, 44, 38, 24, 18, 0, 0.5), func(l *plan.Layer) { l.Blend = "add" }),
		}},
		{name: "blend-rotated-filter-bitmap", w: 64, h: 48, scale: 1, bg: "#a04020", layers: []vecLayer{
			with(vm("a", big, 32, 24, 40, 30, 20, 1), func(l *plan.Layer) {
				l.Blend, l.Blur, l.Filter = "overlay", 2, sepia
			}),
			with(
				vb("t5", straightBitmap(24, 16, 10), 40, 30, 24, 16, 0, 1),
				func(l *plan.Layer) { l.Blend = "screen" },
			),
			with(vm("g", greenScreen(25), 20, 16, 40, 30, 0, 1), func(l *plan.Layer) {
				l.Blend = "multiply"
				l.ChromaKey = &plan.ChromaKeySpec{Color: "#00C81E", Similarity: 0.3, Smoothness: 0.2}
			}),
		}},
		{name: "mask-rect-copy", w: 64, h: 48, scale: 1, bg: "#304050", layers: []vecLayer{
			vm("a", big, 32, 24, 64, 48, 0, 1),
			with(vm("b", opaque, 24, 24, 16, 12, 0, 1), func(l *plan.Layer) {
				l.Mask = &plan.MaskSpec{Shape: "rect", X: 0.25, Y: 0.1, Width: 0.5, Height: 0.7}
			}),
			with(vm("c", soft, 44.5, 26.5, 20, 14, 0, 0.8), func(l *plan.Layer) {
				l.Mask = &plan.MaskSpec{Shape: "rect", X: 0, Y: 0, Width: 0.6, Height: 1, Feather: 5}
			}),
		}},
		{name: "mask-ellipse-rotated-feather", w: 96, h: 72, scale: 1, bg: "#203040", layers: []vecLayer{
			vm("a", big, 48, 36, 96, 72, 0, 1),
			with(vm("b", big, 44, 34, 50, 38, 25, 1), func(l *plan.Layer) {
				l.Mask = &plan.MaskSpec{Shape: "ellipse", X: 0.1, Y: 0.05, Width: 0.8, Height: 0.9, Feather: 6}
			}),
			with(vm("c", soft, 64, 46, 30, 21, -40, 0.6), func(l *plan.Layer) {
				l.Mask = &plan.MaskSpec{Shape: "ellipse", X: 0.2, Y: 0.2, Width: 0.5, Height: 0.6, Invert: true}
			}),
		}},
		{name: "mask-invert-blur-blend-bitmap", w: 64, h: 48, scale: 0.5, bg: "#802010", layers: []vecLayer{
			vm("a", big, 32, 24, 64, 48, 0, 1),
			with(vm("b", soft, 30, 24, 40, 28, 12, 1), func(l *plan.Layer) {
				l.Blur, l.Blend, l.Filter = 3, "overlay", sepia
				l.Mask = &plan.MaskSpec{
					Shape:   "rect",
					X:       0.1,
					Y:       0.1,
					Width:   0.8,
					Height:  0.8,
					Feather: 8,
					Invert:  true,
				}
			}),
			with(vb("t6", straightBitmap(24, 16, 12), 50, 34, 24, 16, 0, 1), func(l *plan.Layer) {
				l.Mask = &plan.MaskSpec{Shape: "ellipse", X: 0, Y: 0, Width: 1, Height: 1, Feather: 4}
			}),
			with(vm("g", greenScreen(26), 20, 16, 40, 30, 0, 1), func(l *plan.Layer) {
				l.ChromaKey = &plan.ChromaKeySpec{Color: "#00C81E", Similarity: 0.3, Smoothness: 0.2}
				l.Blend = "screen"
				l.Mask = &plan.MaskSpec{Shape: "ellipse", X: 0.3, Y: 0.3, Width: 0.001, Height: 0.001}
			}),
		}},
		{name: "mask-blurred-copy", w: 48, h: 36, scale: 1, bg: "#102030", layers: []vecLayer{
			with(vm("a", big, 24.4, 18.6, 40, 30, 0, 1), func(l *plan.Layer) {
				l.Blur = 2
				l.Mask = &plan.MaskSpec{Shape: "rect", X: 0.2, Y: 0.2, Width: 0.6, Height: 0.6, Feather: 2}
			}),
		}},
		{name: "flip-copy-opaque", w: 64, h: 48, scale: 1, bg: "#304050", layers: []vecLayer{
			vm("a", big, 32, 24, 64, 48, 0, 1),
			with(vm("b", opaque, 20, 18, 24, 16, 0, 1), func(l *plan.Layer) { l.FlipX = true }),
			with(vm("c", opaque, 46, 30, 20, 14, 0, 1), func(l *plan.Layer) { l.FlipY = true }),
			with(vm("d", soft, 44, 14, 16, 12, 0, 0.7), func(l *plan.Layer) { l.FlipX, l.FlipY = true, true }),
		}},
		{name: "flip-rotated-bilinear", w: 96, h: 72, scale: 1, bg: "#203040", layers: []vecLayer{
			vm("a", big, 48, 36, 96, 72, 0, 1),
			with(vm("b", big, 40, 34, 50, 38, 25, 1), func(l *plan.Layer) { l.FlipX = true }),
			with(vm("c", soft, 68, 44, 31, 21, -40, 0.6), func(l *plan.Layer) { l.FlipY = true }),
			with(vm("d", opaque, 20, 56, 23, 15, 133, 1), func(l *plan.Layer) { l.FlipX, l.FlipY = true, true }),
		}},
		{name: "flip-blur-mask-blend-bitmap", w: 64, h: 48, scale: 0.5, bg: "#802010", layers: []vecLayer{
			vm("a", big, 32, 24, 64, 48, 0, 1),
			with(vm("b", soft, 30, 24, 40, 28, 12, 1), func(l *plan.Layer) {
				l.Blur, l.Blend, l.FlipX = 3, "overlay", true
				l.Mask = &plan.MaskSpec{Shape: "rect", X: 0.1, Y: 0.1, Width: 0.5, Height: 0.8, Feather: 6}
			}),
			with(vb("t7", straightBitmap(24, 16, 14), 50, 34, 24, 16, 0, 1), func(l *plan.Layer) {
				l.FlipX, l.FlipY = true, true
				l.Mask = &plan.MaskSpec{Shape: "ellipse", X: 0, Y: 0, Width: 0.6, Height: 1}
			}),
			with(vm("g", greenScreen(27), 20, 16, 40, 30, 0, 1), func(l *plan.Layer) {
				l.FlipY = true
				l.ChromaKey = &plan.ChromaKeySpec{Color: "#00C81E", Similarity: 0.3, Smoothness: 0.2}
			}),
		}},
		{
			name:  "effect-motion-blur",
			w:     64,
			h:     48,
			scale: 1,
			bg:    "#203040",
			frame: 3,
			fw:    64,
			fh:    48,
			layers: []vecLayer{
				vm("a", big, 32, 24, 64, 48, 0, 1),
				with(vm("b", soft, 20, 20, 20, 14, 0, 1), func(l *plan.Layer) {
					l.Effects = []timeline.Effect{
						fx(timeline.EffectMotionBlur, map[string]float64{"amount": 0.5, "angle": 0}),
					}
				}),
				with(vm("c", soft, 46, 30, 24, 17, 23, 0.8), func(l *plan.Layer) {
					l.Effects = []timeline.Effect{
						fx(timeline.EffectMotionBlur, map[string]float64{"amount": 0.2, "angle": -125}),
					}
				}),
			},
		},
		{name: "effect-glow", w: 64, h: 48, scale: 1, bg: "#101010", frame: 0, fw: 64, fh: 48, layers: []vecLayer{
			with(vm("a", big, 24, 24, 40, 30, 0, 1), func(l *plan.Layer) {
				l.Effects = []timeline.Effect{
					fx(timeline.EffectGlow, map[string]float64{"amount": 0.8, "radius": 3, "threshold": 0.4}),
				}
			}),
			with(vm("b", soft, 46, 30, 24, 17, -17, 0.9), func(l *plan.Layer) {
				l.Effects = []timeline.Effect{
					fx(timeline.EffectGlow, map[string]float64{"amount": 1, "radius": 2, "threshold": 0.2}),
				}
			}),
		}},
		{
			name:  "effect-vignette-grain",
			w:     64,
			h:     48,
			scale: 1,
			bg:    "#202020",
			frame: 7,
			fw:    64,
			fh:    48,
			layers: []vecLayer{
				with(vm("a", big, 32, 24, 64, 48, 0, 1), func(l *plan.Layer) {
					l.Effects = []timeline.Effect{
						fx(timeline.EffectVignette, map[string]float64{"amount": 0.8, "softness": 0.4}),
						fx(timeline.EffectGrain, map[string]float64{"amount": 0.7, "size": 2, "seed": 9}),
					}
				}),
				with(vm("b", soft, 20, 24, 20, 14, 31, 1), func(l *plan.Layer) {
					l.Effects = []timeline.Effect{
						fx(timeline.EffectGrain, map[string]float64{"amount": 1, "size": 1, "seed": 65535}),
					}
				}),
				with(vb("t", straightBitmap(24, 16, 15), 50, 34, 24, 16, 0, 1), func(l *plan.Layer) {
					l.Effects = []timeline.Effect{
						fx(timeline.EffectVignette, map[string]float64{"amount": 1, "softness": 0}),
					}
				}),
			},
		},
		{
			name:  "effect-rgb-split",
			w:     64,
			h:     48,
			scale: 0.5,
			bg:    "#303030",
			frame: 0,
			fw:    128,
			fh:    96,
			layers: []vecLayer{
				with(vm("a", soft, 64, 48, 60, 42, 0, 1), func(l *plan.Layer) {
					l.Effects = []timeline.Effect{
						fx(timeline.EffectRGBSplit, map[string]float64{"amount": 0.9, "angle": 35}),
					}
				}),
				with(vm("b", big, 30, 30, 40, 30, 12, 1), func(l *plan.Layer) {
					l.Effects = []timeline.Effect{
						fx(timeline.EffectRGBSplit, map[string]float64{"amount": 0.3, "angle": 180}),
					}
				}),
			},
		},
		{
			name: "effects-stacked-masked-flipped", w: 96, h: 72, scale: 1, bg: "#202838", frame: 12, fw: 96, fh: 72,
			layers: []vecLayer{
				vm("a", big, 48, 36, 96, 72, 0, 1),
				with(vm("b", greenScreen(28), 50, 36, 50, 38, 15, 0.9), func(l *plan.Layer) {
					l.Blur, l.FlipX = 2, true
					l.Mask = &plan.MaskSpec{Shape: "ellipse", X: 0.1, Y: 0.1, Width: 0.8, Height: 0.8, Feather: 6}
					l.Effects = []timeline.Effect{
						fx(timeline.EffectMotionBlur, map[string]float64{"amount": 0.3, "angle": 20}),
						fx(timeline.EffectGlow, map[string]float64{"amount": 0.6, "radius": 3, "threshold": 0.3}),
						fx(timeline.EffectVignette, map[string]float64{"amount": 0.7, "softness": 0.6}),
						fx(timeline.EffectGrain, map[string]float64{"amount": 0.4, "size": 3, "seed": 5}),
						fx(timeline.EffectRGBSplit, map[string]float64{"amount": 0.4, "angle": -60}),
					}
				}),
			},
		},
		{name: "transition-whip", w: 96, h: 72, scale: 1, bg: "#202838", frame: 4, fw: 96, fh: 72, layers: []vecLayer{
			vm("a", big, 48, 36, 96, 72, 0, 1),
			with(vm("b", soft, 30, 24, 30, 21, 0, 1), func(l *plan.Layer) {
				l.Transition = &plan.TransitionSpec{Kind: "whip", Edge: "in", P: 6000, Direction: "right"}
			}),
			with(vm("c", big, 66, 48, 36, 27, 14, 1), func(l *plan.Layer) {
				l.Transition = &plan.TransitionSpec{Kind: "whip", Edge: "out", P: 12000, Direction: "up"}
			}),
		}},
		{name: "transition-dip", w: 64, h: 48, scale: 1, bg: "#202838", frame: 4, fw: 64, fh: 48, layers: []vecLayer{
			vm("a", big, 32, 24, 64, 48, 0, 1),
			with(vm("b", soft, 20, 24, 20, 14, 0, 1), func(l *plan.Layer) {
				l.Transition = &plan.TransitionSpec{Kind: "dip", Edge: "in", P: 20000, Color: "#ff8000"}
			}),
			with(vm("c", big, 46, 24, 24, 18, 20, 0.8), func(l *plan.Layer) {
				l.Transition = &plan.TransitionSpec{Kind: "dip", Edge: "out", P: 50000, Color: "#000000"}
			}),
		}},
		{name: "transition-wipe", w: 96, h: 72, scale: 1, bg: "#202838", frame: 4, fw: 96, fh: 72, layers: []vecLayer{
			vm("a", big, 48, 36, 96, 72, 0, 1),
			with(vm("b", opaque, 24, 20, 32, 24, 0, 1), func(l *plan.Layer) {
				l.Transition = &plan.TransitionSpec{Kind: "wipe", Edge: "in", P: 32768, Direction: "left"}
			}),
			with(vm("c", soft, 70, 20, 30, 21, 17, 1), func(l *plan.Layer) {
				l.Transition = &plan.TransitionSpec{Kind: "wipe", Edge: "out", P: 20000, Direction: "up"}
				l.Mask = &plan.MaskSpec{Shape: "ellipse", X: 0, Y: 0, Width: 1, Height: 1, Feather: 4}
			}),
			with(vm("d", big, 30, 56, 36, 27, -9, 0.9), func(l *plan.Layer) {
				l.Transition = &plan.TransitionSpec{Kind: "wipe", Edge: "in", P: 41000, Direction: "down"}
				l.Blur = 1
			}),
			with(vm("e", opaque, 74, 56, 28, 20, 0, 1), func(l *plan.Layer) {
				l.Transition = &plan.TransitionSpec{Kind: "wipe", Edge: "out", P: 9000, Direction: "right"}
			}),
		}},
		{name: "grade-hsl", w: 64, h: 64, scale: 1, bg: "#000000", layers: []vecLayer{
			with(vm("a", cube, 32, 32, 64, 64, 0, 1), func(l *plan.Layer) { l.Grade = &plan.GradeSpec{Hsl: hsl} }),
		}},
		{name: "grade-wheels", w: 64, h: 64, scale: 1, bg: "#000000", layers: []vecLayer{
			with(
				vm("a", cube, 32, 32, 64, 64, 0, 1),
				func(l *plan.Layer) { l.Grade = &plan.GradeSpec{Wheels: wheels} },
			),
		}},
		{name: "grade-curves", w: 64, h: 64, scale: 1, bg: "#000000", layers: []vecLayer{
			with(
				vm("a", cube, 32, 32, 64, 64, 0, 1),
				func(l *plan.Layer) { l.Grade = &plan.GradeSpec{Curves: curves} },
			),
		}},
		{name: "grade-all", w: 96, h: 72, scale: 1, bg: "#202838", layers: []vecLayer{
			vm("a", big, 48, 36, 96, 72, 0, 1),
			with(vm("b", soft, 40, 36, 40, 28, 12, 0.9), func(l *plan.Layer) {
				l.ColorMatrix = bright
				l.Grade = &plan.GradeSpec{Hsl: hsl, Wheels: wheels, Curves: curves}
				l.Filter = sepia
			}),
			with(vm("c", cube, 70, 40, 50, 50, 0, 1), func(l *plan.Layer) {
				l.Grade = &plan.GradeSpec{Hsl: hsl[:2], Curves: &timeline.Curves{Master: curves.Master}}
			}),
		}},
		{name: "lut-5", w: 64, h: 64, scale: 1, bg: "#000000", layers: []vecLayer{
			withLUT(vm("a", cube, 32, 32, 64, 64, 0, 1), table5, 1),
		}},
		{name: "lut-17-mixed", w: 64, h: 64, scale: 1, bg: "#000000", layers: []vecLayer{
			withLUT(vm("a", cube, 32, 32, 64, 64, 0, 1), table17, 0.37),
		}},
		{name: "lut-stacked", w: 96, h: 72, scale: 1, bg: "#202838", layers: []vecLayer{
			vm("a", big, 48, 36, 96, 72, 0, 1),
			with(withLUT(vm("b", soft, 40, 36, 40, 28, 12, 0.9), table17, 0.8), func(l *plan.Layer) {
				l.ColorMatrix = bright
				l.Grade = &plan.GradeSpec{Hsl: hsl, Curves: &timeline.Curves{Master: curves.Master}}
				l.Filter = sepia
			}),
			withLUT(vm("c", greenScreen(31), 70, 40, 40, 30, -8, 1), table5, 0.5),
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
	bms := map[string]bitmap.Bitmap{}
	tables := map[string]*lut.Table{}
	var layers []plan.Layer
	for i := range c.layers {
		l := &c.layers[i]
		layers = append(layers, l.layer)
		if l.table != nil {
			tables[l.layer.Lut.Asset] = l.table
		}
		if l.layer.Kind == plan.Bitmap {
			bms[l.layer.Bitmap.Key()] = *l.bitmap
		} else {
			imgs[l.layer.ItemID] = Image{Pix: l.pix, W: l.w, H: l.h, Opaque: l.opaque}
		}
	}
	comp := &Compositor{
		Bands:   bands,
		Media:   func(l *plan.Layer) (Image, error) { return imgs[l.ItemID], nil },
		Bitmaps: func(s plan.BitmapSpec) (bitmap.Bitmap, error) { return bms[s.Key()], nil },
		LUTs:    func(asset string) (*lut.Table, error) { return tables[asset], nil },
	}
	dst := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
	if err := comp.Compose(
		dst, plan.FramePlan{Frame: c.frame, Width: c.fw, Height: c.fh, Background: c.bg, Layers: layers}, c.scale,
	); err != nil {
		t.Fatal(err)
	}
	return dst.Pix
}

func effectCode(kind string) uint8 {
	switch kind {
	case timeline.EffectMotionBlur:
		return 0
	case timeline.EffectGlow:
		return 1
	case timeline.EffectVignette:
		return 2
	case timeline.EffectGrain:
		return 3
	case timeline.EffectRGBSplit:
		return 4
	}

	panic("not a pixel effect: " + kind)
}

func transitionCode(kind string) uint8 {
	switch kind {
	case "whip":
		return 1
	case "dip":
		return 2
	case "wipe":
		return 3
	}

	panic("not a pixel transition: " + kind)
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

// withLUT gives the layer the table, as the asset "lut-<size>", mixed in by the intensity.
func withLUT(l vecLayer, t *lut.Table, intensity float64) vecLayer {
	l.table = t
	l.layer.Lut = &plan.LutSpec{Asset: "lut-" + strconv.Itoa(t.Size), Intensity: intensity}

	return l
}

// mixTable is a table of n points on a side that mixes the three channels, so that the interpolation sees
// entries that differ on every axis.
func mixTable(n int) *lut.Table {
	t := &lut.Table{Size: n, Data: make([]uint16, 0, 3*n*n*n)}
	v := n - 1

	for b := range n {
		for g := range n {
			for r := range n {
				t.Data = append(t.Data,
					uint16((7*r+2*g+b)*65535/(10*v)),
					uint16((r+6*g+3*b)*65535/(10*v)),
					uint16(65535-(3*r+3*g+4*b)*65535/(10*v)),
				)
			}
		}
	}

	return t
}

// colourCube is an opaque 64 by 64 image of the 4096 colours of a 16 step grid on each channel.
func colourCube() Image {
	pix := make([]byte, 64*64*4)

	for i := range 64 * 64 {
		pix[i*4], pix[i*4+1], pix[i*4+2], pix[i*4+3] = uint8(i&15*17), uint8(i>>4&15*17), uint8(i>>8&15*17), 255
	}

	return Image{Pix: pix, W: 64, H: 64, Opaque: true}
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
	w(uint32(8))
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
		w(uint32(c.frame))
		w(uint32(c.fw))
		w(uint32(c.fh))
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

			w(uint8(blendOf(pl.Blend)))

			if m := pl.Mask; m == nil {
				w(uint8(0))
			} else {
				w(uint8(1))
				shape := uint8(0)
				if m.Shape == "ellipse" {
					shape = 1
				}
				w(shape)
				for _, v := range []float64{m.X, m.Y, m.Width, m.Height} {
					w(v)
				}
				w(uint32(m.Feather))
				invert := uint8(0)
				if m.Invert {
					invert = 1
				}
				w(invert)
			}

			flips := uint8(0)
			if pl.FlipX {
				flips |= 1
			}
			if pl.FlipY {
				flips |= 2
			}
			w(flips)

			w(uint8(len(pl.Effects)))

			for i := range pl.Effects {
				ef := &pl.Effects[i]
				w(effectCode(ef.Kind))

				names := timeline.EffectParams[ef.Kind]
				w(uint8(len(names)))

				for _, name := range names {
					w(effectParam(ef, name, 0))
				}
			}

			if t := pl.Transition; t == nil {
				w(uint8(0))
			} else {
				w(transitionCode(t.Kind))
				w(uint8(map[string]int{"in": 0, "out": 1}[t.Edge]))
				w(uint32(t.P))
				w(uint8(map[string]int{"": 0, "left": 0, "right": 1, "up": 2, "down": 3}[t.Direction]))
				col := parseColour(t.Color)
				buf.Write(col[:])
			}

			encodeGrade(&buf, pl.Grade)

			if pl.Lut == nil {
				w(uint8(0))
			} else {
				w(uint8(1))
				w(uint32(l.table.Size))
				w(pl.Lut.Intensity)
				w(l.table.Data)
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

// encodeGrade writes the grade of a layer: a flag, the eight HSL bands in the order of timeline.HslRanges (hue,
// saturation, lightness), the three wheels (a flag, hue, amount, luma) and the four curves (a count, then x and y).
func encodeGrade(buf *bytes.Buffer, g *plan.GradeSpec) {
	w := func(v any) { _ = binary.Write(buf, binary.LittleEndian, v) }

	if g == nil {
		w(uint8(0))
		return
	}

	w(uint8(1))

	for _, name := range timeline.HslRanges {
		var b timeline.HslBand

		for _, x := range g.Hsl {
			if x.Range == name {
				b = x
			}
		}

		w([3]float64{b.Hue, b.Saturation, b.Lightness})
	}

	var wheels [3]*timeline.Wheel
	if g.Wheels != nil {
		wheels = [3]*timeline.Wheel{g.Wheels.Shadows, g.Wheels.Midtones, g.Wheels.Highlights}
	}

	for _, wh := range wheels {
		if wh == nil {
			w(uint8(0))
			continue
		}

		w(uint8(1))
		w([3]float64{wh.Hue, wh.Amount, wh.Luma})
	}

	var curves [4][]timeline.CurvePoint
	if g.Curves != nil {
		curves = [4][]timeline.CurvePoint{g.Curves.Master, g.Curves.Red, g.Curves.Green, g.Curves.Blue}
	}

	for _, c := range curves {
		w(uint8(len(c)))

		for _, p := range c {
			w([2]float64{p.X, p.Y})
		}
	}
}
