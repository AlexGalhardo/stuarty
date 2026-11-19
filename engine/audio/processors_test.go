package audio

import (
	"math"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// gainAt is the gain in dB of a chain at a frequency: the RMS of the second half of a second of a sine, output over input.
func gainAt(t *testing.T, freq float64, specs ...planSpec) float64 {
	t.Helper()

	in := sine(freq, 0.25, SampleRate)
	out := through(t, in, specs...)
	half := len(in) / 2

	return db(rms(out[half:]) / rms(in[half:]))
}

func TestEqualiser(t *testing.T) {
	t.Parallel()

	peak := spec(timeline.ProcEQ, "b3_hz", 1000, "b3_gain_db", 6, "b3_q", 1)
	if g := gainAt(t, 1000, peak); math.Abs(g-6) > 0.1 {
		t.Errorf("peaking band +6 dB at its centre gives %.3f dB", g)
	}

	if g := gainAt(t, 100, peak); math.Abs(g) > 0.3 {
		t.Errorf("a peaking band at 1 kHz changes 100 Hz by %.3f dB", g)
	}

	cut := spec(timeline.ProcEQ, "b2_hz", 250, "b2_gain_db", -12)
	if g := gainAt(t, 250, cut); math.Abs(g+12) > 0.1 {
		t.Errorf("band -12 dB at 250 Hz gives %.3f dB", g)
	}

	hp := spec(timeline.ProcEQ, "hp_hz", 100)
	if g := gainAt(t, 100, hp); math.Abs(g+3.01) > 0.1 {
		t.Errorf("high-pass at 100 Hz gives %.3f dB at 100 Hz, want -3.01", g)
	}

	if g := gainAt(t, 1000, hp); g < -0.1 {
		t.Errorf("high-pass at 100 Hz gives %.3f dB at 1 kHz", g)
	}

	// A second-order Butterworth falls 12 dB per octave: -24.1 dB two octaves down.
	if g := gainAt(t, 25, hp); math.Abs(g+24.1) > 1 {
		t.Errorf("high-pass at 100 Hz gives %.3f dB at 25 Hz, want about -24.1", g)
	}

	lp := spec(timeline.ProcEQ, "lp_hz", 4000)
	if g := gainAt(t, 4000, lp); math.Abs(g+3.01) > 0.1 {
		t.Errorf("low-pass at 4 kHz gives %.3f dB at 4 kHz, want -3.01", g)
	}

	if g := gainAt(t, 500, lp); g < -0.1 {
		t.Errorf("low-pass at 4 kHz gives %.3f dB at 500 Hz", g)
	}

	// With every parameter at its default the equaliser is not run: the signal is the same bits.
	in := sine(440, 0.3, 4800)
	out := through(t, in, spec(timeline.ProcEQ))

	for i := range in {
		if in[i] != out[i] {
			t.Fatalf("a flat equaliser changed sample %d", i)
		}
	}

	l, r := fixture(SampleRate / 4)
	chain := spec(
		timeline.ProcEQ,
		"hp_hz",
		80,
		"b2_hz",
		250,
		"b2_gain_db",
		-3,
		"b4_hz",
		3000,
		"b4_gain_db",
		2,
		"lp_hz",
		12000,
	)
	gl, gr := throughStereo(t, l, r, 0, chain)
	golden(t, "equaliser", gl, gr)
}

type planSpec = plan.ProcessorSpec

func steady(t *testing.T, in []float64, s planSpec) []float64 {
	t.Helper()

	return through(t, in, s)
}

func square(amp float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = amp
		if i/24%2 == 1 {
			out[i] = -amp
		}
	}

	return out
}

