package motion

import (
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func doc() *timeline.Document {
	d := &timeline.Document{
		Settings: timeline.Settings{Width: 1080, Height: 1920, FPS: timeline.Rational{Num: 30, Den: 1}},
		Assets:   map[string]timeline.Asset{},
		Tracks: map[string]timeline.Track{
			"trk_aaaaaaaaaa": {ID: "trk_aaaaaaaaaa", Kind: timeline.TrackVisual, Name: "Overlay", Order: "a0"},
			"trk_bbbbbbbbbb": {ID: "trk_bbbbbbbbbb", Kind: timeline.TrackVisual, Name: "Video", Order: "a1"},
			"trk_cccccccccc": {ID: "trk_cccccccccc", Kind: timeline.TrackAudio, Name: "Music", Order: "a2"},
		},
		Items: map[string]timeline.Item{},
	}
	caption := func(id string, start, dur int, words ...string) {
		it := timeline.NewItem(id, timeline.ItemCaption, "trk_aaaaaaaaaa", start, dur)
		for i, w := range words {
			it.Words = append(it.Words, timeline.Word{Text: w, Start: i * 5, Duration: 5})
		}

		d.Items[id] = it
	}
	caption("itm_cap0000001", 0, 30, "before", "range")
	caption("itm_cap0000002", 40, 20, "hello", "world")
	caption("itm_cap0000003", 80, 40, "inside", "too")
	caption("itm_cap0000004", 200, 30, "after")

	return d
}

func TestBriefHoldsWhatARenderMustFit(t *testing.T) {
	t.Parallel()

	b, err := Build(doc(), 50, 130, "trk_aaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}

	if b.Width != 1080 || b.Height != 1920 || b.FrameRate != (timeline.Rational{Num: 30, Den: 1}) || b.DurationFrames != 80 {
		t.Errorf("size, rate or length: %+v", b)
	}

	// SDD-004 section 4.1: a 1080x1920 frame has the safe area {65, 269, 950, 979}.
	if b.SafeArea != (Rect{X: 65, Y: 269, Width: 950, Height: 979}) ||
		b.Margins != (Margins{Left: 65, Right: 65, Top: 269, Bottom: 672}) {
		t.Errorf("safe area %+v margins %+v", b.SafeArea, b.Margins)
	}

	if b.Track == nil || !b.Track.Overlay || !b.NeedsAlpha {
		t.Errorf("the overlay track needs alpha: %+v", b.Track)
	}

	// The captions that show inside [50, 130), times relative to the start of the range, in order.
	got := []Caption{
		{ItemID: "itm_cap0000002", Start: -10, End: 10, Text: "hello world"},
		{ItemID: "itm_cap0000003", Start: 30, End: 70, Text: "inside too"},
	}
	if !slices.Equal(b.Captions, got) {
		t.Errorf("captions %+v, want %+v", b.Captions, got)
	}
}

func TestBriefTrackDecidesAlpha(t *testing.T) {
	t.Parallel()

	for id, want := range map[string]bool{"trk_aaaaaaaaaa": true, "trk_bbbbbbbbbb": false, "": false} {
		b, err := Build(doc(), 0, 30, id)
		if err != nil {
			t.Fatal(err)
		}

		if b.NeedsAlpha != want {
			t.Errorf("track %q: needs_alpha %v, want %v", id, b.NeedsAlpha, want)
		}
	}
}

func TestBriefRefusals(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		from, until int
		track       string
		reason      string
	}{
		"negative":   {-1, 10, "", "range_negative"},
		"empty":      {10, 10, "", "range_empty"},
		"reversed":   {20, 10, "", "range_empty"},
		"too long":   {0, MaxFrames + 1, "", "range_too_long"},
		"no track":   {0, 10, "trk_zzzzzzzzzz", "track_unknown"},
		"audio":      {0, 10, "trk_cccccccccc", "track_not_visual"},
		"longest ok": {0, MaxFrames, "", ""},
	} {
		_, err := Build(doc(), c.from, c.until, c.track)

		switch {
		case c.reason == "" && err != nil:
			t.Errorf("%s: %v", name, err)
		case c.reason != "" && (err == nil || err.Reason != c.reason):
			t.Errorf("%s: %v, want %s", name, err, c.reason)
		}
	}
}
