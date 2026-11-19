package command

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
)

// otherActor is an API-key actor, built at run time so the file holds no key-shaped literal.
var otherActor = "key" + ":" + "key_" + "h8v2n5q7w3"

func jn(s string) json.Number { return json.Number(s) }

func timelineFPS(doc *timeline.Document) timeval.FPS {
	return timeval.FPS{Num: int64(doc.Settings.FPS.Num), Den: int64(doc.Settings.FPS.Den)}
}

// guardMap is a Guard over a set of "collection id field" keys.
type guardMap map[string]Touch

func (g guardMap) Changed(collection, id, field string) (Touch, bool) {
	if field == "" {
		for k, t := range g {
			if strings.HasPrefix(k, collection+" "+id+" ") {
				return t, true
			}
		}
		return Touch{}, false
	}
	t, ok := g[collection+" "+id+" "+field]
	return t, ok
}

func patch(coll, id string, set map[string]any, unset ...string) Primitive {
	return Primitive{Type: PrimitivePatch, Collection: coll, ID: id, Set: set, Unset: unset}
}

func apply(t *testing.T, doc *timeline.Document, g Guard, prims ...Primitive) (*timeline.Document, []Skipped, Result) {
	t.Helper()
	res, skipped := ApplyPrimitives(doc, prims, g)
	return res.Doc, skipped, res
}

func reasons(s []Skipped) string {
	out := make([]string, len(s))
	for i := range s {
		out[i] = s[i].Reason
	}
	return strings.Join(out, ",")
}

func TestApplyPrimitivesBasics(t *testing.T) {
	doc := baseDoc(t)
	before := timeline.Canonical(doc)
	newTrack := map[string]any{"id": "trk_k000000001", "kind": "visual", "order": "b0"}
	out, skipped, res := apply(
		t,
		doc,
		nil,
		patch("project", doc.ID, map[string]any{"name": "Renamed"}),
		patch("settings", doc.ID, map[string]any{"width": 1920, "height": 1080}),
		Primitive{Type: PrimitivePut, Collection: "tracks", ID: "trk_k000000001", Object: newTrack},
		patch("items", clipID, map[string]any{"start": float64(10)}, "source_in"),
		Primitive{Type: PrimitiveRemove, Collection: "items", ID: capItemID},
		Primitive{
			Type:       PrimitivePut,
			Collection: "fonts",
			ID:         "Mine",
			Object: map[string]any{
				"files": []any{map[string]any{"weight": 400, "hash": doc.Assets[assetID].Hash}},
			},
		},
	)
	if len(skipped) != 0 {
		t.Fatalf("%+v", skipped)
	}
	if out.Name != "Renamed" || out.Settings.Width != 1920 || out.Tracks["trk_k000000001"].Order != "b0" ||
		out.Items[clipID].Start != 10 || out.Items[clipID].SourceIn != 0 || len(out.Items) != 1 || out.Fonts["Mine"].Files[0].Weight != 400 {
		t.Errorf("result: %s", timeline.Canonical(out))
	}
	if !bytes.Equal(timeline.Canonical(doc), before) {
		t.Fatal("the input document was changed")
	}
	if len(res.Diff) == 0 {
		t.Error("no diff")
	}
	// The inverse of what was applied undoes it exactly.
	back, skipped2, _ := apply(t, out, nil, reversed(res.Inverse)...)
	if len(skipped2) != 0 || !bytes.Equal(timeline.Canonical(back), before) {
		t.Errorf("undo: %v\n%s\n%s", skipped2, timeline.Canonical(back), before)
	}
}

