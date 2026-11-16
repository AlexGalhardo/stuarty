package audio

import (
	"math"
	"testing"
)

func TestDenoiseParamsAreMonotonicAndZeroIsOff(t *testing.T) {
	t.Parallel()

	for _, off := range []float64{0, -0.5, math.NaN(), 0.0001} {
		if _, _, ok := DenoiseParams(off); ok {
			t.Errorf("denoise %v asked for a filter", off)
		}
	}
	prevNR := 0.0
	for i := 1; i <= 1000; i++ {
		d := float64(i) / 1000
		nr, nf, ok := DenoiseParams(d)
		if !ok || nr < prevNR || nr < 0.01 || nr > MaxDenoiseNR || nf < -80 || nf > -20 {
			t.Fatalf("denoise %v: nr %v nf %v ok %v after %v", d, nr, nf, ok, prevNR)
		}
		prevNR = nr
	}
	nr, nf, _ := DenoiseParams(1)
	if nr != MaxDenoiseNR || nf != DenoiseNF {
		t.Errorf("denoise 1 gives %v dB and %v dBFS", nr, nf)
	}
	if nr2, nf2, _ := DenoiseParams(7); nr2 != nr || nf2 != nf {
		t.Errorf("a strength past 1 gives %v and %v", nr2, nf2)
	}
	if nr, nf, _ := DenoiseParams(0.5); nr != 15 || nf != DenoiseNF {
		t.Errorf("denoise 0.5 gives %v dB and %v dBFS", nr, nf)
	}
}
