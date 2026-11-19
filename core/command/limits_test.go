package command

import (
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const lastItem = "itm_z000000499"

// limitsDoc is a document at every limit of SDD-001 §4 at once: 50 tracks, 200 assets,
// 500 items of which 100 captions hold the 5,000 words, and the last item ends at frame
// 108,000 of a 120 fps project.
func limitsDoc() *timeline.Document {
	b := newCaseDoc().with(func(d *timeline.Document) {
		d.Settings.FPS = timeline.Rational{Num: 120, Den: 1}
		d.Assets = map[string]timeline.Asset{}
		d.Tracks = map[string]timeline.Track{}
	})

	for i := range timeline.MaxAssets {
		id := fmt.Sprintf("ast_z%09d", i)
		b.d.Assets[id] = timeline.Asset{
			ID: id, Hash: hashN(i), Kind: timeline.AssetImage, Name: "i", Width: 10, Height: 10,
		}
	}

	for i := range timeline.MaxTracks {
		id := fmt.Sprintf("trk_z%09d", i)
		b.d.Tracks[id] = timeline.Track{ID: id, Kind: timeline.TrackVisual, Name: "T", Order: fmt.Sprintf("a%02d", i)}
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

		b.d.Items[id] = it
	}

	return b.d
}

// TestDocumentAtTheLimits runs a document at every limit of SDD-001 §4 through Apply
// and lint: the time arithmetic holds at frame 108,000 and 120 fps, and one
// more of anything is refused.
func TestDocumentAtTheLimits(t *testing.T) {
	doc := limitsDoc()
	if vs := timeline.Validate(doc); len(vs) != 0 {
		t.Fatalf("the limits document is invalid: %v", vs)
	}

	reg := fullRegistry()
	env := Env{NewIDs: (&seqIDs{}).next}

	res, err := reg.ApplyBatch(doc, []Operation{
		opJSON("item.move", fmt.Sprintf(`{"item_id":%q,"start":"%df"}`, lastItem, timeline.MaxFrames-100)),
		opJSON("item.rename", fmt.Sprintf(`{"item_id":%q,"name":"last"}`, lastItem)),
	}, env)
	if err != nil {
		t.Fatalf("apply at the limits: %v", err)
	}

	if end := res.Doc.Items[lastItem].Start + res.Doc.Items[lastItem].Duration; end != timeline.MaxFrames {
		t.Errorf("the last item ends at %d, want %d", end, timeline.MaxFrames)
	}

	past := opJSON("item.move", fmt.Sprintf(`{"item_id":%q,"start":"%df"}`, lastItem, timeline.MaxFrames-99))
	_, err = reg.Apply(res.Doc, past, env)
	if e := AsError(err); err == nil || e.Code != CodeInvalidTime {
		t.Errorf("a move past frame %d returned %v, want %s", timeline.MaxFrames, err, CodeInvalidTime)
	}

	// The plan window at the limits belongs to core/plan's tests: core/command never
	// imports core/plan (SDD-004 §1 rule 1, enforced by internal/tools/importcheck).
	for _, mode := range []lint.Mode{lint.Fast, lint.Full} {
		for _, f := range lint.Lint(res.Doc, lint.Env{}, mode) {
			if f.Range != nil && (f.Range.Start < 0 || f.Range.End > timeline.MaxFrames) {
				t.Errorf("%s has the range %v outside the project", f.Code, *f.Range)
			}
		}
	}

	over := map[string]string{
		"track.add": `{"kind":"visual","name":"one more"}`,
		"text.add":  `{"track_id":"trk_z000000000","start":"50000f","duration":"1s","text":"x"}`,
	}
	for name, args := range over {
		_, err = reg.Apply(res.Doc, opJSON(name, args), env)
		if e := AsError(err); err == nil || e.Code != CodeLimit {
			t.Errorf("%s over the limit returned %v, want %s", name, err, CodeLimit)
		}
	}
}
