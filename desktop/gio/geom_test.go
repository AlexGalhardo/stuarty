package gioapp

import (
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeval"
)

func TestFramePixelRoundTrip(t *testing.T) {
	t.Parallel()

	fps := timeval.FPS{Num: 30, Den: 1}
	const px = 90.0

	for _, frame := range []int{0, 15, 30, 45, 300} {
		x := frameToX(frame, fps, px)
		got := xToFrame(x, fps, px)

		if got != frame {
			t.Fatalf("frame %d -> x %d -> frame %d", frame, x, got)
		}
	}
}

func TestHitTimeline(t *testing.T) {
	t.Parallel()

	clips := []placedClip{{id: "itm_a", start: 40, dur: 80, row: 0}}
	frameAt := func(int) int { return 3 }

	if h := hitTimeline(clips, 10, 10, 28, 36, 6, frameAt); h.kind != hitRuler {
		t.Fatalf("ruler hit = %v", h.kind)
	}

	if h := hitTimeline(clips, 50, 40, 28, 36, 6, frameAt); h.kind != hitBody || h.itemID != "itm_a" {
		t.Fatalf("body hit = %+v", h)
	}

	if h := hitTimeline(clips, 42, 40, 28, 36, 6, frameAt); h.kind != hitStart {
		t.Fatalf("start edge = %v", h.kind)
	}

	if h := hitTimeline(clips, 116, 40, 28, 36, 6, frameAt); h.kind != hitEnd {
		t.Fatalf("end edge = %v", h.kind)
	}

	if h := hitTimeline(clips, 10, 40, 28, 36, 6, frameAt); h.kind != hitEmpty {
		t.Fatalf("empty = %v", h.kind)
	}
}

func TestFramesArg(t *testing.T) {
	t.Parallel()

	if framesArg(45) != "45f" {
		t.Fatalf("framesArg = %q", framesArg(45))
	}
}
