package audio

import "math"

const (
	// PeakCeilingDBTP is the true peak the mix may not pass after the gain (SDD-004 section 7). It
	// leaves room for the overshoot of the AAC encoder.
	PeakCeilingDBTP = -1.5
	// MinMeasuredSamples is 0.4 s: a mix shorter than that is not measured (a loudness meter needs a
	// 400 ms block), and gets no gain.
	MinMeasuredSamples = SampleRate * 4 / 10
)

// GainDB is the one gain, in decibels, that takes a mix of samples frames with the measured integrated
// loudness (LUFS) and true peak (dBTP) to the target loudness, reduced when it would push the true
// peak above PeakCeilingDBTP. A silent mix (a nil measurement) or one shorter than 0.4 s gets 0.
func GainDB(integratedLUFS, truePeakDBTP *float64, samples int64, targetLUFS float64) float64 {
	if integratedLUFS == nil || truePeakDBTP == nil || samples < MinMeasuredSamples {
		return 0
	}
	if math.IsInf(*integratedLUFS, 0) || math.IsInf(*truePeakDBTP, 0) {
		return 0
	}
	return math.Min(targetLUFS-*integratedLUFS, PeakCeilingDBTP-*truePeakDBTP)
}