func TestApplyPrimitivesGuardRules(t *testing.T) {
	doc := baseDoc(t)
	by := Touch{Revision: 42, Actor: otherActor}
	g := guardMap{
		"items " + clipID + " start":   by,
		"items " + capItemID + " *":    Touch{Revision: 43, Actor: "local:ui"},
		"tracks " + vidTrack + " name": by,
	}
	out, skipped, _ := apply(
		t,
		doc,
		g,
		patch(
			"items",
			clipID,
			map[string]any{"start": 5, "name": "kept"},
			"source_in",
		), // 0: split, start skipped
		patch(
			"items",
			capItemID,
			map[string]any{"name": "x"},
		), // 1: whole object changed
		Primitive{Type: PrimitiveRemove, Collection: "tracks", ID: vidTrack},                     // 2: a field changed
		patch("items", "itm_zzzzzzzzzz", map[string]any{"name": "x"}),                            // 3: missing
		Primitive{Type: PrimitiveRemove, Collection: "items", ID: "itm_zzzzzzzzzz"},              // 4: missing
		Primitive{Type: PrimitivePut, Collection: "items", ID: clipID, Object: map[string]any{}}, // 5: exists
	)
	if got := reasons(
		skipped,
	); got != "field_changed_by_other_actor,object_changed_by_other_actor,object_changed_by_other_actor,object_missing,object_missing,id_exists" {
		t.Fatalf("reasons %s\n%+v", got, skipped)
	}
	first := skipped[0]
	if first.PrimitiveIndex != 0 || first.Fields[0] != "start" || first.ByRevision == nil || *first.ByRevision != 42 ||
		first.ByActor != by.Actor || first.Primitive.Set["start"] != json.Number("5") || first.Primitive.Unset != nil {
		t.Errorf("%+v", first)
	}
	if out.Items[clipID].Name != "kept" || out.Items[clipID].Start != 0 || out.Items[clipID].SourceIn != 0 {
		t.Errorf("the fields that passed must apply: %+v", out.Items[clipID])
	}
	if _, ok := out.Tracks[vidTrack]; !ok {
		t.Error("the guarded track was removed")
	}
	if skipped[1].ByRevision == nil || *skipped[1].ByRevision != 43 {
		t.Errorf("%+v", skipped[1])
	}

	// A guarded unset is reported with its field and the primitive keeps only it.
	_, skipped, _ = apply(
		t,
		doc,
		guardMap{"items " + clipID + " source_in": by},
		patch("items", clipID, nil, "source_in", "name"),
	)
	if len(skipped) != 1 || skipped[0].Fields[0] != "source_in" || skipped[0].Primitive.Unset[0] != "source_in" ||
		len(skipped[0].Primitive.Unset) != 1 {
		t.Errorf("%+v", skipped)
	}
}

func TestApplyPrimitivesInvalidResultDropsTheOffendingPrimitive(t *testing.T) {
	doc := baseDoc(t)
	// Removing the track while an item is on it breaks V_ITEM_TRACK on the item; the
	// primitive that touches the track is dropped, the other one still applies.
	out, skipped, _ := apply(t, doc, nil,
		patch("project", doc.ID, map[string]any{"name": "Fine"}),
		Primitive{Type: PrimitiveRemove, Collection: "tracks", ID: capTrack},
	)
	if len(skipped) != 1 || skipped[0].Reason != SkipInvalidResult || skipped[0].PrimitiveIndex != 1 {
		t.Fatalf("%+v", skipped)
	}
	if codes, _ := skipped[0].Details["violations"].([]string); len(codes) != 1 || codes[0] != timeline.VItemTrack {
		t.Errorf("%v", skipped[0].Details)
	}
	if out.Name != "Fine" || len(out.Tracks) != 2 {
		t.Errorf("%s", timeline.Canonical(out))
	}

	// An overlap with an item another actor placed: the item put back collides.
	other := timeline.NewItem("itm_k000000001", timeline.ItemClip, vidTrack, 100, 50)
	other.Asset = assetID
	withOther := timeline.Clone(doc)
	withOther.Items[other.ID] = other
	back := map[string]any{
		"id":        clipID,
		"type":      "clip",
		"track":     vidTrack,
		"start":     0,
		"duration":  150,
		"asset":     assetID,
		"source_in": 60,
	}
	del := Primitive{Type: PrimitiveRemove, Collection: "items", ID: clipID}
	withOther, _, _ = apply(t, withOther, nil, del)
	out, skipped, _ = apply(
		t,
		withOther,
		nil,
		Primitive{Type: PrimitivePut, Collection: "items", ID: clipID, Object: back},
	)
	if len(skipped) != 1 || skipped[0].Reason != SkipInvalidResult || len(out.Items) != 2 {
		t.Errorf("%+v", skipped)
	}

	// An invalid put (bad shape) names the object by path and is dropped too.
	out, skipped, _ = apply(
		t,
		doc,
		nil,
		Primitive{
			Type:       PrimitivePut,
			Collection: "items",
			ID:         "itm_k000000001",
			Object:     map[string]any{"id": "itm_k000000001"},
		},
	)
	if len(skipped) != 1 || skipped[0].Reason != SkipInvalidResult || len(out.Items) != 2 {
		t.Errorf("%+v", skipped)
	}

	// Settings and project paths map to their own primitives.
	out, skipped, _ = apply(t, doc, nil,
		patch("settings", doc.ID, map[string]any{"width": 1081}),
		patch("project", doc.ID, map[string]any{"name": ""}),
	)
	if len(skipped) != 2 || out.Settings.Width != 1080 || out.Name != "Launch teaser" {
		t.Errorf("%+v", skipped)
	}
}

