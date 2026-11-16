package timeval

import (
	"math"
	"math/rand/v2"
	"testing"
)

var sampleFPS = []FPS{{24, 1}, {25, 1}, {30000, 1001}, {30, 1}, {60, 1}}

func ceilDivOld(a, b int64) int64 { return (a + b - 1) / b }

func TestFramesInsideAndCovering(t *testing.T) {
	t.Parallel()

	f := FPS{30, 1} // a frame is 1600 samples at 48 kHz

	if s, e := f.FramesInside(1000, 5000, 48000); s != 1 || e != 3 {
		t.Errorf("FramesInside = %d, %d; want 1, 3", s, e)
	}

	if s, e := f.FramesInside(100, 1500, 48000); e > s {
		t.Errorf("a range with no whole frame is not empty: %d, %d", s, e)
	}

	if s, e := f.FramesCovering(1000, 3300, 48000); s != 0 || e != 3 {
		t.Errorf("FramesCovering = %d, %d; want 0, 3", s, e)
	}

	if s, e := f.FramesCovering(1600, 3200, 48000); s != 1 || e != 2 {
		t.Errorf("an aligned range keeps its frames: %d, %d", s, e)
	}

	// 30000/1001: a frame is 1601.6 samples.
	if s, e := (FPS{30000, 1001}).FramesInside(0, 48048, 48000); s != 0 || e != 30 {
		t.Errorf("30000/1001: %d, %d", s, e)
	}

	for _, c := range []struct{ s, want int64 }{{0, 0}, {799, 0}, {800, 1}, {2399, 1}, {2400, 2}} {
		if got := f.FrameAtSample(c.s, 48000); got != c.want {
			t.Errorf("FrameAtSample(%d) = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestSampleRulesMatchTheOldIntegerFormulas(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(1, 2))

	for range 20000 {
		f := sampleFPS[rng.IntN(len(sampleFPS))]
		rate := []int64{44100, 48000}[rng.IntN(2)]
		a, b := rng.Int64N(1<<31), rng.Int64N(1<<31)
		per := rate * f.Den

		s, e := f.FramesInside(a, b, rate)
		if s != ceilDivOld(a*f.Num, per) || e != b*f.Num/per {
			t.Fatalf("FramesInside(%d,%d,%d) %v = %d,%d", a, b, rate, f, s, e)
		}

		s, e = f.FramesCovering(a, b, rate)
		if s != a*f.Num/per || e != ceilDivOld(b*f.Num, per) {
			t.Fatalf("FramesCovering(%d,%d,%d) %v = %d,%d", a, b, rate, f, s, e)
		}

		if got, want := f.FrameAtSample(a, rate), (2*a*f.Num+per)/(2*per); got != want {
			t.Fatalf("FrameAtSample(%d,%d) %v = %d, want %d", a, rate, f, got, want)
		}

		ms := rng.Int64N(1 << 31)
		if got, want := f.FrameAtMillis(ms), (2*ms*f.Num+1000*f.Den)/(2000*f.Den); got != want {
			t.Fatalf("FrameAtMillis(%d) %v = %d, want %d", ms, f, got, want)
		}
	}
}

func TestSampleRulesDoNotOverflow(t *testing.T) {
	t.Parallel()

	f := FPS{60, 1}
	s := int64(math.MaxInt64 / 2)

	// The int64 product s*60 would overflow; the exact result is s*60/48000.
	if got, want := f.FrameAtSample(s, 48000), s/800; got != want && got != want+1 {
		t.Errorf("FrameAtSample near MaxInt64/2 = %d, want about %d", got, want)
	}

	if a, b := f.FramesInside(s, s+1600, 48000); b < a {
		t.Errorf("FramesInside near MaxInt64/2 = %d, %d", a, b)
	}
}
