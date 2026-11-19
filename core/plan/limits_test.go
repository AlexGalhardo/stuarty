package plan

import (
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// limitsDoc is a document at every limit of SDD-001 §4 at once: 50 tracks, 200 assets,
// 500 items of which 100 captions hold the 5,000 words, and the last item ends at frame
// 108,000 of a 120 fps project.
func limitsDoc() *timeline.Document {
	d := newDoc()
	d.Settings.FPS = timeline.Rational{Num: 120, Den: 1}
	d.Assets = map[string]timeline.Asset{}
	d.Tracks = map[string]timeline.Track{}
	d.Items = map[string]timeline.Item{}
	d.Name = "Limits"

	for i := range timeline.MaxAssets {
		id := fmt.Sprintf("ast_z%09d", i)
		d.Assets[id] = timeline.Asset{
			ID: id, Hash: fmt.Sprintf("sha256:%064x", i+1), Kind: timeline.AssetImage, Name: "i", Width: 10, Height: 10,
		}
	}

	for i := range timeline.MaxTracks {
		id := fmt.Sprintf("trk_z%09d", i)
		d.Tracks[id] = timeline.Track{ID: id, Kind: timeline.TrackVisual, Name: "T", Order: fmt.Sprintf("a%02d", i)}
	}

	for i := range timeline.MaxItems {
		id := fmt.Sprintf("itm_z%09d", i)
		track := fmt.Sprintf("trk_z%09d", i%timeline.MaxTracks)
		start := (i / timeline.MaxTracks) * 10000
		if i == timeline.MaxItems-1 {
			start = timeline.MaxFrames - 100
		}

		it := timeline.NewItem(id, timeline.ItemText, track, start, 100)
		it.Text = "x"

		if i < 100 {
			it.Type, it.Text = timeline.ItemCaption, ""
			for j := range 50 {
				it.Words = append(it.Words, timeline.Word{Text: "w", Start: j * 2, Duration: 2})
			}
		}

		d.Items[id] = it
	}

	return d
}

// TestWindowAtTheLimits runs the plan window over a document at every SDD-001 §4 limit:
// the time arithmetic holds at frame 108,000 and 120 fps.
func TestWindowAtTheLimits(t *testing.T) {
	doc := limitsDoc()
	if v := timeline.Validate(doc); len(v) != 0 {
		t.Fatalf("the limits document is invalid: %v", v)
	}

	w := Window(doc, timeline.MaxFrames-MaxWindow, timeline.MaxFrames)
	if w.EndFrame != timeline.MaxFrames || len(w.Layers) == 0 {
		t.Errorf("window at the end: EndFrame %d with %d layers", w.EndFrame, len(w.Layers))
	}

	for _, l := range w.Layers {
		if l.First < timeline.MaxFrames-MaxWindow || l.Last >= timeline.MaxFrames || l.Last < l.First {
			t.Fatalf("layer %s spans %d to %d outside the window", l.Item, l.First, l.Last)
		}
	}
}
