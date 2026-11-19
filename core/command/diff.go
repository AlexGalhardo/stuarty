package command

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Kinds of Change (SDD-002 §A.5).
const (
	KindProjectUpdated  = "project.updated"
	KindSettingsUpdated = "settings.updated"
	KindAssetAdded      = "asset.added"
	KindAssetRemoved    = "asset.removed"
	KindTrackAdded      = "track.added"
	KindTrackUpdated    = "track.updated"
	KindTrackRemoved    = "track.removed"
	KindItemAdded       = "item.added"
	KindItemUpdated     = "item.updated"
	KindItemRemoved     = "item.removed"
	KindFontAdded       = "font.added"
	KindFontRemoved     = "font.removed"
)

// Pair is [before, after] of one changed field; an absent side is null.
type Pair [2]any

// Change is one entry of a diff. Values are decoded JSON in canonical form (a
// field equal to its default is absent, so it reads as null).
type Change struct {
	Kind    string          `json:"kind"`
	ID      string          `json:"id"`
	Changes map[string]Pair `json:"changes,omitempty"`
	Object  map[string]any  `json:"object,omitempty"`
}

// UnmarshalJSON reads numbers as json.Number so that a stored diff compares equal
// to a computed one.
func (c *Change) UnmarshalJSON(data []byte) error {
	type plain Change
	return decodeNumbers(data, (*plain)(c))
}

func decodeNumbers(data []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(into)
}

// collections are the maps of the document whose objects have an identifier; the
// diff and the primitives address them by name.
var collections = []string{"assets", "tracks", "items", "fonts"}

// reserved are the top-level keys of a document that are not project fields.
var reserved = map[string]bool{"assets": true, "tracks": true, "items": true, "fonts": true, "settings": true}

