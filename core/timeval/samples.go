package timeval

import "math/big"

// The rules that place an analysis result (silences, speech, beats) on project
// frames. Each one is a named rounding rule on exact integer arithmetic, so a
// long input at a high sample rate cannot overflow int64 on the way.

// frameRatio returns the exact number of frames in s samples at sampleRate as
// the fraction n/d, where n = s*Num and d = sampleRate*Den.
func (f FPS) frameRatio(s, sampleRate int64) (n, d *big.Int) {
	n = new(big.Int).Mul(big.NewInt(s), big.NewInt(f.Num))
	d = new(big.Int).Mul(big.NewInt(sampleRate), big.NewInt(f.Den))

	return n, d
}

// floorSamples is the frame count of s samples, rounded down.
func (f FPS) floorSamples(s, sampleRate int64) int64 {
	n, d := f.frameRatio(s, sampleRate)

	return n.Div(n, d).Int64()
}

// ceilSamples is the frame count of s samples, rounded up.
func (f FPS) ceilSamples(s, sampleRate int64) int64 {
	n, d := f.frameRatio(s, sampleRate)
	n.Add(n, d).Sub(n, big.NewInt(1))

	return n.Div(n, d).Int64()
}

// FramesInside is the frame range [start, end) that lies entirely inside the
// sample range [s0, s1) at sampleRate: the start rounds up and the end rounds
// down. The range is empty (end <= start) when no whole frame fits.
func (f FPS) FramesInside(s0, s1, sampleRate int64) (start, end int64) {
	return f.ceilSamples(s0, sampleRate), f.floorSamples(s1, sampleRate)
}

// FramesCovering is the smallest frame range that covers the sample range
// [s0, s1) at sampleRate: the start rounds down and the end rounds up.
func (f FPS) FramesCovering(s0, s1, sampleRate int64) (start, end int64) {
	return f.floorSamples(s0, sampleRate), f.ceilSamples(s1, sampleRate)
}

// FrameAtSample is the frame nearest to sample s at sampleRate, halves rounding up.
func (f FPS) FrameAtSample(s, sampleRate int64) int64 {
	n, d := f.frameRatio(s, sampleRate)
	n.Lsh(n, 1).Add(n, d)
	d.Lsh(d, 1)

	return n.Div(n, d).Int64()
}
