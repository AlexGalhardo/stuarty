package compositor

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/lut"
)

func identityTable(n int) *lut.Table {
	t := &lut.Table{Size: n}

	for b := range n {
		for g := range n {
			for r := range n {
				for _, v := range []int{r, g, b} {
					t.Data = append(t.Data, uint16((v*65535+(n-1)/2)/(n-1)))
				}
			}
		}
	}

	return t
}

// A table on two points that is the identity gives back every grey exactly, and a finer identity table keeps every
// channel within one level.
func TestLUTIdentityTables(t *testing.T) {
	two := identityTable(2)
	for v := range int64(256) {
		if r, g, b := lutPixel(two, 1000, v, v, v); r != v || g != v || b != v {
			t.Fatalf("grey %d became %d %d %d", v, r, g, b)
		}
	}

	for _, n := range []int{3, 17, 33} {
		tb := identityTable(n)

		for v := int64(0); v < 256; v += 7 {
			in := [3]int64{v, 255 - v, v * 3 % 256}
			r, g, b := lutPixel(tb, 1000, in[0], in[1], in[2])

			for k, got := range [3]int64{r, g, b} {
				if d := got - in[k]; d < -1 || d > 1 {
					t.Fatalf("size %d, %v: channel %d is %d", n, in, k, got)
				}
			}
		}
	}
}

// The same pixels are pinned in renderer/stuarty-renderer/src/lut.rs.
func TestLUTInvertingTableAndMix(t *testing.T) {
	inv := &lut.Table{Size: 2}

	for i := range 8 {
		for _, bit := range []int{i & 1, i >> 1 & 1, i >> 2 & 1} {
			inv.Data = append(inv.Data, uint16(65535-bit*65535))
		}
	}

	for _, c := range []struct {
		im      int64
		in, out [3]int64
	}{
		{1000, [3]int64{10, 200, 255}, [3]int64{245, 55, 0}},
		{500, [3]int64{0, 200, 255}, [3]int64{128, 128, 128}},
		{0, [3]int64{10, 200, 255}, [3]int64{10, 200, 255}},
	} {
		r, g, b := lutPixel(inv, c.im, c.in[0], c.in[1], c.in[2])
		if got := [3]int64{r, g, b}; got != c.out {
			t.Errorf("mix %d of %v = %v, want %v", c.im, c.in, got, c.out)
		}
	}
}

func TestLUTMixHoldsTheIntensityToItsRange(t *testing.T) {
	nan := 0.0
	nan /= nan

	for in, want := range map[float64]int64{-3: 0, 0: 0, 0.6: 600, 0.0004: 0, 0.0005: 1, 7: 1000, nan: 0} {
		if got := lutMix(in); got != want {
			t.Errorf("lutMix(%v) = %d, want %d", in, got, want)
		}
	}
}

// BenchmarkApplyLUTVertical1080 is the cost of grading one 1080x1920 frame of noise with a 33 point table, on one band.
func BenchmarkApplyLUTVertical1080(b *testing.B) {
	const w, h = 1080, 1920

	src := Image{Pix: make([]byte, w*h*4), W: w, H: h, Opaque: true}
	// Noise: the worst case for the colour memory of the pass, no two neighbours alike.
	seed := uint32(1)
	for i := range src.Pix {
		seed = seed*1664525 + 1013904223
		src.Pix[i] = byte(seed >> 24)
	}

	for i := 3; i < len(src.Pix); i += 4 {
		src.Pix[i] = 255
	}

	tb := identityTable(33)
	c := &Compositor{Bands: 1}

	b.SetBytes(int64(len(src.Pix)))
	b.ResetTimer()

	for range b.N {
		c.applyLUT(src, tb, 800)
	}
}

// lutPixelPlain is the interpolation written out for one pixel, without the per-frame preparation: the reference
// that the prepared kernel must equal on every pixel.
func lutPixelPlain(t *lut.Table, im, r, g, b int64) [3]int64 {
	n := int64(t.Size)
	n1 := n - 1
	pr, pg, pb := r*n1, g*n1, b*n1
	r0, g0, b0 := pr/255, pg/255, pb/255
	fr, fg, fb := pr%255, pg%255, pb%255
	r1, g1, b1 := min(r0+1, n1), min(g0+1, n1), min(b0+1, n1)

	var sum [3]int64

	for i, w := range []int64{
		(255 - fr) * (255 - fg) * (255 - fb), fr * (255 - fg) * (255 - fb), (255 - fr) * fg * (255 - fb), fr * fg * (255 - fb),
		(255 - fr) * (255 - fg) * fb, fr * (255 - fg) * fb, (255 - fr) * fg * fb, fr * fg * fb,
	} {
		ri, gi, bi := r0, g0, b0
		if i&1 != 0 {
			ri = r1
		}

		if i&2 != 0 {
			gi = g1
		}

		if i&4 != 0 {
			bi = b1
		}

		o := 3 * (ri + n*(gi+n*bi))
		for k := range sum {
			sum[k] += w * int64(t.Data[o+int64(k)])
		}
	}

	in := [3]int64{r, g, b}

	var out [3]int64

	for k := range out {
		v := (sum[k] + lutDen/2) / lutDen
		out[k] = (in[k]*(1000-im) + v*im + 500) / 1000
	}

	return out
}

func TestLUTKernelEqualsThePlainInterpolation(t *testing.T) {
	seed := uint32(12345)
	next := func() uint32 {
		seed = seed*1664525 + 1013904223

		return seed >> 8
	}

	for _, n := range []int{2, 3, 17, 33, 65} {
		tb := &lut.Table{Size: n, Data: make([]uint16, 3*n*n*n)}
		for i := range tb.Data {
			tb.Data[i] = uint16(next())
		}

		for _, im := range []int64{1000, 800, 1} {
			k := newLUTKernel(tb, im)
			edge := []int64{0, 1, 127, 128, 254, 255}

			for _, r := range edge {
				for _, g := range edge {
					for _, b := range edge {
						x, y, z := k.pixel(r, g, b)
						if got, want := [3]int64{x, y, z}, lutPixelPlain(tb, im, r, g, b); got != want {
							t.Fatalf("size %d mix %d %d %d %d: %v, want %v", n, im, r, g, b, got, want)
						}
					}
				}
			}

			for range 20000 {
				r, g, b := int64(next()&255), int64(next()&255), int64(next()&255)
				x, y, z := k.pixel(r, g, b)

				if got, want := [3]int64{x, y, z}, lutPixelPlain(tb, im, r, g, b); got != want {
					t.Fatalf("size %d mix %d %d %d %d: %v, want %v", n, im, r, g, b, got, want)
				}
			}
		}
	}
}

// The memory of colours gives back what the function gives, for colours that collide and for black, whose slot starts
// empty.
func TestColourMemoEqualsTheFunction(t *testing.T) {
	fn := func(r, g, b int64) (int64, int64, int64) { return 255 - r, (g*3 + 1) % 256, b ^ 0x55 }

	var m colourMemo

	seed := uint32(7)

	for i := range 200000 {
		r, g, b := int64(i%2)*0, int64(i%3)*0, int64(0)
		if i%5 != 0 {
			seed = seed*1664525 + 1013904223
			r, g, b = int64(seed>>24), int64(seed>>16&255), int64(seed>>8&255)
		}

		x, y, z := m.pixel(fn, r, g, b)
		wx, wy, wz := fn(r, g, b)

		if x != wx || y != wy || z != wz {
			t.Fatalf("%d %d %d: %d %d %d, want %d %d %d", r, g, b, x, y, z, wx, wy, wz)
		}
	}
}
