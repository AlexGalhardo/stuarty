package plan

import "github.com/AlexGalhardo/stuarty/core/timeline"

// AnimImageSourceFrame is the frame of an animated image shown at frame r of its
// item (SDD-004 §3.3): the time of the frame in milliseconds, rounded to the nearest
// millisecond (halves up), taken modulo the sum of the frame delays, selects the
// frame whose cumulative delay it has not reached. Integer arithmetic only. An
// empty or invalid delay list gives frame 0.
func AnimImageSourceFrame(delays []int, fps timeline.Rational, r int) int {
	var total int64

	for _, d := range delays {
		if d <= 0 {
			return 0
		}

		total += int64(d)
	}

	if total == 0 || fps.Num <= 0 || fps.Den <= 0 || r <= 0 {
		return 0
	}

	ms := (2*int64(r)*1000*int64(fps.Den) + int64(fps.Num)) / (2 * int64(fps.Num))
	pos := ms % total

	var cum int64

	for i, d := range delays {
		if cum += int64(d); pos < cum {
			return i
		}
	}

	return len(delays) - 1
}
