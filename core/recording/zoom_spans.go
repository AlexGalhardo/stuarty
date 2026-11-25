package recording

import (
	"math"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// ZoomSpan is one zoom of a clip as its scale keyframes hold it (SDD-025 section 17): the frame the view starts
// to move in, the frame it is fully in, the frame it starts to move out and the frame it is back. The frames are
// relative to the start of the clip, as keyframes are.
type ZoomSpan struct {
	StartFrame   int
	FullFrame    int
	ReleaseFrame int
	EndFrame     int
}

// scaleRest is how close to the resting scale a keyframe is for the view to count as at rest.
const scaleRest = 0.001

// ZoomSpans lists the zooms in the scale keyframes of a clip, in time order. A zoom is a hold: two keyframes in a
// row at one scale above the resting scale, which is what recording.auto_zoom writes for each zoom; the keyframe
// before the hold starts the zoom and the keyframe after it ends it, so a pan from one zoom to the next is a
// zoom of its own. Keyframes a person set by hand count the same way.
func ZoomSpans(scale []timeline.Keyframe, rest float64) []ZoomSpan {
	var spans []ZoomSpan

	for i := 0; i+1 < len(scale); i++ {
		a, b := scale[i], scale[i+1]
		if a.Value <= rest+scaleRest || math.Abs(a.Value-b.Value) > scaleRest {
			continue
		}

		// The hold runs to its last keyframe at that scale: a hold of three keyframes is one zoom, not two, and it is
		// released at its last keyframe.
		last := i + 1
		for last+1 < len(scale) && math.Abs(scale[last+1].Value-b.Value) <= scaleRest {
			last++
		}

		span := ZoomSpan{StartFrame: a.T, FullFrame: a.T, ReleaseFrame: scale[last].T, EndFrame: scale[last].T}
		if i > 0 && scale[i-1].Value < a.Value-scaleRest {
			span.StartFrame = scale[i-1].T
		}

		if last+1 < len(scale) && scale[last+1].Value < b.Value-scaleRest {
			span.EndFrame = scale[last+1].T
		}

		spans = append(spans, span)
		// The release keyframe belongs to this zoom; the next one can start at it only as the start of a pan.
		i = last
	}

	return spans
}
