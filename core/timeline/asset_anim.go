package timeline

import "fmt"

// Display time of one frame of an animated image, in milliseconds.
const (
	MinFrameDelay = 10
	MaxFrameDelay = 60000
)

// IsAnimated reports whether the asset is an animated image (SDD-001 §3.2).
func (a Asset) IsAnimated() bool { return a.Kind == AssetImage && len(a.FrameDelays) > 0 }

// animation checks the animation facts of an asset: only an image has them, and
// frame_count must match the delays.
func (v *validator) animation(path, key string, a Asset) {
	if len(a.FrameDelays) == 0 {
		return
	}
	if a.Kind != AssetImage {
		v.add(VShape, path+".frame_delays", key, "frame_delays applies to image assets only")
		return
	}
	if len(a.FrameDelays) < 2 || len(a.FrameDelays) > 1000 {
		v.add(VShape, path+".frame_delays", key, "an animated image has 2 to 1000 frames")
	}
	if a.FrameCount != len(a.FrameDelays) {
		v.add(
			VShape,
			path+".frame_count",
			key,
			"frame_count %d differs from the %d frame_delays",
			a.FrameCount,
			len(a.FrameDelays),
		)
	}
	for i, d := range a.FrameDelays {
		if d < MinFrameDelay || d > MaxFrameDelay {
			v.add(
				VShape,
				fmt.Sprintf("%s.frame_delays[%d]", path, i),
				key,
				"frame delay must be %d to %d ms",
				MinFrameDelay,
				MaxFrameDelay,
			)
		}
	}
}
