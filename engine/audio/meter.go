package audio

import (
	"math"
	"slices"
)

// Measurement is what the meter says about a stretch of audio (SDD-004 section 7.1).
type Measurement struct {
	// IntegratedLUFS is the integrated loudness of ITU-R BS.1770-4; nil for audio with no block above the absolute
	// gate (silence, or less than 0.4 s).
	IntegratedLUFS *float64 `json:"integrated_lufs"`
	// TruePeakDBTP is the highest sample after 4 times oversampling, in dB; nil for digital silence.
	TruePeakDBTP *float64 `json:"true_peak_dbtp"`
	// NoiseFloorDBFS is the mean-square level of the 10th percentile of the 100 ms blocks, in dBFS: -120 for digital
	// silence or less than 100 ms.
	NoiseFloorDBFS float64 `json:"noise_floor_dbfs"`
	// SpeechToNoiseDB is the integrated loudness minus the noise floor; nil when there is no loudness.
	SpeechToNoiseDB *float64 `json:"speech_to_noise_db"`
}

const (
	meterBlock       = SampleRate / 10 // a hop of the loudness gate and a block of the noise floor: 100 ms
	meterGateBlocks  = 4               // hops in a 400 ms block
	absoluteGateLUFS = -70.0
	relativeGateLU   = -10.0
	lufsOffset       = -0.691
	// MinNoiseFloorDBFS is the floor that digital silence reports.
	MinNoiseFloorDBFS = -120.0
	// truePeakPhases is the oversampling of the true peak, and truePeakTaps the taps of each phase.
	truePeakPhases = 4
	truePeakTaps   = 12
)

// kWeighting is the two filters of ITU-R BS.1770-4 at 48 kHz: the high shelf and the high-pass.
func kWeighting() [2]*biquad {
	return [2]*biquad{
		{
			b0: 1.53512485958697,
			b1: -2.69169618940638,
			b2: 1.19839281085285,
			a1: -1.69065929318241,
			a2: 0.73248077421585,
		},
		{b0: 1, b1: -2, b2: 1, a1: -1.99004745483398, a2: 0.99007225036621},
	}
}

// Meter measures interleaved stereo float32 blocks as they come, so a measurement does not hold the audio.
type Meter struct {
	k [2][2]*biquad // [stage][channel]; the filters hold the state of both channels

	left     int     // samples of the current 100 ms block
	weighted float64 // sum of the K-weighted squares of both channels in the block
	raw      float64 // sum of the squares of both channels in the block

	hops  []float64 // the weighted sum of each finished 100 ms block
	raws  []float64 // the raw sum of each finished 100 ms block
	peak  float64
	hist  [2][truePeakTaps]float64
	phase [truePeakPhases][truePeakTaps]float64
}

// NewMeter is a meter at the start of a stretch of audio.
func NewMeter() *Meter {
	m := &Meter{}
	for c := range 2 {
		ks := kWeighting()
		m.k[0][c], m.k[1][c] = ks[0], ks[1]
	}

	for p := 1; p < truePeakPhases; p++ {
		f := float64(p) / truePeakPhases

		for j := range truePeakTaps {
			d := float64(truePeakTaps/2) - float64(j) - f
			m.phase[p][j] = sinc(d) * (0.5 + 0.5*math.Cos(math.Pi*d/float64(truePeakTaps/2)))
		}
	}

	return m
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}

	return math.Sin(math.Pi*x) / (math.Pi * x)
}

// Write adds a block of interleaved stereo frames.
func (m *Meter) Write(block []float32) {
	for i := 0; i+1 < len(block); i += 2 {
		for c := range 2 {
			x := float64(block[i+c])
			m.raw += mul(x, x)
			y := m.k[1][c].run(c, m.k[0][c].run(c, x))
			m.weighted += mul(y, y)
			m.truePeak(c, x)
		}

		m.left++

		if m.left == meterBlock {
			m.hops = append(m.hops, m.weighted)
			m.raws = append(m.raws, m.raw)
			m.left, m.weighted, m.raw = 0, 0, 0
		}
	}
}

// truePeak keeps the highest value of the sample and of the oversampled points between it and the one before.
func (m *Meter) truePeak(c int, x float64) {
	copy(m.hist[c][1:], m.hist[c][:truePeakTaps-1])
	m.hist[c][0] = x
	m.peak = math.Max(m.peak, math.Abs(x))

	for p := 1; p < truePeakPhases; p++ {
		var y float64

		for j, h := range m.phase[p] {
			y += mul(m.hist[c][j], h)
		}

		m.peak = math.Max(m.peak, math.Abs(y))
	}
}

// Result is the measurement of everything written so far.
func (m *Meter) Result() Measurement {
	out := Measurement{NoiseFloorDBFS: MinNoiseFloorDBFS}

	if m.peak > 0 {
		p := 20 * math.Log10(m.peak)
		out.TruePeakDBTP = &p
	}

	if n := len(m.raws); n > 0 {
		sorted := slices.Sorted(slices.Values(m.raws))
		ms := sorted[(n-1)/10] / float64(2*meterBlock)

		if ms > 0 {
			out.NoiseFloorDBFS = math.Max(10*math.Log10(ms), MinNoiseFloorDBFS)
		}
	}

	if l := m.integrated(); l != nil {
		out.IntegratedLUFS = l
		s := *l - out.NoiseFloorDBFS
		out.SpeechToNoiseDB = &s
	}

	return out
}

// integrated is the gated loudness of the 400 ms blocks taken every 100 ms.
func (m *Meter) integrated() *float64 {
	var blocks []float64

	for i := 0; i+meterGateBlocks <= len(m.hops); i++ {
		var z float64
		for _, h := range m.hops[i : i+meterGateBlocks] {
			z += h
		}

		blocks = append(blocks, z/float64(meterGateBlocks*meterBlock))
	}

	loud := func(z float64) float64 { return lufsOffset + 10*math.Log10(z) }
	mean := func(gate float64) (float64, int) {
		var sum float64

		n := 0

		for _, z := range blocks {
			if z > 0 && loud(z) > gate {
				sum += z
				n++
			}
		}

		return sum, n
	}

	sum, n := mean(absoluteGateLUFS)
	if n == 0 {
		return nil
	}

	sum, n = mean(loud(sum/float64(n)) + relativeGateLU)
	if n == 0 {
		return nil
	}

	l := loud(sum / float64(n))

	return &l
}
