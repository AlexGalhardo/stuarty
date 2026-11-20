package image

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// The collections of the flat form of a document (SDD-021 section 4).
const (
	CollDocument = "document"
	CollLayers   = "layers"
	CollAssets   = "assets"
)

// The kinds of Change of an image diff.
const (
	KindDocumentUpdated = "document.updated"
	KindLayerAdded      = "layer.added"
	KindLayerUpdated    = "layer.updated"
	KindLayerRemoved    = "layer.removed"
	KindAssetAdded      = "asset.added"
	KindAssetRemoved    = "asset.removed"
)

// Flat is a document as objects with identifiers: the fields of the document, every layer with its group in
// the field parent, and the table of assets. It is the form in which the diff, the inverse and the conflicts
// are computed, so that the unit of a change is the object (ADR-0006).
type Flat struct {
	Doc    map[string]any
	Layers map[string]map[string]any
	Assets map[string]map[string]any
}

// tree encodes any value as a decoded JSON tree whose numbers are json.Number.
func tree(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		panic("image: a value does not encode: " + err.Error())
	}

	var out any

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	if err := dec.Decode(&out); err != nil {
		panic("image: a value does not decode: " + err.Error())
	}

	return out
}

func object(v any) map[string]any {
	m, _ := tree(v).(map[string]any)
	if m == nil {
		m = map[string]any{}
	}

	return m
}

// Flatten returns the flat form of a document.
func Flatten(d *Document) Flat {
	head := *d
	head.Layers, head.Assets = nil, nil
	doc := object(head)
	delete(doc, "layers")
	delete(doc, "assets")

	f := Flat{Doc: doc, Layers: map[string]map[string]any{}, Assets: map[string]map[string]any{}}

	Walk(d, func(l, parent *Layer, _ int) {
		flat := *l
		flat.Children = nil
		m := object(flat)
		m["parent"] = ""

		if parent != nil {
			m["parent"] = parent.ID
		}

		f.Layers[l.ID] = m
	})

	for h, a := range d.Assets {
		f.Assets[h] = object(a)
	}

	return f
}

// Unflatten rebuilds a document from its flat form: the layers of a group are sorted by their order key.
func Unflatten(f Flat) (*Document, error) {
	var d Document

	if err := fromTree(f.Doc, &d); err != nil {
		return nil, err
	}

	if len(f.Assets) > 0 {
		d.Assets = map[string]AssetRef{}

		for h, m := range f.Assets {
			var a AssetRef
			if err := fromTree(m, &a); err != nil {
				return nil, err
			}

			d.Assets[h] = a
		}
	}

	byParent := map[string][]Layer{}
	seen := 0

	for id, m := range f.Layers {
		parent, _ := m["parent"].(string)
		rest := maps.Clone(m)
		delete(rest, "parent")

		var l Layer
		if err := fromTree(rest, &l); err != nil {
			return nil, err
		}

		if l.ID != id {
			return nil, fmt.Errorf("the layer stored as %s says it is %s", id, l.ID)
		}

		byParent[parent] = append(byParent[parent], l)
		seen++
	}

	var build func(parent string, depth int) ([]Layer, error)

	built := 0

	build = func(parent string, depth int) ([]Layer, error) {
		if depth > MaxDepth+1 {
			return nil, errors.New("the layers are nested too deep or loop")
		}

		ls := byParent[parent]
		sort.Slice(ls, func(i, j int) bool {
			if ls[i].Order != ls[j].Order {
				return ls[i].Order < ls[j].Order
			}

			return ls[i].ID < ls[j].ID
		})

		for i := range ls {
			built++

			kids, err := build(ls[i].ID, depth+1)
			if err != nil {
				return nil, err
			}

			ls[i].Children = kids
		}

		return ls, nil
	}

	roots, err := build("", 0)
	if err != nil {
		return nil, err
	}

	if built != seen {
		return nil, errors.New("a layer names a group that is not in the document")
	}

	d.Layers = roots
	if d.Layers == nil {
		d.Layers = []Layer{}
	}

	return &d, nil
}

func fromTree(m map[string]any, into any) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	return dec.Decode(into)
}

// jsonEqual compares two decoded JSON trees.
func jsonEqual(a, b any) bool { return reflect.DeepEqual(a, b) }

func fieldChanges(before, after map[string]any) map[string]command.Pair {
	out := map[string]command.Pair{}

	for _, k := range unionKeys(before, after) {
		b, bok := before[k]
		a, aok := after[k]

		if bok && aok && jsonEqual(b, a) {
			continue
		}

		var p command.Pair
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

func unionKeys[V any](a, b map[string]V) []string {
	seen := map[string]bool{}

	for k := range a {
		seen[k] = true
	}

	for k := range b {
		seen[k] = true
	}

	return slices.Sorted(maps.Keys(seen))
}

func patchBack(collection, id string, before, after map[string]any) (command.Primitive, bool) {
	p := command.Primitive{Type: command.PrimitivePatch, Collection: collection, ID: id}

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

// Compare returns the net difference between two documents as a diff and as the primitives that undo it
// (SDD-002 section A.5 and A.6): by object, remove what was added, put back what was removed, patch what
// changed back to its old values.
func Compare(before, after *Document) ([]command.Change, []command.Primitive) {
	fb, fa := Flatten(before), Flatten(after)

	var (
		diff    []command.Change
		inverse []command.Primitive
	)

	id := after.ID
	if id == "" {
		id = before.ID
	}

	if ch := fieldChanges(fb.Doc, fa.Doc); len(ch) > 0 {
		diff = append(diff, command.Change{Kind: KindDocumentUpdated, ID: id, Changes: ch})
	}

	if p, ok := patchBack(CollDocument, id, fb.Doc, fa.Doc); ok {
		inverse = append(inverse, p)
	}

	type group struct {
		name                    string
		before, after           map[string]map[string]any
		added, updated, removed string
	}

	for _, g := range []group{
		{CollLayers, fb.Layers, fa.Layers, KindLayerAdded, KindLayerUpdated, KindLayerRemoved},
		{CollAssets, fb.Assets, fa.Assets, KindAssetAdded, "", KindAssetRemoved},
	} {
		for _, key := range unionKeys(g.before, g.after) {
			bo, bok := g.before[key]
			ao, aok := g.after[key]

			switch {
			case !bok:
				diff = append(diff, command.Change{Kind: g.added, ID: key, Object: ao})
				inverse = append(inverse, command.Primitive{Type: command.PrimitiveRemove, Collection: g.name, ID: key})
			case !aok:
				diff = append(diff, command.Change{Kind: g.removed, ID: key, Object: bo})
				inverse = append(inverse, command.Primitive{
					Type: command.PrimitivePut, Collection: g.name, ID: key, Object: bo,
				})
			case !jsonEqual(bo, ao):
				if g.updated == "" { // an asset has no update kind
					diff = append(diff,
						command.Change{Kind: g.removed, ID: key, Object: bo},
						command.Change{Kind: g.added, ID: key, Object: ao})
					inverse = append(inverse, command.Primitive{
						Type: command.PrimitivePut, Collection: g.name, ID: key, Object: bo,
					})

					continue
				}

				diff = append(diff, command.Change{Kind: g.updated, ID: key, Changes: fieldChanges(bo, ao)})

				if p, ok := patchBack(g.name, key, bo, ao); ok {
					inverse = append(inverse, p)
				}
			}
		}
	}

	return diff, inverse
}
