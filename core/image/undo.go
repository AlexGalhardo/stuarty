package image

import (
	"maps"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// UndoResult is what applying primitives made of a document.
type UndoResult struct {
	Doc     *Document
	Inverse []command.Primitive // undoes exactly what was applied
	Diff    []command.Change
}

// ApplyPrimitives applies the primitives of an undo or a redo to doc, in the given order, skipping those the
// guard rejects (another actor changed the object since) and those whose result would be invalid, and reports
// each skipped one with its reason (SDD-005 section 12.2). The surviving primitives are applied together to a
// working copy and the result is checked once; while it is invalid, every primitive that touches a layer named
// by a violation is dropped and the application starts again from doc, so it ends.
func ApplyPrimitives(doc *Document, prims []command.Primitive, guard command.Guard) (UndoResult, []command.Skipped) {
	base := Flatten(doc)
	live, skipped := filterPrimitives(base, prims, guard)

	known := baselineViolations(doc)

	var after *Document

	for {
		work := cloneFlat(base)
		for _, lp := range live {
			applyToFlat(&work, lp.p)
		}

		next, vs := finishFlat(work, known)
		if len(vs) == 0 {
			after = next

			break
		}

		touched := map[string]bool{}
		for _, v := range vs {
			if v.Layer != "" {
				touched[v.Layer] = true
			}
		}

		var kept []livePrimitive

		for _, lp := range live {
			if lp.p.Collection == CollLayers && touched[lp.p.ID] {
				skipped = append(skipped, invalidResult(lp, vs))
			} else {
				kept = append(kept, lp)
			}
		}

		if len(kept) == len(live) { // a violation no primitive explains: give up on the rest
			for _, lp := range live {
				skipped = append(skipped, invalidResult(lp, vs))
			}

			kept = nil
		}

		live = kept
		if len(live) == 0 { // nothing is left to apply: the document stays as it is, and the loop ends here
			after = Clone(doc)

			break
		}
	}

	slices.SortStableFunc(skipped, func(a, b command.Skipped) int { return a.PrimitiveIndex - b.PrimitiveIndex })

	diff, inverse := Compare(doc, after)

	return UndoResult{Doc: after, Inverse: inverse, Diff: diff}, skipped
}

type livePrimitive struct {
	index int
	p     command.Primitive
}

func invalidResult(lp livePrimitive, vs []Violation) command.Skipped {
	codes := make([]string, 0, len(vs))
	for _, v := range vs {
		codes = append(codes, v.Code+" "+v.Path)
	}

	return command.Skipped{
		PrimitiveIndex: lp.index, Primitive: lp.p, Reason: command.SkipInvalidResult,
		Details: map[string]any{"violations": codes},
	}
}

func cloneFlat(f Flat) Flat {
	out := Flat{Doc: maps.Clone(f.Doc), Layers: map[string]map[string]any{}, Assets: map[string]map[string]any{}}

	for k, v := range f.Layers {
		out.Layers[k] = maps.Clone(v)
	}

	for k, v := range f.Assets {
		out.Assets[k] = maps.Clone(v)
	}

	return out
}

// baselineViolations are the violations the document had before the undo: they are not the undo's doing, so
// they neither fail it nor make it loop.
func baselineViolations(d *Document) map[string]bool {
	known := map[string]bool{}
	for _, v := range Validate(d) {
		known[v.Code+" "+v.Path+" "+v.Layer] = true
	}

	return known
}

// finishFlat rebuilds the document and checks it, leaving out the violations in known.
func finishFlat(f Flat, known map[string]bool) (*Document, []Violation) {
	d, err := Unflatten(f)
	if err != nil {
		return nil, []Violation{{Code: "structure", Path: "layers", Message: err.Error()}}
	}

	var vs []Violation

	for _, v := range Validate(d) {
		if !known[v.Code+" "+v.Path+" "+v.Layer] {
			vs = append(vs, v)
		}
	}

	return d, vs
}

func collection(f *Flat, name string) map[string]map[string]any {
	switch name {
	case CollLayers:
		return f.Layers
	case CollAssets:
		return f.Assets
	}

	return nil
}

func applyToFlat(f *Flat, p command.Primitive) {
	if p.Collection == CollDocument {
		if p.Type == command.PrimitivePatch {
			patchFields(f.Doc, p)
		}

		return
	}

	objs := collection(f, p.Collection)
	if objs == nil {
		return
	}

	switch p.Type {
	case command.PrimitivePut:
		objs[p.ID] = maps.Clone(p.Object)
	case command.PrimitiveRemove:
		delete(objs, p.ID)
	case command.PrimitivePatch:
		if o, ok := objs[p.ID]; ok {
			patchFields(o, p)
		}
	}
}

func patchFields(obj map[string]any, p command.Primitive) {
	maps.Copy(obj, p.Set)

	for _, k := range p.Unset {
		delete(obj, k)
	}
}

func validPrimitive(p command.Primitive) bool {
	if p.ID == "" || !slices.Contains([]string{CollDocument, CollLayers, CollAssets}, p.Collection) {
		return false
	}

	switch p.Type {
	case command.PrimitivePut:
		return p.Collection != CollDocument && p.Object != nil
	case command.PrimitiveRemove:
		return p.Collection != CollDocument
	case command.PrimitivePatch:
		return len(p.Set) > 0 || len(p.Unset) > 0
	}

	return false
}

func filterPrimitives(base Flat, prims []command.Primitive, guard command.Guard) ([]livePrimitive, []command.Skipped) {
	f := &filter{guard: guard, present: map[string]bool{}}

	for id := range base.Layers {
		f.present[CollLayers+" "+id] = true
	}

	for id := range base.Assets {
		f.present[CollAssets+" "+id] = true
	}

	for i, p := range prims {
		switch {
		case !validPrimitive(p):
			f.skip(i, p, command.SkipInvalid, nil, nil)
		case p.Type == command.PrimitivePut:
			f.put(i, p)
		case p.Type == command.PrimitiveRemove:
			f.remove(i, p)
		default:
			f.patch(i, p)
		}
	}

	return f.live, f.skipped
}

// filter decides, primitive by primitive, which ones are applied. present tracks what the live primitives add
// and remove, so that a put after a remove of the same id is judged against the document as it will be.
type filter struct {
	guard   command.Guard
	present map[string]bool
	live    []livePrimitive
	skipped []command.Skipped
}

func (f *filter) skip(i int, p command.Primitive, reason string, by *command.Touch, fields []string) {
	s := command.Skipped{PrimitiveIndex: i, Primitive: p, Reason: reason, Fields: fields}
	if by != nil {
		rev := by.Revision
		s.ByRevision, s.ByActor = &rev, by.Actor
	}

	f.skipped = append(f.skipped, s)
}

func (f *filter) changed(coll, id, field string) (*command.Touch, bool) {
	if f.guard == nil {
		return nil, false
	}

	by, ok := f.guard.Changed(coll, id, field)
	if !ok {
		return nil, false
	}

	return &by, true
}

func (f *filter) put(i int, p command.Primitive) {
	key := p.Collection + " " + p.ID

	if f.present[key] {
		f.skip(i, p, command.SkipIDExists, nil, nil)

		return
	}

	if by, ok := f.changed(p.Collection, p.ID, "*"); ok {
		f.skip(i, p, command.SkipObjectChanged, by, nil)

		return
	}

	f.present[key] = true
	f.live = append(f.live, livePrimitive{i, p})
}

func (f *filter) remove(i int, p command.Primitive) {
	key := p.Collection + " " + p.ID

	if !f.present[key] {
		f.skip(i, p, command.SkipObjectMissing, nil, nil)

		return
	}

	if by, ok := f.changed(p.Collection, p.ID, ""); ok {
		f.skip(i, p, command.SkipObjectChanged, by, nil)

		return
	}

	delete(f.present, key)
	f.live = append(f.live, livePrimitive{i, p})
}

func (f *filter) patch(i int, p command.Primitive) {
	if p.Collection != CollDocument && !f.present[p.Collection+" "+p.ID] {
		f.skip(i, p, command.SkipObjectMissing, nil, nil)

		return
	}

	if by, ok := f.changed(p.Collection, p.ID, "*"); ok {
		f.skip(i, p, command.SkipObjectChanged, by, nil)

		return
	}

	rest, fields, by := splitPatch(p, func(field string) (*command.Touch, bool) {
		return f.changed(p.Collection, p.ID, field)
	})

	if len(fields) > 0 {
		f.skip(i, restOf(p, fields), command.SkipFieldChanged, by, fields)
	}

	if len(rest.Set) > 0 || len(rest.Unset) > 0 {
		f.live = append(f.live, livePrimitive{i, rest})
	}
}

// splitPatch separates the fields of a patch that another actor changed from the rest.
func splitPatch(
	p command.Primitive,
	changed func(field string) (*command.Touch, bool),
) (rest command.Primitive, fields []string, by *command.Touch) {
	rest = command.Primitive{Type: p.Type, Collection: p.Collection, ID: p.ID}

	take := func(field string) bool {
		if t, ok := changed(field); ok {
			fields = append(fields, field)

			if by == nil || t.Revision > by.Revision {
				by = t
			}

			return true
		}

		return false
	}

	for _, k := range slices.Sorted(maps.Keys(p.Set)) {
		if !take(k) {
			if rest.Set == nil {
				rest.Set = map[string]any{}
			}

			rest.Set[k] = p.Set[k]
		}
	}

	for _, k := range p.Unset {
		if !take(k) {
			rest.Unset = append(rest.Unset, k)
		}
	}

	return rest, fields, by
}

// restOf is the part of a patch that was skipped: the given fields.
func restOf(p command.Primitive, fields []string) command.Primitive {
	out := command.Primitive{Type: p.Type, Collection: p.Collection, ID: p.ID}

	for _, k := range fields {
		if v, ok := p.Set[k]; ok {
			if out.Set == nil {
				out.Set = map[string]any{}
			}

			out.Set[k] = v
		} else {
			out.Unset = append(out.Unset, k)
		}
	}

	return out
}

// Touched is a batch committed by someone else, as the guard of an undo needs it: its revision, its actor and
// its diff.
type Touched struct {
	Revision int64
	Actor    string
	Diff     []command.Change
}

type touchGuard struct{ batches []Touched }

// GuardFrom is the Guard over the batches that were committed after the step being undone: the latest one that
// changed what a primitive touches is the one reported.
func GuardFrom(batches []Touched) command.Guard { return touchGuard{batches} }

var guardKinds = map[string]string{
	CollLayers: "layer", CollAssets: "asset", CollDocument: "document",
}

func (g touchGuard) Changed(collection, id, field string) (command.Touch, bool) {
	prefix := guardKinds[collection]
	if prefix == "" {
		return command.Touch{}, false
	}

	for _, b := range slices.Backward(g.batches) {
		for _, ch := range b.Diff {
			if kindPrefix(ch.Kind) != prefix || collection != CollDocument && ch.ID != id {
				continue
			}

			whole := ch.Kind == prefix+".added" || ch.Kind == prefix+".removed"
			if field == "" || field == "*" && whole || whole {
				return command.Touch{Revision: b.Revision, Actor: b.Actor}, true
			}

			if _, ok := ch.Changes[field]; ok && field != "*" {
				return command.Touch{Revision: b.Revision, Actor: b.Actor}, true
			}
		}
	}

	return command.Touch{}, false
}

func kindPrefix(kind string) string {
	for i := range len(kind) {
		if kind[i] == '.' {
			return kind[:i]
		}
	}

	return kind
}
