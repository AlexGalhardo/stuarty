package command

import (
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// diffBase is a document with a clip, a text, a caption and an audio item.
func diffBase() *timeline.Document {
	return base().
		clip(itemA, vidTr, 0, 120, 0).
		text(itemB, capTr, 200, 60).
		caption(itemC, 10, 4).
		audio("itm_a000000001", 0, 300, 0).d
}

func mutate(t *testing.T, doc *timeline.Document, f func(d *timeline.Document)) *timeline.Document {
	t.Helper()

	out := timeline.Clone(doc)
	f(out)

	if vs := timeline.Validate(out); len(vs) != 0 {
		t.Fatalf("the test document is invalid: %v", vs)
	}

	return out
}

func editItem(d *timeline.Document, id string, f func(it *timeline.Item)) {
	it := d.Items[id]
	f(&it)
	d.Items[id] = it
}

// TestDiffDocumentsKinds is the table of every kind of change between two documents: what the
// entry says (verb, noun, kind) and that the changes are those of the batch diff.
func TestDiffDocumentsKinds(t *testing.T) {
	a := diffBase()

	cases := []struct {
		name   string
		edit   func(d *timeline.Document)
		verb   string
		noun   string
		kind   string
		id     string
		fields []string
	}{
		{
			"clip removed", func(d *timeline.Document) { delete(d.Items, itemA) },
			VerbRemoved, "clip", KindItemRemoved, itemA, nil,
		},
		{
			"clip added", func(d *timeline.Document) {
				d.Items["itm_n000000001"] = timeline.NewItem("itm_n000000001", timeline.ItemShape, vidTr, 400, 30)
				editItem(
					d,
					"itm_n000000001",
					func(it *timeline.Item) { it.Shape, it.Width, it.Height = "rect", 10, 10 },
				)
			},
			VerbAdded, "shape", KindItemAdded, "itm_n000000001", nil,
		},
		{
			"clip moved", func(d *timeline.Document) { editItem(d, itemA, func(it *timeline.Item) { it.Start = 30 }) },
			VerbMoved, "clip", KindItemUpdated, itemA,
			[]string{"start"},
		},
		{
			"clip trimmed at the end",
			func(d *timeline.Document) { editItem(d, itemA, func(it *timeline.Item) { it.Duration = 90 }) },
			VerbTrimmed, "clip", KindItemUpdated, itemA,
			[]string{"duration"},
		},
		{
			"clip trimmed at the start", func(d *timeline.Document) {
				editItem(d, itemA, func(it *timeline.Item) { it.Start, it.Duration, it.SourceIn = 20, 100, 20 })
			},
			VerbTrimmed, "clip", KindItemUpdated, itemA,
			[]string{"duration", "source_in", "start"},
		},
		{
			"property changed",
			func(d *timeline.Document) { editItem(d, itemA, func(it *timeline.Item) { it.Opacity = 0.5 }) },
			VerbChanged,
			"clip",
			KindItemUpdated,
			itemA,
			[]string{"opacity"},
		},
		{
			"text changed",
			func(d *timeline.Document) { editItem(d, itemB, func(it *timeline.Item) { it.Text = "Bye" }) },
			VerbChanged,
			"text",
			KindItemUpdated,
			itemB,
			[]string{"text"},
		},
		{
			"caption removed", func(d *timeline.Document) { delete(d.Items, itemC) },
			VerbRemoved, "caption", KindItemRemoved, itemC, nil,
		},
		{
			"caption moved",
			func(d *timeline.Document) { editItem(d, itemC, func(it *timeline.Item) { it.Start = 50 }) },
			VerbMoved,
			"caption",
			KindItemUpdated,
			itemC,
			[]string{"start"},
		},
		{
			"track added", func(d *timeline.Document) {
				d.Tracks["trk_n000000001"] = timeline.Track{
					ID: "trk_n000000001", Kind: timeline.TrackVisual, Name: "New", Order: "a3",
				}
			},
			VerbAdded, "track", KindTrackAdded, "trk_n000000001", nil,
		},
		{
			"track renamed", func(d *timeline.Document) {
				tr := d.Tracks[audTr]
				tr.Name = "Sound"
				d.Tracks[audTr] = tr
			},
			VerbChanged, "track", KindTrackUpdated, audTr,
			[]string{"name"},
		},
		{
			"project renamed", func(d *timeline.Document) { d.Name = "Another" },
			VerbChanged, "project", KindProjectUpdated, prjID,
			[]string{"name"},
		},
		{
			"settings changed", func(d *timeline.Document) { d.Settings.Width = 720 },
			VerbChanged, "settings", KindSettingsUpdated, prjID,
			[]string{"width"},
		},
		{
			"asset removed", func(d *timeline.Document) { delete(d.Items, itemA); delete(d.Assets, vidAsset) },
			VerbRemoved, "asset", KindAssetRemoved, vidAsset, nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := mutate(t, a, tc.edit)
			entries := DiffEntries(a, b)

			var got *DiffEntry

			for i := range entries {
				if entries[i].Change.ID == tc.id && entries[i].Change.Kind == tc.kind {
					got = &entries[i]
				}
			}

			if got == nil {
				t.Fatalf("no entry of kind %s for %s in %+v", tc.kind, tc.id, entries)
			}

			if got.Verb != tc.verb || got.Noun != tc.noun {
				t.Errorf("entry is %s %s, want %s %s", got.Verb, got.Noun, tc.verb, tc.noun)
			}

			if !strings.HasPrefix(got.Line, tc.verb+" ") {
				t.Errorf("the line %q does not start with the verb %q", got.Line, tc.verb)
			}

			if tc.noun != "project" && tc.noun != "settings" && !strings.Contains(got.Line, tc.id) {
				t.Errorf("the line %q does not name %s", got.Line, tc.id)
			}

			for _, f := range tc.fields {
				if _, ok := got.Change.Changes[f]; !ok {
					t.Errorf("the change has no field %s: %+v", f, got.Change.Changes)
				}
			}

			if len(Diff(a, b)) != len(entries) {
				t.Errorf("Diff has %d changes, DiffEntries %d", len(Diff(a, b)), len(entries))
			}
		})
	}
}

