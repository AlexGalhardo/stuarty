package gioapp

import (
	"time"

	"github.com/AlexGalhardo/vukem-video/core/timeval"
)

// frameAfter is the timeline frame origin plus the frames that elapsed at fps.
// Playback uses it so the playhead follows the wall clock instead of one render.
func frameAfter(origin int, elapsed time.Duration, fps timeval.FPS) int {
	if fps.Num <= 0 || fps.Den <= 0 {
		fps = timeval.FPS{Num: 30, Den: 1}
	}

	if elapsed < 0 {
		elapsed = 0
	}

	step := max(elapsed.Milliseconds()*fps.Num/(fps.Den*1000), 0)

	return origin + int(step)
}
