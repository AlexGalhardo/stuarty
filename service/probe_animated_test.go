package service

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeval"
)

func TestProbeTextOfAnAnimatedImage(t *testing.T) {
	t.Parallel()

	fps := timeval.FPS{Num: 30, Den: 1}
	p := &ProbeSummary{
		Kind: "image", Width: 512, Height: 512, SizeBytes: 1024, Animated: true, Frames: 30, LoopDurationMS: 2000,
	}

	got := probeText(fps, p)
	for _, want := range []string{"animated image 512x512", "30 frames", "loop 2.000 s"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q does not contain %q", got, want)
		}
	}

	still := &ProbeSummary{Kind: "image", Width: 512, Height: 512, SizeBytes: 1024}
	if got := probeText(fps, still); strings.Contains(got, "animated") {
		t.Errorf("a still image reads %q", got)
	}
}
