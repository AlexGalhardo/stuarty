package audio

import (
	"math"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const (
	nrNoiseRMS = 0.003 // about -50 dBFS
	nrToneAmp  = 0.1   // -20 dBFS
)

// speechLike is the noisy fixture: seconds of white noise at about -50 dBFS under bursts of a tone with harmonics,
// 0.4 s on and 0.4 s off, starting with half a second of noise alone.
func speechLike(seconds float64) (sig []float64, bursts [][2]int) {
	n := int(seconds * SampleRate)
	voice := make([]float64, n)

	for at := SampleRate / 2; at+19200 <= n; at += 38400 {
		for i := range 19200 {
			t := float64(at+i) / SampleRate
			env := math.Min(1, math.Min(float64(i), float64(19200-i))/480)
			voice[at+i] = env * nrToneAmp * (math.Sin(2*math.Pi*220*t) + 0.6*math.Sin(2*math.Pi*660*t) + 0.4*math.Sin(2*math.Pi*1320*t)) / 1.5
		}

		bursts = append(bursts, [2]int{at, at + 19200})
	}

	return add(voice, noise(11, nrNoiseRMS, n)), bursts
}

// pause is a stretch between two bursts, away from the edges.
func pause(bursts [][2]int, k int) (from, to int) {
	return bursts[k][1] + 4800, bursts[k+1][0] - 4800
}

func TestNoiseReduction(t *testing.T) {
	t.Parallel()

	sig, bursts := speechLike(4)
	nr := spec(timeline.ProcNoiseReduction, "strength", 0.5)
	out := through(t, sig, nr)

	// The noise falls by a stated amount in a pause after the estimate has settled: at least 12 dB at a strength of
	// 0.5 (the floor of that strength is 18 dB).
	a, b := pause(bursts, 2)
	before, after := rms(sig[a:b]), rms(out[a:b])

	if got := db(after / before); got > -12 {
		t.Errorf("the noise fell by %.1f dB, want at least 12", -got)
	} else {
		t.Logf("noise fell by %.1f dB at strength 0.5", -got)
	}

	// The speech is kept: the level of the middle of a burst, tone and harmonics, within 1 dB.
	m0, m1 := bursts[2][0]+4800, bursts[2][1]-4800
	if got := db(rms(out[m0:m1]) / rms(sig[m0:m1])); math.Abs(got) > 1 {
		t.Errorf("the speech changed by %.2f dB", got)
	}

	// A stronger setting takes more noise and a weaker one less.
	strong := through(t, sig, spec(timeline.ProcNoiseReduction, "strength", 1))
	weak := through(t, sig, spec(timeline.ProcNoiseReduction, "strength", 0.2))

	if s, w := rms(strong[a:b]), rms(weak[a:b]); s >= w {
		t.Errorf("strength 1 leaves %.5f, strength 0.2 leaves %.5f", s, w)
	}

	l, r := make([]float64, SampleRate), make([]float64, SampleRate)
	copy(l, sig[:SampleRate])
	copy(r, sig[:SampleRate])
	gl, gr := throughStereo(t, l, r, 0, nr)
	golden(t, "noise_reduction", gl, gr)
}

// A marked profile is measured from the range of the asset and is in force from the first sample.
func TestNoiseReductionMarkedProfile(t *testing.T) {
	t.Parallel()

	sig, bursts := speechLike(4)

	// The first 0.4 s are noise alone.
	prof := measureNoiseProfile(sig[:19200], sig[:19200])
	if len(prof) != nrBins {
		t.Fatalf("a profile of %d bins", len(prof))
	}

	chain, err := NewChain([]planSpec{spec(timeline.ProcNoiseReduction, "strength", 0.5)}, prof)
	if err != nil {
		t.Fatal(err)
	}

	out, _ := runChain(t, chain, sig, sig, 0)

	// The noise is down already in the first 0.4 s, which an estimate has not settled for.
	if got := db(rms(out[4800:19200]) / rms(sig[4800:19200])); got > -12 {
		t.Errorf("with a marked profile the noise fell by %.1f dB in the first 0.4 s, want at least 12", -got)
	}

	m0, m1 := bursts[1][0]+4800, bursts[1][1]-4800
	if got := db(rms(out[m0:m1]) / rms(sig[m0:m1])); math.Abs(got) > 1 {
		t.Errorf("the speech changed by %.2f dB", got)
	}

	if measureNoiseProfile(sig[:500], sig[:500]) != nil {
		t.Error("a range shorter than a frame has a profile")
	}
}

// The stage delays its output, the chain compensates: a burst after silence starts where it started.
func TestNoiseReductionKeepsTheTiming(t *testing.T) {
	t.Parallel()

	const at = 24000

	sig := make([]float64, 48000)
	copy(sig[at:], sine(1000, 0.5, 9600))

	out := through(t, sig, spec(timeline.ProcNoiseReduction, "strength", 0.3))

	first := func(x []float64) int {
		for i, v := range x {
			if math.Abs(v) > 0.2 {
				return i
			}
		}

		return -1
	}

	if got, want := first(out), first(sig); math.Abs(float64(got-want)) > 200 {
		t.Errorf("the burst starts at %d, want %d", got, want)
	}

	if (&noiseReduction{}).latency() != nrFrame {
		t.Error("the latency is not the frame")
	}
}

func TestFFTRoundTripAndPeak(t *testing.T) {
	t.Parallel()

	f := newFFT(nrFrame)
	re, im := make([]float64, nrFrame), make([]float64, nrFrame)
	x := add(sine(750, 1, nrFrame), noise(3, 0.1, nrFrame))
	copy(re, x)

	f.transform(re, im, false)

	// 750 Hz is bin 750 x 1024 / 48000 = 16.
	best := 0
	for k := range nrBins {
		if re[k]*re[k]+im[k]*im[k] > re[best]*re[best]+im[best]*im[best] {
			best = k
		}
	}

	if best != 16 {
		t.Errorf("the peak is bin %d, want 16", best)
	}

	f.transform(re, im, true)

	for i := range x {
		if math.Abs(re[i]-x[i]) > 1e-12 {
			t.Fatalf("sample %d came back as %v, was %v", i, re[i], x[i])
		}
	}
}
