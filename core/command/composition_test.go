package command

import (
	"bytes"
	"maps"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// production is the graph of a test: the document being edited and the other timelines.
func production(host *timeline.Document, others ...*timeline.Document) caseGraph {
	g := caseGraph{host.ID: host}
	for _, o := range others {
		g[o.ID] = o
	}

	return g
}

func batch(t *testing.T, doc *timeline.Document, env Env, ops ...Operation) BatchResult {
	t.Helper()

	res, err := fullRegistry().ApplyBatch(doc, ops, env)
	if err != nil {
		t.Fatalf("%v: %v", ops, err)
	}

	return res
}

// SDD-002 C.85: create then undo gives back the original items, byte for byte (epic 32 criterion 5).
func TestCompositionCreateThenUndoRestoresTheItems(t *testing.T) {
	t.Parallel()

	doc := base().
		clip(itemA, vidTr, 30, 90, 0, func(i *timeline.Item) { i.Opacity, i.Brightness = 0.7, 0.2 }).
		text(itemB, capTr, 40, 60).
		audio(itemC, 30, 90, 0).d
	before := timeline.Canonical(doc)

	env := Env{NewIDs: (&seqIDs{}).next, Timelines: production(doc)}
	res := batch(
		t,
		doc,
		env,
		opJSON(
			"composition.create_from_selection",
			`{"item_ids":["`+itemA+`","`+itemB+`","`+itemC+`"],"name":"Intro"}`,
		),
	)

	if len(res.Spawned) != 1 || len(res.Spawned[0].Items) != 3 {
		t.Fatalf("spawned %d documents, want one holding the 3 items", len(res.Spawned))
	}

	if n := len(res.Doc.Items); n != 1 {
		t.Fatalf("the document holds %d items after the move, want the composition", n)
	}

	back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
	if len(skipped) != 0 || !bytes.Equal(timeline.Canonical(back.Doc), before) {
		t.Fatalf("undo did not restore the items exactly (skipped %v)", skipped)
	}
}

// The new timeline holds the items as they were, from frame 0, and the composition shows all of it: the two
// together draw what the items drew.
func TestCompositionCreateKeepsTheItemsAndTheirPlace(t *testing.T) {
	t.Parallel()

	doc := base().clip(itemA, vidTr, 100, 60, 10).text(itemB, capTr, 130, 60).d
	res := batch(
		t,
		doc,
		Env{NewIDs: (&seqIDs{}).next, Timelines: production(doc)},
		opJSON(
			"composition.create_from_selection",
			`{"item_ids":["`+itemA+`","`+itemB+`"],"timeline_id":"`+tlNew+`","id":"`+itemK+`"}`,
		),
	)

	inner := res.Spawned[0]
	if inner.ID != tlNew || inner.Items[itemA].Start != 0 || inner.Items[itemB].Start != 30 {
		t.Errorf(
			"the new timeline is %s with items at %d and %d, want %s at 0 and 30",
			inner.ID,
			inner.Items[itemA].Start,
			inner.Items[itemB].Start,
			tlNew,
		)
	}

	if inner.Items[itemA].SourceIn != 10 || inner.Items[itemA].Asset != vidAsset {
		t.Errorf("the clip lost its source: %+v", inner.Items[itemA])
	}

	if _, ok := inner.Assets[vidAsset]; !ok || len(inner.Assets) != 1 {
		t.Errorf(
			"the new timeline holds assets %v, want only the one the clip uses",
			slices.Sorted(maps.Keys(inner.Assets)),
		)
	}

	if vs := timeline.Validate(inner); len(vs) != 0 {
		t.Errorf("the new timeline is not valid: %v", vs)
	}

	c := res.Doc.Items[itemK]
	if c.Type != timeline.ItemComposition || c.Timeline != tlNew || c.Start != 100 || c.Duration != 90 ||
		c.Track != capTr {
		t.Errorf("the composition is %+v, want %s from 100 for 90 frames on the top track", c, tlNew)
	}
}

// Expand is the inverse of create for a composition that only places its timeline: the items come back with
// their ids, their frames and their properties, on tracks of their own.
func TestCompositionExpandUndoesCreate(t *testing.T) {
	t.Parallel()

	doc := base().clip(itemA, vidTr, 30, 90, 5, func(i *timeline.Item) { i.Contrast = 1.3 }).
		text(itemB, capTr, 50, 40).
		d
	ids := &seqIDs{}

	made := batch(
		t,
		doc,
		Env{NewIDs: ids.next, Timelines: production(doc)},
		opJSON(
			"composition.create_from_selection",
			`{"item_ids":["`+itemA+`","`+itemB+`"],"timeline_id":"`+tlNew+`","id":"`+itemK+`"}`,
		),
	)

	expanded := batch(t, made.Doc, Env{NewIDs: ids.next, Timelines: production(made.Doc, made.Spawned[0])},
		opJSON("composition.expand", `{"item_id":"`+itemK+`"}`))

	if _, ok := expanded.Doc.Items[itemK]; ok {
		t.Error("the composition is still there after expand")
	}

	for _, id := range []string{itemA, itemB} {
		got, want := expanded.Doc.Items[id], doc.Items[id]
		got.Track, want.Track = "", ""

		if !bytes.Equal(canonicalItem(t, got), canonicalItem(t, want)) {
			t.Errorf("item %s came back as %+v, want %+v", id, got, want)
		}
	}

	if vs := timeline.Validate(expanded.Doc); len(vs) != 0 {
		t.Errorf("the expanded document is not valid: %v", vs)
	}

	// The relative order of the two tracks is kept: the clip's track (below) under the text's.
	a, b := expanded.Doc.Tracks[expanded.Doc.Items[itemA].Track], expanded.Doc.Tracks[expanded.Doc.Items[itemB].Track]
	if b.Order >= a.Order {
		t.Errorf("the text track %q is not above the clip track %q", b.Order, a.Order)
	}
}

func canonicalItem(t *testing.T, it timeline.Item) []byte {
	t.Helper()

	d := base().d
	d.Items = map[string]timeline.Item{it.ID: it}
	d.Tracks = map[string]timeline.Track{}

	return timeline.Canonical(d)
}

// The nesting rules hold for what create makes: a selection that holds a composition nests one deeper.
func TestCompositionCreateKeepsTheNestingWithinTheLimit(t *testing.T) {
	t.Parallel()

	doc := base().with(compItem(tlB, 0, 100)).d
	ok := chain(tlB, tlC, tlD)
	env := Env{NewIDs: (&seqIDs{}).next, Timelines: production(doc, ok...)}

	// B holds C holds D (depth 2): the new timeline is depth 3, the host depth 4.
	batch(t, doc, env, opJSON("composition.create_from_selection", `{"item_ids":["`+itemA+`"]}`))

	deep := chain(tlB, tlC, tlD, tlE)
	env.Timelines = production(doc, deep...)

	if _, err := fullRegistry().ApplyBatch(doc, []Operation{opJSON("composition.create_from_selection", `{"item_ids":["`+itemA+`"]}`)}, env); err == nil {
		t.Fatal("a nesting of depth 5 was accepted")
	} else if e := AsError(
		err,
	); e.Code != CodeLimit ||
		e.Details["reason"] != reasonDepth {
		t.Fatalf("got %s %v, want E_LIMIT composition_depth", e.Code, e.Details)
	}
}

// What expand adds, create can take away again: after expand the nested timeline is no longer used by the
// document, and a second expand of the same item is a not-found.
func TestCompositionExpandTwiceIsNotFound(t *testing.T) {
	t.Parallel()

	doc := base().with(compItem(tlB, 0, 300)).d
	env := Env{NewIDs: (&seqIDs{}).next, Timelines: production(doc, nestedTL(tlB, 300))}
	res := batch(t, doc, env, opJSON("composition.expand", `{"item_id":"`+itemA+`"}`))

	_, err := fullRegistry().ApplyBatch(res.Doc, []Operation{opJSON("composition.expand", `{"item_id":"`+itemA+`"}`)}, env)
	if err == nil || AsError(err).Code != CodeNotFound {
		t.Fatalf("a second expand answered %v, want E_NOT_FOUND", err)
	}
}

// CompositionFeature and the planner agree on what a plain composition is: every property that the planner
// groups is named, so expand refuses what flattening could not have drawn alike.
func TestCompositionFeatureNamesWhatMakesAGroup(t *testing.T) {
	t.Parallel()

	x := 10.0
	mods := map[string]func(*timeline.Item){
		"opacity":    func(i *timeline.Item) { i.Opacity = 0.5 },
		"scale":      func(i *timeline.Item) { i.Scale = 0.5 },
		"rotation":   func(i *timeline.Item) { i.Rotation = 3 },
		"position":   func(i *timeline.Item) { i.X = &x },
		"keyframes":  func(i *timeline.Item) { i.Keyframes = timeline.Keyframes{"scale": {{T: 0, Value: 1}}} },
		"transition": func(i *timeline.Item) { i.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 5} },
		"mask":       func(i *timeline.Item) { i.Mask = &timeline.Mask{Shape: "rect", Width: 1, Height: 1} },
		"effects":    func(i *timeline.Item) { i.Effects = []timeline.Effect{{Kind: "vignette"}} },
		"shadow":     func(i *timeline.Item) { i.Shadow = &timeline.Shadow{Color: "#000000"} },
		"outline":    func(i *timeline.Item) { i.Outline = &timeline.Outline{Color: "#000000", Width: 2} },
		"blur":       func(i *timeline.Item) { i.Blur = 2 },
		"filter":     func(i *timeline.Item) { i.Filter = "sepia" },
		"blend":      func(i *timeline.Item) { i.Blend = "add" },
		"flip":       func(i *timeline.Item) { i.FlipY = true },
	}

	plain := timeline.NewItem(itemA, timeline.ItemComposition, vidTr, 0, 10)
	if f := timeline.CompositionFeature(&plain); f != "" {
		t.Fatalf("a plain composition has the feature %q", f)
	}

	for name, mod := range mods {
		it := plain
		mod(&it)

		if f := timeline.CompositionFeature(&it); f != name {
			t.Errorf("%s: CompositionFeature = %q", name, f)
		}
	}
}
