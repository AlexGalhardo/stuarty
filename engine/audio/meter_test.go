package audio

import (
	"math"
	"testing"
)

func interleave(l, r []float64) []float32 {
	out := make([]float32, 0, 2*len(l))
	for i := range l {
		out = append(out, float32(l[i]), float32(r[i]))
	}

	return out
}

func measure(l, r []float64) Measurement {
	m := NewMeter()
	m.Write(interleave(l, r))

	return m.Result()
}

func TestMeterLoudnessOfASine(t *testing.T) {
	t.Parallel()

	// A 997 Hz sine at -20 dBFS on both channels is -20 LUFS (ITU-R BS.1770-4, the offset of -0.691 is that of the
	// K-weighting at 1 kHz).
	x := sine(997, 0.1, 5*SampleRate)
	got := measure(x, x)

	if got.IntegratedLUFS == nil || math.Abs(*got.IntegratedLUFS+20) > 0.15 {
		t.Fatalf("integrated loudness %v, want -20 LUFS", got.IntegratedLUFS)
	}

	// Twice the amplitude is 6.02 dB more.
	y := sine(997, 0.2, 5*SampleRate)
	got2 := measure(y, y)

	if d := *got2.IntegratedLUFS - *got.IntegratedLUFS; math.Abs(d-6.02) > 0.01 {
		t.Errorf("doubling the level adds %.3f LU", d)
	}

	// Silence under the absolute gate has no loudness; a mix shorter than 400 ms has none either.
	zero := make([]float64, 3*SampleRate)
	if m := measure(
		zero,
		zero,
	); m.IntegratedLUFS != nil || m.TruePeakDBTP != nil ||
		m.NoiseFloorDBFS != MinNoiseFloorDBFS {
		t.Errorf("silence measures %+v", m)
	}

	if m := measure(x[:SampleRate/4], x[:SampleRate/4]); m.IntegratedLUFS != nil {
		t.Errorf("0.25 s has a loudness: %v", *m.IntegratedLUFS)
	}
}

// The gates: a loud passage with a long very quiet one is measured on the loud one only.
func TestMeterGates(t *testing.T) {
	t.Parallel()

	loud := sine(997, 0.1, 4*SampleRate)
	quiet := sine(997, 0.1*math.Pow(10, -25.0/20), 8*SampleRate)
	both := append(append([]float64{}, loud...), quiet...)

	got := measure(both, both)
	if got.IntegratedLUFS == nil || math.Abs(*got.IntegratedLUFS+20) > 0.3 {
		t.Errorf(
			"integrated loudness %v, want about -20 LUFS (the quiet part is under the relative gate)",
			got.IntegratedLUFS,
		)
	}
}

func TestMeterTruePeak(t *testing.T) {
	t.Parallel()

	// A sine at a quarter of the sampling rate with a phase of 45 degrees has samples of 0.707 of its peak: the true
	// peak is the 0.5.
	x := make([]float64, 4800)
	for i := range x {
		x[i] = 0.5 * math.Sin(math.Pi/2*float64(i)+math.Pi/4)
	}

	got := measure(x, x)
	if got.TruePeakDBTP == nil || math.Abs(*got.TruePeakDBTP-db(0.5)) > 0.3 {
		t.Fatalf("true peak %v, want %.2f dBTP", got.TruePeakDBTP, db(0.5))
	}

	if sample := db(0.5 * math.Sqrt2 / 2); *got.TruePeakDBTP < sample+2 {
		t.Errorf("the true peak %.2f is not above the sample peak %.2f", *got.TruePeakDBTP, sample)
	}
}

func TestMeterNoiseFloor(t *testing.T) {
	t.Parallel()

	// 4 s of noise at -60 dBFS with a loud tone over a third of it: the floor is that of the noise.
	n := 4 * SampleRate
	sig := noise(5, 0.001, n)

	for i := n / 3; i < 2*n/3; i++ {
		sig[i] += 0.2 * math.Sin(2*math.Pi*440*float64(i)/SampleRate)
	}

	got := measure(sig, sig)
	if math.Abs(got.NoiseFloorDBFS+60) > 1 {
		t.Errorf("noise floor %.2f dBFS, want about -60", got.NoiseFloorDBFS)
	}

	if got.SpeechToNoiseDB == nil || *got.SpeechToNoiseDB < 30 {
		t.Errorf("speech to noise %v", got.SpeechToNoiseDB)
	}
}