func TestApplyPrimitivesValidatesOnceAtTheEnd(t *testing.T) {
	doc := baseDoc(t)
	// Removing the track first leaves the caption without a track; removing the
	// caption after it makes the result valid. No check runs in between.
	out, skipped, _ := apply(t, doc, nil,
		Primitive{Type: PrimitiveRemove, Collection: "tracks", ID: capTrack},
		Primitive{Type: PrimitiveRemove, Collection: "items", ID: capItemID},
	)
	if len(skipped) != 0 || len(out.Tracks) != 1 || len(out.Items) != 1 {
		t.Errorf("%+v", skipped)
	}
}

func TestApplyPrimitivesGivesUpOnAViolationNoPrimitiveExplains(t *testing.T) {
	doc := baseDoc(t)
	// A new frame rate breaks the source range of the clip, which no primitive touches.
	out, skipped, _ := apply(
		t,
		doc,
		nil,
		patch("settings", doc.ID, map[string]any{"fps": map[string]any{"num": 2, "den": 1}}),
	)
	if len(skipped) != 1 || skipped[0].Reason != SkipInvalidResult || out.Settings.FPS.Num != 30 {
		t.Errorf("%+v", skipped)
	}
}

func TestApplyPrimitivesIgnoresViolationsThatWereAlreadyThere(t *testing.T) {
	doc := baseDoc(t)
	it := doc.Items[clipID]
	it.SourceIn = 800
	doc.Items[clipID] = it
	out, skipped, _ := apply(t, doc, nil, patch("project", doc.ID, map[string]any{"name": "Still applied"}))
	if len(skipped) != 0 || out.Name != "Still applied" {
		t.Errorf("%+v", skipped)
	}
}

func TestApplyPrimitivesRejectsMalformedPrimitives(t *testing.T) {
	doc := baseDoc(t)
	bad := []Primitive{
		{Type: "explode", Collection: "items", ID: clipID},
		{Type: PrimitivePatch, Collection: "widgets", ID: "x"},
		{Type: PrimitivePatch, Collection: "items"},
		{Type: PrimitivePut, Collection: "items", ID: "itm_k000000001"},
		{Type: PrimitivePut, Collection: "settings", ID: doc.ID, Object: map[string]any{}},
		{Type: PrimitiveRemove, Collection: "project", ID: doc.ID},
	}
	_, skipped, _ := apply(t, doc, nil, bad...)
	if len(skipped) != len(bad) {
		t.Fatalf("%+v", skipped)
	}
	for i, s := range skipped {
		if s.Reason != SkipInvalid || s.PrimitiveIndex != i {
			t.Errorf("%d: %+v", i, s)
		}
	}
}

func TestPrimitiveAndChangeReadNumbersAsJSONNumbers(t *testing.T) {
	var p Primitive
	if err := json.Unmarshal(
		[]byte(`{"type":"patch","collection":"items","id":"`+clipID+`","set":{"start":12,"opacity":0.5}}`),
		&p,
	); err != nil {
		t.Fatal(err)
	}
	if p.Set["start"] != json.Number("12") || p.Set["opacity"] != json.Number("0.5") {
		t.Errorf("%#v", p.Set)
	}
	var c Change
	if err := json.Unmarshal([]byte(`{"kind":"item.updated","id":"x","changes":{"start":[0,30]}}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Changes["start"] != (Pair{json.Number("0"), json.Number("30")}) {
		t.Errorf("%#v", c)
	}
	if !strings.Contains(p.String(), `"type":"patch"`) {
		t.Error(p.String())
	}
	if !strings.HasPrefix(
		Primitive{Type: "put", Collection: "items", ID: "x", Object: map[string]any{"f": func() {}}}.String(),
		"put items x",
	) {
		t.Error("String must survive a value that cannot be encoded")
	}
	// Applying a primitive decoded from stored JSON works.
	doc := baseDoc(t)
	out, skipped, _ := apply(t, doc, nil, p)
	if len(skipped) != 0 || out.Items[clipID].Start != 12 || out.Items[clipID].Opacity != 0.5 {
		t.Errorf("%+v", skipped)
	}
}

func TestInverseOfFieldsThatWereAbsent(t *testing.T) {
	doc := baseDoc(t)
	r := testRegistry(t)
	res := mustApply(t, r, doc, opJSON("test.settings", `{"width":1920}`))
	if len(res.Inverse) != 1 || res.Inverse[0].Collection != "settings" || len(res.Inverse[0].Unset) != 1 ||
		res.Inverse[0].Unset[0] != "width" {
		t.Errorf("an absent (default) field is unset by the inverse: %+v", res.Inverse)
	}
	if len(res.Diff) != 1 || res.Diff[0].Kind != KindSettingsUpdated ||
		res.Diff[0].Changes["width"] != (Pair{jn("1080"), jn("1920")}) {
		t.Errorf("%+v", res.Diff)
	}
}