func TestDiffDocumentsEqualHasNoChange(t *testing.T) {
	a := diffBase()

	if d := Diff(a, timeline.Clone(a)); len(d) != 0 {
		t.Fatalf("equal documents have changes: %+v", d)
	}

	if e := DiffEntries(a, timeline.Clone(a)); len(e) != 0 {
		t.Fatalf("equal documents have entries: %+v", e)
	}
}

// TestDiffDocumentsSignificanceOrder: tracks and whole items first, then the project and the
// settings, trims, moves, other changes, then assets.
func TestDiffDocumentsSignificanceOrder(t *testing.T) {
	a := diffBase()
	b := mutate(t, a, func(d *timeline.Document) {
		editItem(d, itemA, func(it *timeline.Item) { it.Opacity = 0.5 })                         // changed
		editItem(d, itemB, func(it *timeline.Item) { it.Start = 300 })                           // moved
		editItem(d, itemC, func(it *timeline.Item) { it.Duration, it.Words = 20, it.Words[:2] }) // trimmed
		delete(d.Items, "itm_a000000001")                                                        // removed item
		d.Name = "Renamed"                                                                       // project
		d.Tracks["trk_n000000001"] = timeline.Track{
			ID: "trk_n000000001", Kind: timeline.TrackVisual, Name: "New", Order: "a3",
		}
		d.Assets["ast_n000000001"] = timeline.Asset{
			ID: "ast_n000000001", Hash: "sha256:" + strings.Repeat("c3", 32), Kind: timeline.AssetImage,
			Name: "n.png", Width: 8, Height: 8,
		}
	})

	var order []string
	for _, e := range DiffEntries(a, b) {
		order = append(order, e.Verb+" "+e.Noun)
	}

	want := []string{
		"added track",
		"removed audio",
		"changed project",
		"trimmed caption",
		"moved text",
		"changed clip",
		"added asset",
	}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order\n got %v\nwant %v", order, want)
	}
}

// TestDiffDocumentsMatchesTheBatchDiff: the diff of the two documents is the diff that the
// operations produced, which is also what makes it correct across undo and redo.
func TestDiffDocumentsMatchesTheBatchDiff(t *testing.T) {
	a := diffBase()
	r := fullRegistry()

	res, err := r.ApplyBatch(a, []Operation{
		op2("item.move", `{"item_id":"`+itemA+`","start":"2s"}`),
		op2("item.delete", `{"item_ids":["`+itemB+`"]}`),
	}, Env{})
	if err != nil {
		t.Fatal(err)
	}

	if got := Diff(a, res.Doc); !reflect.DeepEqual(got, res.Diff) {
		t.Fatalf("Diff differs from the batch diff:\n got %+v\nwant %+v", got, res.Diff)
	}

	// Undoing the batch leaves no difference, whatever the route.
	back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
	if len(skipped) != 0 {
		t.Fatalf("undo skipped %+v", skipped)
	}

	if d := Diff(a, back.Doc); len(d) != 0 {
		t.Fatalf("after undo the documents still differ: %+v", d)
	}
}
