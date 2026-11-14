package scan

import "testing"

func square(z *Rasterizer, x0, y0, x1, y1 float32) {
	z.MoveTo(x0, y0)
	z.LineTo(x1, y0)
	z.LineTo(x1, y1)
	z.LineTo(x0, y1)
	z.ClosePath()
}

func coverage(t *testing.T, z *Rasterizer, n int) []uint8 {
	t.Helper()

	dst := make([]uint8, n)
	if !z.Coverage(dst) {
		t.Fatal("Coverage reported a short buffer")
	}

	return dst
}

func TestSquareCoverage(t *testing.T) {
	t.Parallel()

	var z Rasterizer

	z.Reset(4, 4)
	square(&z, 1, 1, 3, 3)

	got := coverage(t, &z, 16)
	want := []uint8{
		0, 0, 0, 0,
		0, 255, 255, 0,
		0, 255, 255, 0,
		0, 0, 0, 0,
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("coverage = %v, want %v", got, want)
		}
	}
}

func TestPartialCoverage(t *testing.T) {
	t.Parallel()

	var z Rasterizer

	z.Reset(3, 1)
	square(&z, 0.5, 0, 2.5, 1)

	got := coverage(t, &z, 3)
	if got[0] < 127 || got[0] > 128 || got[1] != 255 || got[2] < 127 || got[2] > 128 {
		t.Errorf("coverage = %v, want half, full, half", got)
	}
}

// Polygons of opposite orientation cut a hole; of the same orientation they union.
func TestHoleAndUnion(t *testing.T) {
	t.Parallel()

	var z Rasterizer

	z.Reset(6, 6)
	square(&z, 0, 0, 6, 6)

	// The inner square goes the other way round.
	z.MoveTo(2, 2)
	z.LineTo(2, 4)
	z.LineTo(4, 4)
	z.LineTo(4, 2)
	z.ClosePath()

	got := coverage(t, &z, 36)
	if got[3*6+3] != 0 || got[0] != 255 || got[5*6+5] != 255 {
		t.Errorf("hole: centre %d, corners %d %d", got[3*6+3], got[0], got[5*6+5])
	}

	z.Reset(6, 6)
	square(&z, 0, 0, 4, 4)
	square(&z, 2, 2, 6, 6)

	got = coverage(t, &z, 36)
	if got[3*6+3] != 255 || got[0] != 255 || got[5*6+5] != 255 || got[5] != 0 {
		t.Errorf("union: centre %d, corners %d %d %d", got[3*6+3], got[0], got[5*6+5], got[5])
	}
}

func TestOutOfBoundsAndShortBuffer(t *testing.T) {
	t.Parallel()

	var z Rasterizer

	z.Reset(2, 2)
	square(&z, -10, -10, 10, 10)

	if got := coverage(t, &z, 4); got[0] != 255 || got[3] != 255 {
		t.Errorf("a square covering the canvas gives %v", got)
	}

	if z.Coverage(make([]uint8, 3)) {
		t.Error("Coverage accepted a buffer shorter than the canvas")
	}
}

func TestResetClears(t *testing.T) {
	t.Parallel()

	var z Rasterizer

	z.Reset(2, 2)
	square(&z, 0, 0, 2, 2)
	z.Reset(2, 2)

	for _, v := range coverage(t, &z, 4) {
		if v != 0 {
			t.Fatal("Reset left coverage behind")
		}
	}
}
