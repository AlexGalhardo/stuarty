package gioapp

import (
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/timeval"
)

func TestFrameAfter(t *testing.T) {
	t.Parallel()

	fps := timeval.FPS{Num: 30, Den: 1}
	if got := frameAfter(10, time.Second, fps); got != 40 {
		t.Fatalf("1s at 30 fps from 10 = %d, want 40", got)
	}

	if got := frameAfter(0, 500*time.Millisecond, fps); got != 15 {
		t.Fatalf("0.5s = %d, want 15", got)
	}

	if got := frameAfter(3, -time.Second, fps); got != 3 {
		t.Fatalf("negative elapsed = %d, want 3", got)
	}
}
