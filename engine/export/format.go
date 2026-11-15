package export

import (
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// Format is what an export writes (SDD-004 section 8, RF-064). The zero value is MP4.
type Format string

// The formats of an export. Audio is a project's mix only: no picture is composed.
const (
	FormatMP4 Format = "mp4" // H.264 and AAC, the default
	FormatGIF Format = "gif" // composed frames, a fixed palette, written by the engine
	FormatWAV Format = "wav" // the 48 kHz stereo 16-bit mix
	FormatAAC Format = "aac" // the mix as AAC in an MP4 container, extension .m4a
)

// Valid says whether f is a format of an export; the empty string is MP4.
func (f Format) Valid() bool {
	switch f {
	case "", FormatMP4, FormatGIF, FormatWAV, FormatAAC:
		return true
	}

	return false
}

func (f Format) normal() Format {
	if f == "" {
		return FormatMP4
	}

	return f
}

// Ext is the extension of the published file, with its dot.
func (f Format) Ext() string {
	switch f.normal() {
	case FormatGIF:
		return ".gif"
	case FormatWAV:
		return ".wav"
	case FormatAAC:
		return ".m4a"
	case FormatMP4:
	}

	return ".mp4"
}

// gifMaxFPS is the highest frame rate of a GIF: the delay is in hundredths of a second and browsers
// show a delay under 2 as 10, so 25 fps (4 hundredths) is where an even step stays exact enough.
const gifMaxFPS = 25

// gifStep is how many timeline frames one GIF frame covers: the smallest whole number that brings the
// frame rate to gifMaxFPS or less (30 fps gives 2, 60 fps gives 3, 24 fps gives 1).
func gifStep(fps timeline.Rational) int {
	return max(1, (fps.Num+gifMaxFPS*fps.Den-1)/(gifMaxFPS*fps.Den))
}

// gifDelaysCS are the delay of each GIF frame in hundredths of a second for n timeline frames: the
// delays are differences of the rounded start times, so the total stays within one hundredth of the
// timeline and no error piles up. A delay is never below 2 (SDD-004 section 8).
func gifDelaysCS(fps timeline.Rational, n int) []int {
	step := gifStep(fps)
	count := (n + step - 1) / step
	at := func(t int) int {
		// round(100 * t * den / num) without floats
		return int((int64(t)*100*int64(fps.Den) + int64(fps.Num)/2) / int64(fps.Num))
	}
	out := make([]int, count)
	for j := range out {
		end := min((j+1)*step, n)
		out[j] = max(2, at(end)-at(j*step))
	}

	return out
}