func TestCompressor(t *testing.T) {
	t.Parallel()

	// A constant level of -6 dBFS over a -18 dB threshold at 4:1 with a hard knee wants -15 dBFS: 9 dB less.
	c := spec(timeline.ProcCompressor, "threshold_db", -18, "ratio", 4, "knee_db", 0, "attack_ms", 5, "release_ms", 50)
	in := square(0.5, SampleRate)
	out := steady(t, in, c)

	if g := db(math.Abs(out[len(out)-1]) / 0.5); math.Abs(g+9) > 0.05 {
		t.Errorf("steady reduction %.3f dB, want -9", g)
	}

	// Below the threshold nothing changes.
	quiet := square(0.05, 4800)
	for i, v := range steady(t, quiet, c) {
		if v != quiet[i] {
			t.Fatalf("a level under the threshold changed sample %d", i)
		}
	}

	// The reduction follows the attack as a one-pole: after one time constant it has covered 1 - 1/e of its way.
	step := append(square(0.005, 4800), square(0.5, 4800)...)
	out = steady(t, step, c)
	at := 4800 + 5*SampleRate/1000
	got := db(math.Abs(out[at-1]) / 0.5)
	want := -9 * (1 - math.Exp(-1))

	if math.Abs(got-want) > 0.4 {
		t.Errorf("after one attack time the reduction is %.2f dB, want about %.2f", got, want)
	}

	// The release is slower: back from -9 dB, one release time later it has recovered 1 - 1/e of the way to 0.
	back := append(square(0.5, 12000), square(0.005, 12000)...)
	out = steady(t, back, c)
	at = 12000 + 50*SampleRate/1000
	got = db(math.Abs(out[at-1]) / 0.005)
	want = -9 * math.Exp(-1)

	if math.Abs(got-want) > 0.4 {
		t.Errorf("after one release time the reduction is %.2f dB, want about %.2f", got, want)
	}

	// Make-up gain.
	mk := spec(timeline.ProcCompressor, "threshold_db", 0, "makeup_db", 6)
	if g := gainAt(t, 1000, mk); math.Abs(g-6) > 0.05 {
		t.Errorf("make-up of 6 dB gives %.3f dB", g)
	}

	// The soft knee: at the threshold the reduction is (1/ratio - 1) (W/2)^2 / (2 W), and it is continuous at the edges.
	k := newCompressor(spec(timeline.ProcCompressor, "threshold_db", -20, "ratio", 4, "knee_db", 6).Params)
	if r := k.reduction(-20); math.Abs(r+0.5625) > 1e-9 {
		t.Errorf("reduction at the threshold with a 6 dB knee is %.4f, want -0.5625", r)
	}

	if lo, hi := k.reduction(-23), k.reduction(-23+1e-9); math.Abs(lo-hi) > 1e-6 {
		t.Errorf("the knee is not continuous at its lower edge: %v then %v", lo, hi)
	}

	if lo, hi := k.reduction(-17-1e-9), k.reduction(-17); math.Abs(lo-hi) > 1e-6 {
		t.Errorf("the knee is not continuous at its upper edge: %v then %v", lo, hi)
	}

	l, r := fixture(SampleRate / 4)
	gl, gr := throughStereo(t, l, r, 0, spec(timeline.ProcCompressor, "threshold_db", -24, "ratio", 4, "makeup_db", 3))
	golden(t, "compressor", gl, gr)
}

func TestLimiterNeverPassesTheCeiling(t *testing.T) {
	t.Parallel()

	lim := spec(timeline.ProcLimiter, "ceiling_db", -3, "release_ms", 20)
	ceiling := fromDB(-3)

	loud := add(sine(220, 1.2, SampleRate/2), sine(3100, 0.6, SampleRate/2))
	for i, v := range through(t, loud, lim) {
		if math.Abs(v) > ceiling+1e-12 {
			t.Fatalf("sample %d is %.6f over the ceiling %.6f", i, v, ceiling)
		}
	}

	// A signal under the ceiling is untouched.
	soft := sine(440, 0.2, 4800)
	for i, v := range through(t, soft, lim) {
		if v != soft[i] {
			t.Fatalf("a level under the ceiling changed sample %d", i)
		}
	}

	// After a peak the gain returns to 1 as a one-pole with the release time.
	burst := append(square(2, 480), square(0.1, 4800)...)
	out := through(t, burst, lim)
	g0 := math.Abs(out[479]) / 2 // the gain held by the peak
	at := 480 + 20*SampleRate/1000
	got := math.Abs(out[at-1]) / 0.1
	want := 1 - (1-g0)*math.Exp(-1)

	if math.Abs(got-want) > 0.02 {
		t.Errorf("one release time after the peak the gain is %.4f, want about %.4f", got, want)
	}

	l, r := fixture(SampleRate / 4)
	for i := range l {
		l[i] *= 4
		r[i] *= 4
	}

	gl, gr := throughStereo(t, l, r, 0, spec(timeline.ProcLimiter))
	golden(t, "limiter", gl, gr)
}

