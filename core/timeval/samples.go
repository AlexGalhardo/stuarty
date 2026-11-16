package timeval

import "math/big"

// The helpers below place analysis results (sample positions of silences, speech
// and beats) on project frames. They use big.Int so long, high-rate inputs never
// overflow, and they assume non-negative positions and a positive sampleRate.

// sampleFrame returns s*Num/(sampleRate*Den) as a big.Int quotient and remainder
// (floor division for non-negative s).
func (f FPS) sampleFrame(s, sampleRate int64) (q *big.Int, exact bool) {
	n := new(big.Int).Mul(big.NewInt(s), big.NewInt(f.Num))
	den := new(big.Int).Mul(big.NewInt(sampleRate), big.NewInt(f.Den))
	q, r := n.DivMod(n, den, new(big.Int))
	return q, r.Sign() == 0
}

func (f FPS) sampleFloor(s, sampleRate int64) int64 {
	q, _ := f.sampleFrame(s, sampleRate)
	return q.Int64()
}

func (f FPS) sampleCeil(s, sampleRate int64) int64 {
	q, exact := f.sampleFrame(s, sampleRate)
	if !exact {
		q.Add(q, big.NewInt(1))
	}
	return q.Int64()
}

// FramesInside is the frame range [start, end) lying entirely inside the sample
// range [s0, s1) at sampleRate: start rounded up, end rounded down.
func (f FPS) FramesInside(s0, s1, sampleRate int64) (start, end int64) {
	return f.sampleCeil(s0, sampleRate), f.sampleFloor(s1, sampleRate)
}

// FramesCovering is the smallest frame range covering the sample range [s0, s1):
// start rounded down, end rounded up.
func (f FPS) FramesCovering(s0, s1, sampleRate int64) (start, end int64) {
	return f.sampleFloor(s0, sampleRate), f.sampleCeil(s1, sampleRate)
}

// FrameAtSample is the frame nearest to a sample position, halves rounded up.
func (f FPS) FrameAtSample(s, sampleRate int64) int64 {
	n := new(big.Int).Mul(big.NewInt(s), big.NewInt(f.Num))
	d := new(big.Int).Mul(big.NewInt(sampleRate), big.NewInt(f.Den))
	return roundDiv(n, d).Int64()
}
