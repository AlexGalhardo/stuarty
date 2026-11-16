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

// gifMaxFPS is the highest frame rate of a GIF. The GIF is only a preview or a sticker, and its
// size grows with the number of frames: 15 fps keeps the motion readable at about half the bytes of
// 25. Delays are in hundredths of a second, so 15 fps is 6 and 7 alternating, never below 2.
const gifMaxFPS = 15

// gifMaxWidth is the widest a GIF is made: a wider project is composed at this width, with its
// height scaled to match, because a 720p or 1080p GIF is hundreds of megabytes (VV-056).
const gifMaxWidth = 480

// gifSize is the pixel size of the GIF of a project: the project's size, or gifMaxWidth wide with
// the height rounded to the same aspect when the project is wider. scale is the output scale.
func gifSize(st timeline.Settings) (w, h int, scale float64) {
	if st.Width <= gifMaxWidth {
		return st.Width, st.Height, 1
	}

	scale = float64(gifMaxWidth) / float64(st.Width)

	return gifMaxWidth, max(round(float64(st.Height)*scale), 1), scale
}

// gifStep is how many timeline frames one GIF frame covers: the smallest whole number that brings the
// frame rate to gifMaxFPS or less (30 fps gives 2, 60 fps gives 4, 24 fps gives 2).
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

// The bytes of one GIF pixel in one frame, measured on the writer of format_gif.go at 480x270: a
// dithered gradient is 0.48, which is what the 130 MB and 200 MB GIFs of VV-056 came to (0.43 to
// 0.47), flat colour 0.06 and noise 1.34. A typical project is near the gradient; noise is the bound.
const (
	gifTypicalBytesPerPixel = 0.5
	gifWorstBytesPerPixel   = 1.4
)

// GIFEstimate is what a GIF export of a project will be, known before it starts.
type GIFEstimate struct {
	Width, Height int   // the pixel size of the GIF (gifMaxWidth wide at most)
	Frames        int   // the number of GIF frames, after the frame rate is lowered
	TypicalBytes  int64 // the size of ordinary footage
	WorstBytes    int64 // the size of the noisiest picture, which no export passes
}

// EstimateGIF is the size of the GIF of a project of frames timeline frames, from the size and the
// frame rate the export will use (gifSize and gifStep). It is an estimate to warn a person before a
// huge file is written (VV-056), not a promise: the real size depends on the picture.
func EstimateGIF(st timeline.Settings, frames int) GIFEstimate {
	w, h, _ := gifSize(st)
	if frames <= 0 || w <= 0 || h <= 0 || st.FPS.Num <= 0 || st.FPS.Den <= 0 {
		return GIFEstimate{Width: w, Height: h}
	}

	n := (frames + gifStep(st.FPS) - 1) / gifStep(st.FPS)
	px := float64(w) * float64(h) * float64(n)

	return GIFEstimate{
		Width: w, Height: h, Frames: n,
		TypicalBytes: int64(px * gifTypicalBytesPerPixel),
		WorstBytes:   int64(px * gifWorstBytesPerPixel),
	}
}
