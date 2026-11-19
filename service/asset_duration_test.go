package service

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeval"
)

func TestAssetDurationFramesAgreesWithTheDocument(t *testing.T) {
	t.Parallel()

	fps := timeval.FPS{Num: 30, Den: 1}
	samples, count, num, den := int64(5292000), int64(1802), 30, 1 // 110.25 s of audio; 60.067 s of video

	// Probed milliseconds round to 3308 frames; the document floors the samples to 3307.
	audio := &AssetFacts{SampleCount: &samples}
	if got := assetDurationFrames(audio, fps, 110267); got != 3307 {
		t.Errorf("audio frames = %d, want 3307", got)
	}

	video := &AssetFacts{FrameCount: &count, FrameRateNum: &num, FrameRateDen: &den}
	if got := assetDurationFrames(video, fps, 60067); got != 1802 {
		t.Errorf("video frames = %d, want 1802", got)
	}

	if got := assetDurationFrames(&AssetFacts{}, fps, 2000); got != 60 {
		t.Errorf("fallback frames = %d, want 60", got)
	}
}
