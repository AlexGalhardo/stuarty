package ffmpeg

import "strconv"

const (
	// MaxAnimFrames is the most frames an animated image may have (SDD-001 section 3.2).
	MaxAnimFrames = 1000
	// MaxAnimPixels is the most pixels, summed over all frames, an animated image may decode to. It
	// bounds the work and the disk of the frame sequence: 128 Mpixel is 512 MiB of raw RGBA.
	MaxAnimPixels int64 = 134_217_728
)

// CheckAnimation applies the animation ceilings of SDD-007 9.1 to the frame count and the size of an
// animated image, known from the packets and the stream header and before any frame is decoded (T-17).
func CheckAnimation(frames, width, height int) error {
	if frames > MaxAnimFrames {
		return limit(
			"anim_frames",
			int64(frames),
			MaxAnimFrames,
			"",
			"The animated image has more than "+strconv.Itoa(MaxAnimFrames)+" frames, the most accepted.",
		)
	}
	if px := int64(frames) * int64(width) * int64(height); px > MaxAnimPixels {
		return limit(
			"anim_pixels",
			px,
			MaxAnimPixels,
			"",
			"The animated image has "+strconv.FormatInt(px, 10)+" pixels over all its frames; the most accepted is "+
				strconv.FormatInt(MaxAnimPixels, 10)+".",
		)
	}
	return nil
}