func TestGate(t *testing.T) {
	t.Parallel()

	g := spec(timeline.ProcGate, "threshold_db", -50, "range_db", -40, "attack_ms", 2, "hold_ms", 50, "release_ms", 150)

	// Under the threshold the gate is closed: the signal falls by the range.
	hiss := square(0.001, SampleRate)
	out := through(t, hiss, g)

	if got := db(math.Abs(out[len(out)-1]) / 0.001); math.Abs(got+40) > 0.05 {
		t.Errorf("a closed gate lowers by %.3f dB, want -40", got)
	}

	// Over it, the gate is open and the signal passes.
	loud := square(0.1, SampleRate)
	out = through(t, loud, g)

	if got := db(math.Abs(out[len(out)-1]) / 0.1); math.Abs(got) > 0.01 {
		t.Errorf("an open gate changes the level by %.3f dB", got)
	}

	// It stays open for the hold time after the level falls under the threshold, then closes with the release.
	sig := append(square(0.1, 24000), square(0.001, 48000)...)
	out = through(t, sig, g)
	closed := -1

	for i := 24000; i < len(out); i++ {
		if math.Abs(out[i])/0.001 < 0.5 {
			closed = i - 24000

			break
		}
	}

	hold := 50 * SampleRate / 1000
	if closed < hold || closed > hold+5000+SampleRate*150/1000 {
		t.Errorf("the gate fell under half after %d samples; hold is %d", closed, hold)
	}

	// The attack: a gate that opens does so with the attack time constant.
	open := append(square(0.001, 24000), square(0.1, 4800)...)
	out = through(t, open, g)
	at := 24000 + 2*SampleRate/1000
	got := math.Abs(out[at-1]) / 0.1
	floor := fromDB(-40)

	want := floor + (1-floor)*(1-math.Exp(-1))
	if math.Abs(got-want) > 0.05 {
		t.Errorf("after one attack time the gain is %.3f, want about %.3f", got, want)
	}

	l, r := fixture(SampleRate / 4)
	gl, gr := throughStereo(t, l, r, 0, spec(timeline.ProcGate, "threshold_db", -30, "range_db", -30))
	golden(t, "gate", gl, gr)
}

func TestDeEsser(t *testing.T) {
	t.Parallel()

	d := spec(timeline.ProcDeEsser, "frequency_hz", 6500, "threshold_db", -30, "ratio", 4)

	// A band level of -10.5 dBFS is 19.5 dB over the threshold: 14.6 dB of reduction at a ratio of 4.
	in := sine(6500, 0.3, SampleRate)
	out := through(t, in, d)
	half := len(in) / 2

	if g := db(rms(out[half:]) / rms(in[half:])); math.Abs(g+14.6) > 1 {
		t.Errorf("a sibilant tone is lowered by %.2f dB, want about -14.6", g)
	}

	// A tone far from the band is not touched.
	low := sine(500, 0.3, SampleRate)
	out = through(t, low, d)

	if g := db(rms(out[half:]) / rms(low[half:])); math.Abs(g) > 0.3 {
		t.Errorf("a tone at 500 Hz is changed by %.2f dB", g)
	}

	// Under the threshold nothing happens.
	soft := sine(6500, 0.01, SampleRate)
	out = through(t, soft, d)

	if g := db(rms(out[half:]) / rms(soft[half:])); math.Abs(g) > 0.2 {
		t.Errorf("a quiet sibilant tone is changed by %.2f dB", g)
	}

	l, r := fixture(SampleRate / 4)
	gl, gr := throughStereo(t, l, r, 0, d)
	golden(t, "deesser", gl, gr)
}

// The result does not depend on how the stream is cut into calls: a whole chain, the spectral stage included, is the
// same bits in one call and in calls of 1,000 and 7 frames.
func TestChainIsBlockSizeIndependent(t *testing.T) {
	t.Parallel()

	specs := []planSpec{
		spec(timeline.ProcNoiseReduction, "strength", 0.6),
		spec(timeline.ProcEQ, "hp_hz", 80, "b4_hz", 3000, "b4_gain_db", 2),
		spec(timeline.ProcDeEsser),
		spec(timeline.ProcGate, "threshold_db", -40),
		spec(timeline.ProcCompressor, "threshold_db", -24),
		spec(timeline.ProcLimiter),
	}

	l, r := fixture(30000)
	wantL, wantR := throughStereo(t, l, r, 0, specs...)

	for _, call := range []int{1000, 7} {
		gotL, gotR := throughStereo(t, l, r, call, specs...)

		for i := range wantL {
			if gotL[i] != wantL[i] || gotR[i] != wantR[i] {
				t.Fatalf("calls of %d frames differ at sample %d", call, i)
			}
		}
	}
}
