package compositor

import (
	"testing"

	"github.com/AlexGalhardo/vukem-video/engine/lut"
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

// The same pixels are pinned in renderer/vukem-renderer/src/lut.rs.
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
