package export

import (
	"context"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/audio"
)

// MixFrom mixes the timeline audio from frame from, for n frames, and hands the
// result to sink in blocks of stereo float32 at 48 kHz (engine/audio.Mix). The
// sources are the same ones an export mixes. A range with no sound still calls
// sink with silence, so a player can keep its clock. n must be at least 1.
func (r *Renderer) MixFrom(ctx context.Context, from, n int, sink func([]float32) error) error {
	if n < 1 {
		return nil
	}
	if from < 0 {
		from = 0
	}

	doc := r.s.job.Doc
	fps := doc.Settings.FPS
	start := samplesAt(fps, from)
	total := samplesAt(fps, from+n) - start
	if total < 1 {
		return nil
	}

	srcs := plan.Audio(doc, from, from+n)
	for i := range srcs {
		srcs[i].StartSample -= start
	}

	return audio.Mix(ctx, srcs, total, &audioOpener{s: r.s}, sink)
}
