package timeval

import (
	"math"
	"math/rand"
	"testing"
)

var sampleFPS = []FPS{{24, 1}, {25, 1}, {30000, 1001}, {30, 1}, {60, 1}}

func ceilDivOld(a, b int64) int64 { return (a + b - 1) / b }

func TestSampleHelpersTable(t *testing.T) {
	f := FPS{30, 1}
	const rate = 48000 // 1600 samples per frame
	cases := []struct {
		s0, s1                        int64
		inS, inE, covS, covE, nearest int64
	}{
		{0, 3200, 0, 2, 0, 2, 0},
		{1, 3199, 1, 1, 0, 2, 0},
		{800, 2399, 1, 1, 0, 2, 1},
		{799, 2400, 1, 1, 0, 2, 0},
	}
	for _, c := range cases {
		s, e := f.FramesInside(c.s0, c.s1, rate)
		if s != c.inS || e != c.inE {
			t.Errorf("inside(%d,%d) = %d,%d", c.s0, c.s1, s, e)
		}
		s, e = f.FramesCovering(c.s0, c.s1, rate)
		if s != c.covS || e != c.covE {
			t.Errorf("covering(%d,%d) = %d,%d", c.s0, c.s1, s, e)
		}
		if got := f.FrameAtSample(c.s0, rate); got != c.nearest {
			t.Errorf("nearest(%d) = %d, want %d", c.s0, got, c.nearest)
		}
	}
}

func TestSampleHelpersMatchTheOldFormulas(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	for range 20000 {
		f := sampleFPS[rng.Intn(len(sampleFPS))]
		rate := []int64{44100, 48000}[rng.Intn(2)]
		a, b := rng.Int63n(1<<31), rng.Int63n(1<<31)
		num, den := f.Num, f.Den

		s, e := f.FramesInside(a, b, rate)
		if s != ceilDivOld(a*num, rate*den) || e != b*num/(rate*den) {
			t.Fatalf("inside %v %d %d", f, a, b)
		}
		s, e = f.FramesCovering(a, b, rate)
		if s != a*num/(rate*den) || e != ceilDivOld(b*num, rate*den) {
			t.Fatalf("covering %v %d %d", f, a, b)
		}
		per := rate * den
		if got, want := f.FrameAtSample(a, rate), (2*a*num+per)/(2*per); got != want {
			t.Fatalf("nearest %v %d: %d != %d", f, a, got, want)
		}
	}
}

func TestFrameAtMillisMatchesTheWordBoundary(t *testing.T) {
	rng := rand.New(rand.NewSource(14))
	for range 20000 {
		f := sampleFPS[rng.Intn(len(sampleFPS))]
		ms := rng.Int63n(1 << 31)
		if got, want := f.FrameAtMillis(ms), (2*ms*f.Num+1000*f.Den)/(2000*f.Den); got != want {
			t.Fatalf("%v %d: %d != %d", f, ms, got, want)
		}
	}
}

func TestSampleHelpersDoNotOverflow(t *testing.T) {
	f := FPS{60, 1}
	s := int64(math.MaxInt64 / 2)
	if got, want := f.FrameAtSample(s, 48000), s/800; got < want || got > want+1 {
		t.Fatalf("got %d, want about %d", got, want)
	}
	a, b := f.FramesCovering(s, s+1, 48000)
	if a != s/800 || b != s/800+1 {
		t.Fatalf("covering = %d,%d", a, b)
	}
}
