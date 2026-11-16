package audio

import "math"

// The arguments of FFmpeg's afftdn that a denoise strength maps to (SDD-004 section 7): the noise
// reduction in dB grows linearly with the strength up to MaxDenoiseNR, and the noise floor that the
// filter assumes is DenoiseNF dBFS whatever the strength. afftdn attenuates what lies near the assumed
// floor and passes what is well above it, so the floor is the level of the noise of a typical recording
// once its loudness is normalized (the ingest brings every source to about -23 LUFS); a louder noise is
// reduced less. Raising the floor with the strength was tried and made the strongest setting the
// weakest on a noise below the floor.
const (
	MaxDenoiseNR = 30  // dB at a strength of 1
	DenoiseNF    = -45 // dBFS
)

// DenoiseParams maps a strength (0 to 1, anything above is 1) to the `nr` and `nf` of afftdn; nr is
// rounded to the 2 decimals of the filter argument, so it only grows with the strength. The last result is
// false when the strength asks for no filter at all (0, under 0.0005 or not a number).
func DenoiseParams(denoise float64) (nr, nf float64, ok bool) {
	if !(denoise > 0) {
		return 0, 0, false
	}
	d := min(denoise, 1)
	nr = math.Round(d*MaxDenoiseNR*100) / 100
	if nr < 0.01 {
		return 0, 0, false
	}
	nf = DenoiseNF
	return nr, nf, true
}
