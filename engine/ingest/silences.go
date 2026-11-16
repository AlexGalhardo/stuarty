package ingest

// FrameRange is a half-open range [Start, End) of frames on the time axis of an asset.
type FrameRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// MinSilence keeps the silences that last at least minSamples, in the order they have: the
// filter of clip.remove_silences (SDD-002 C.27) and of the audible-onset step of the transcript
// repair, which uses the -35 dB set with a minimum of 300 ms (ADR-0021).
func MinSilence(ranges []SilenceRange, minSamples int64) []SilenceRange {
	out := make([]SilenceRange, 0, len(ranges))

	for _, r := range ranges {
		if r.End-r.Start >= minSamples {
			out = append(out, r)
		}
	}

	return out
}