// jsonEqual compares two decoded JSON values (the trees of timeline.Raw). It has the
// result of reflect.DeepEqual on them without its per-call reflection cost.
func jsonEqual(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case string:
		y, ok := b.(string)
		return ok && x == y
	case json.Number:
		y, ok := b.(json.Number)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || (x == nil) != (y == nil) || len(x) != len(y) {
			return false
		}
		for k, xv := range x {
			yv, ok := y[k]
			if !ok || !jsonEqual(xv, yv) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || (x == nil) != (y == nil) || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !jsonEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}

// rawPair returns the canonical trees of before and after for diffOf and inverseOf.
// An object of a collection that is equal in both documents is left out of both
// trees, which is what those functions would conclude from it anyway; only the
// objects that differ pay for the encoding.
func rawPair(before, after *timeline.Document) (timeline.RawDoc, timeline.RawDoc) {
	return pairWith(before, after, timeline.Raw, timeline.RawValue)
}

// fullPair is rawPair with every default written out, the form of a fetched
// document: the diff is computed on it, so an added object and both sides of a
// changed field read as the client stores them (SDD-002 §A.5).
func fullPair(before, after *timeline.Document) (timeline.RawDoc, timeline.RawDoc) {
	return pairWith(before, after, timeline.Full, timeline.FullValue)
}

func pairWith(
	before, after *timeline.Document,
	doc func(*timeline.Document) timeline.RawDoc,
	value func(any) any,
) (timeline.RawDoc, timeline.RawDoc) {
	rb, ra := doc(withoutCollections(before)), doc(withoutCollections(after))
	addChanged(rb, ra, "assets", before.Assets, after.Assets, value)
	addChanged(rb, ra, "tracks", before.Tracks, after.Tracks, value)
	addChanged(rb, ra, "items", before.Items, after.Items, value)
	addChanged(rb, ra, "fonts", before.Fonts, after.Fonts, value)
	return rb, ra
}

// withoutCollections is a shallow copy of doc with the collections emptied, so that
// encoding it yields the project fields and the settings only.
func withoutCollections(doc *timeline.Document) *timeline.Document {
	c := *doc
	c.Assets, c.Tracks, c.Items, c.Fonts = nil, nil, nil, nil
	return &c
}

func addChanged[V any](rb, ra timeline.RawDoc, name string, before, after map[string]V, value func(any) any) {
	bm, am := map[string]any{}, map[string]any{}
	for id, bv := range before {
		if av, ok := after[id]; ok && reflect.DeepEqual(bv, av) {
			continue
		}
		bm[id] = value(bv)
	}
	for id, av := range after {
		if bv, ok := before[id]; ok && reflect.DeepEqual(bv, av) {
			continue
		}
		am[id] = value(av)
	}
	rb[name], ra[name] = bm, am
}

func objects(doc timeline.RawDoc, collection string) map[string]any {
	m, _ := doc[collection].(map[string]any)
	return m
}

func objectOf(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func projectFields(doc timeline.RawDoc) map[string]any {
	out := map[string]any{}
	for k, v := range doc {
		if !reserved[k] {
			out[k] = v
		}
	}
	return out
}

func projectID(docs ...timeline.RawDoc) string {
	for _, d := range docs {
		if id, ok := d["id"].(string); ok {
			return id
		}
	}
	return ""
}

func unionKeys(a, b map[string]any) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func fieldChanges(before, after map[string]any) map[string]Pair {
	out := map[string]Pair{}
	for _, k := range unionKeys(before, after) {
		b, bok := before[k]
		a, aok := after[k]
		if bok && aok && jsonEqual(b, a) {
			continue
		}
		var p Pair
		if bok {
			p[0] = b
		}
		if aok {
			p[1] = a
		}
		out[k] = p
	}
	return out
}

var singular = map[string]string{"assets": "asset", "tracks": "track", "items": "item", "fonts": "font"}

// diffOf is the net difference between two documents: project fields, settings,
// then assets, tracks, items and fonts, each by identifier.
func diffOf(before, after timeline.RawDoc) []Change {
	var out []Change
	id := projectID(after, before)
	if ch := fieldChanges(projectFields(before), projectFields(after)); len(ch) > 0 {
		out = append(out, Change{Kind: KindProjectUpdated, ID: id, Changes: ch})
	}
	if ch := fieldChanges(objectOf(before["settings"]), objectOf(after["settings"])); len(ch) > 0 {
		out = append(out, Change{Kind: KindSettingsUpdated, ID: id, Changes: ch})
	}
	for _, coll := range collections {
		b, a := objects(before, coll), objects(after, coll)
		for _, key := range unionKeys(b, a) {
			bo, bok := b[key]
			ao, aok := a[key]
			kind := singular[coll]
			switch {
			case !bok:
				out = append(out, Change{Kind: kind + ".added", ID: key, Object: objectOf(ao)})
			case !aok:
				out = append(out, Change{Kind: kind + ".removed", ID: key, Object: objectOf(bo)})
			case !jsonEqual(bo, ao):
				if coll == "fonts" { // a font has no update kind
					out = append(out,
						Change{Kind: KindFontRemoved, ID: key, Object: objectOf(bo)},
						Change{Kind: KindFontAdded, ID: key, Object: objectOf(ao)})
					continue
				}
				out = append(
					out,
					Change{Kind: kind + ".updated", ID: key, Changes: fieldChanges(objectOf(bo), objectOf(ao))},
				)
			}
		}
	}
	return out
}

// patchBack returns the primitive that restores the fields of after to before.
func patchBack(collection, id string, before, after map[string]any) (Primitive, bool) {
	p := Primitive{Type: PrimitivePatch, Collection: collection, ID: id}
	for k := range fieldChanges(before, after) {
		if v, ok := before[k]; ok {
			if p.Set == nil {
				p.Set = map[string]any{}
			}
			p.Set[k] = v
		} else {
			p.Unset = append(p.Unset, k)
		}
	}
	slices.Sort(p.Unset)
	return p, p.Set != nil || p.Unset != nil
}

// inverseOf computes the primitives that undo the change from before to after
// (SDD-002 §A.6): remove what was added, put back what was removed, patch what was
// updated back to its old values (a field that was absent is unset).
func inverseOf(before, after timeline.RawDoc) []Primitive {
	var out []Primitive
	id := projectID(after, before)
	if p, ok := patchBack("project", id, projectFields(before), projectFields(after)); ok {
		out = append(out, p)
	}
	if p, ok := patchBack("settings", id, objectOf(before["settings"]), objectOf(after["settings"])); ok {
		out = append(out, p)
	}
	for _, coll := range collections {
		b, a := objects(before, coll), objects(after, coll)
		for _, key := range unionKeys(b, a) {
			bo, bok := b[key]
			ao, aok := a[key]
			switch {
			case !bok:
				out = append(out, Primitive{Type: PrimitiveRemove, Collection: coll, ID: key})
			case !aok:
				out = append(out, Primitive{Type: PrimitivePut, Collection: coll, ID: key, Object: objectOf(bo)})
			default:
				if p, ok := patchBack(coll, key, objectOf(bo), objectOf(ao)); ok {
					out = append(out, p)
				}
			}
		}
	}
	return out
}

// cloneMap deep-copies a decoded JSON object.
func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, e := range m {
		out[k] = cloneTree(e)
	}
	return out
}

// cloneTree deep-copies a decoded JSON value.
func cloneTree(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = cloneTree(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cloneTree(e)
		}
		return out
	default:
		return v
	}
}

// normalizeTree re-reads a value through JSON so that numbers are json.Number, the
// form of a document tree, whatever produced the value (float64 from a plain decode).
func normalizeTree(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := decodeNumbers(b, &out); err != nil {
		return v
	}
	return out
}

func collectionOfPrefix(id string) string {
	switch {
	case strings.HasPrefix(id, timeline.PrefixItem+"_"):
		return "items"
	case strings.HasPrefix(id, timeline.PrefixTrack+"_"):
		return "tracks"
	case strings.HasPrefix(id, timeline.PrefixAsset+"_"):
		return "assets"
	}
	return ""
}
