package raster

import (
	"math"
	"testing"
)

func TestSincosAgreesWithTheStandardLibrary(t *testing.T) {
	t.Parallel()

	for i := -400; i <= 400; i++ {
		x := float64(i) * 0.0173
		s, c := sincos(x)
		ws, wc := math.Sincos(x)

		if math.Abs(s-ws) > 1e-12 || math.Abs(c-wc) > 1e-12 {
			t.Fatalf("sincos(%v) = %v, %v, want %v, %v", x, s, c, ws, wc)
		}
	}
}

func TestAtan2AgreesWithTheStandardLibrary(t *testing.T) {
	t.Parallel()

	for yi := -12; yi <= 12; yi++ {
		for xi := -12; xi <= 12; xi++ {
			y, x := float64(yi)*0.37, float64(xi)*0.41

			if got, want := atan2(y, x), math.Atan2(y, x); math.Abs(got-want) > 1e-12 {
				t.Fatalf("atan2(%v, %v) = %v, want %v", y, x, got, want)
			}
		}
	}
}

func TestHypotAgreesWithTheStandardLibrary(t *testing.T) {
	t.Parallel()

	for i := 1; i < 200; i++ {
		x, y := float64(i)*0.713, float64(200-i)*1.37
		if got, want := hypot(x, y), math.Hypot(x, y); math.Abs(got-want) > 1e-12*want {
			t.Fatalf("hypot(%v, %v) = %v, want %v", x, y, got, want)
		}
	}
}

// TestDeterministicBits pins the exact bits of the helpers. It runs on every
// architecture in CI: if one of them fused an operation or took another path,
// the bits would differ and this test would fail there, not only the goldens.
func TestDeterministicBits(t *testing.T) {
	t.Parallel()

	s, c := sincos(0.7)
	cases := []struct {
		name string
		got  float64
		bits uint64
	}{
		{"sin 0.7", s, 0x3fe49d6e694619b8},
		{"cos 0.7", c, 0x3fe87996529f9d92},
		{"atan2 1.3 -0.4", atan2(1.3, -0.4), 0x3ffde8a225150890},
		{"hypot 3.3 4.1", hypot(3.3, 4.1), 0x40150d649142ae73},
	}

	for _, tc := range cases {
		if got := math.Float64bits(tc.got); got != tc.bits {
			t.Errorf("%s: bits %#x (%v), want %#x", tc.name, got, tc.got, tc.bits)
		}
	}
}

// TestProductsAreNotFused uses values for which a fused multiply-add and a
// product rounded first give different results, and asserts the rounded
// product: the helpers that feed the geometry must never fuse.
func TestProductsAreNotFused(t *testing.T) {
	t.Parallel()

	// Found by search: fma(a, b, c) != a*b + c.
	a, b, c := 1.000000001, 1.0000000031, -1.0000000041000001

	if math.FMA(a, b, c) == float64(a*b)+c {
		t.Fatal("the test values do not tell fused from unfused: pick others")
	}

	if got, want := c+mul(a, b), float64(a*b)+c; got != want {
		t.Errorf("c + mul(a, b) = %v, want the unfused %v", got, want)
	}

	// lerp is a + t*(b-a): the product must be rounded before the sum.
	la, lb, lt := -1.000000001, 0.0, 1.0000000031

	if math.FMA(lt, lb-la, la) == la+float64(lt*(lb-la)) {
		t.Fatal("the lerp values do not tell fused from unfused")
	}

	if got, want := lerp(la, lb, lt), la+float64(lt*(lb-la)); got != want {
		t.Errorf("lerp = %v, want the unfused %v", got, want)
	}
}
