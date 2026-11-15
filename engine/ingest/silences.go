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

// FramesFromSamples converts sample ranges at 48 kHz into asset frame ranges at the frame rate
// of the asset. A frame belongs to a range only when it lies wholly inside it, so the start
// rounds up and the end rounds down; a range that holds no whole frame is dropped. The input
// is sorted and not overlapping, and so is the output.
func FramesFromSamples(ranges []SilenceRange, rate Rational) []FrameRange {
	out := make([]FrameRange, 0, len(ranges))

	if rate.Num <= 0 || rate.Den <= 0 {
		return out
	}

	per := int64(SampleRate) * rate.Den

	for _, r := range ranges {
		start := (r.Start*rate.Num + per - 1) / per
		end := r.End * rate.Num / per

		if start < end {
			out = append(out, FrameRange{Start: start, End: end})
		}
	}

	return out
}
