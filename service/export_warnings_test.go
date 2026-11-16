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
	if len(got) != 1 || !strings.Contains(got[0], "MiB") || !strings.Contains(got[0], "mp4") {
		t.Errorf("a 60 s GIF warns %q, want one warning with the size", got)
	}

	if got := exportWarnings(doc, FormatGIF, 30*5); got != nil {
		t.Errorf("a 5 s GIF warns %q", got)
	}

	if got := exportWarnings(doc, FormatMP4, 30*60); got != nil {
		t.Errorf("an mp4 warns %q", got)
	}
}
