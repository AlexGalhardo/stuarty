package plan

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func TestTransparentBackgroundIsMarkedAndBlackForOldExecutors(t *testing.T) {
	t.Parallel()

	d := newDoc()
	put(d, clip("itm_a", trkBottom, 0, 10))

	if fp := Frame(d, 0); fp.Transparent || fp.Background == "" {
		t.Errorf("an opaque project must not be marked transparent: %+v", fp)
	}

	d.Settings.Background = timeline.BackgroundTransparent

	fp := Frame(d, 0)
	if !fp.Transparent || fp.Background != "#000000" {
		t.Errorf("frame plan background %q transparent %v, want #000000 and true", fp.Background, fp.Transparent)
	}

	if w := Window(d, 0, 5); !w.Transparent || w.Background != "#000000" {
		t.Errorf("window background %q transparent %v, want #000000 and true", w.Background, w.Transparent)
	}
}
