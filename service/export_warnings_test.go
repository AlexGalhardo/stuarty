package service

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func TestExportWarnsOfAHugeGIFOnly(t *testing.T) {
	t.Parallel()

	doc := &timeline.Document{Settings: timeline.Settings{
		Width: 1920, Height: 1080, FPS: timeline.Rational{Num: 30, Den: 1},
	}}

	got := exportWarnings(doc, FormatGIF, 30*60)
	if len(got) != 2 || !strings.Contains(got[0], "scaled to 480x270") || !strings.Contains(got[1], "MiB") ||
		!strings.Contains(got[1], "mp4") {
		t.Errorf("a 60 s GIF of 1080p warns %q, want the scaling and the size", got)
	}

	if got := exportWarnings(doc, FormatGIF, 30*5); len(got) != 1 || !strings.Contains(got[0], "480 pixels wide") {
		t.Errorf("a 5 s GIF warns %q, want the scaling only", got)
	}

	small := &timeline.Document{Settings: timeline.Settings{
		Width: 400, Height: 400, FPS: timeline.Rational{Num: 30, Den: 1},
	}}
	if got := exportWarnings(small, FormatGIF, 30*5); got != nil {
		t.Errorf("a 400 px GIF warns %q", got)
	}

	if got := exportWarnings(doc, FormatMP4, 30*60); got != nil {
		t.Errorf("an mp4 warns %q", got)
	}
}
