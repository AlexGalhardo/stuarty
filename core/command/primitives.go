package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// The three primitives of SDD-002 §A.6.
const (
	PrimitivePatch  = "patch"
	PrimitivePut    = "put"
	PrimitiveRemove = "remove"
)

// Reasons for which a primitive is skipped (SDD-005 §12.2).
const (
	SkipFieldChanged  = "field_changed_by_other_actor"
	SkipObjectChanged = "object_changed_by_other_actor"
	SkipObjectMissing = "object_missing"
	SkipIDExists      = "id_exists"
	SkipInvalidResult = "invalid_result"
	SkipInvalid       = "invalid_primitive"
)

// Primitive is one step of an undo: patch sets and unsets top-level fields of an
// object, put inserts an object that was removed, remove deletes one that was
// created. Collection is settings, project, assets, tracks, items or fonts; for
// settings and project the ID is the project identifier, for fonts the family.
type Primitive struct {
	Type       string         `json:"type"`
	Collection string         `json:"collection"`
	ID         string         `json:"id"`
	Set        map[string]any `json:"set,omitempty"`
	Unset      []string       `json:"unset,omitempty"`
	Object     map[string]any `json:"object,omitempty"`
}

// UnmarshalJSON reads numbers as json.Number, the form of a document tree.
func (p *Primitive) UnmarshalJSON(data []byte) error {
	type plain Primitive
	return decodeNumbers(data, (*plain)(p))
}

// Touch says who changed something and in which revision.
type Touch struct {
	Revision int64
	Actor    string
}

// Guard says whether another actor changed what a primitive touches. The field ""
// means any field and "*" means the whole object was added or removed.
type Guard interface {
	Changed(collection, id, field string) (by Touch, changed bool)
}

// Skipped is a primitive that was not applied (SDD-005 §12.3).
type Skipped struct {
	PrimitiveIndex int            `json:"primitive_index"`
	Primitive      Primitive      `json:"primitive"`
	Fields         []string       `json:"fields,omitempty"`
	Reason         string         `json:"reason"`
	ByRevision     *int64         `json:"by_revision,omitempty"`
	ByActor        string         `json:"by_actor,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
}

type livePrimitive struct {
	index int
	p     Primitive
}

// ApplyPrimitives applies primitives in the given order to doc, skipping those the
// guard rejects or whose result would break the document. The surviving primitives
// are applied together to a working copy with no validation in between, and the
// result is validated once; while it reports violations, every primitive that
// touches an object named in one is dropped and the application starts again from
// doc. Each round drops at least one primitive, so it ends. Result.Inverse undoes
// exactly what was applied.
func ApplyPrimitives(doc *timeline.Document, prims []Primitive, guard Guard) (Result, []Skipped) {
	base := timeline.Raw(doc)
	known := baselineViolations(doc)
	live, skipped := filterPrimitives(base, prims, guard)

	var after *timeline.Document
	for {
		work := cloneMap(base)
		for _, lp := range live {
			applyToTree(work, lp.p)
		}
		newDoc, vs := finish(work, known)
		if len(vs) == 0 {
			after = newDoc
			break
		}
		touched := touchedObjects(work, vs)
		var kept []livePrimitive
		for _, lp := range live {
			if touched[objectKey(base, lp.p)] {
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
	}
	slices.SortStableFunc(skipped, func(a, b Skipped) int { return a.PrimitiveIndex - b.PrimitiveIndex })
	return Result{
		Doc:     after,
		Inverse: inverseOf(base, timeline.Raw(after)),
		Diff:    diffOf(fullPair(doc, after)),
	}, skipped
}

func invalidResult(lp livePrimitive, vs []timeline.Violation) Skipped {
	codes := make([]string, 0, len(vs))
	for _, v := range vs {
		if !slices.Contains(codes, v.Code) {
			codes = append(codes, v.Code)
		}
	}
	return Skipped{
		PrimitiveIndex: lp.index,
		Primitive:      lp.p,
		Reason:         SkipInvalidResult,
		Details:        map[string]any{"violations": codes},
	}
}

func baselineViolations(doc *timeline.Document) map[string]bool {
	known := map[string]bool{}
	for _, v := range timeline.Validate(doc) {
		known[v.Code+" "+v.Path] = true
	}
	return known
}

// finish types a tree and returns the violations that are new, that is, not already
// in the document the primitives started from.
func finish(tree map[string]any, known map[string]bool) (*timeline.Document, []timeline.Violation) {
	doc, err := timeline.FromRaw(timeline.RawDoc(tree))
	var all []timeline.Violation
	if err != nil {
		if ve, ok := errors.AsType[*timeline.ViolationsError](err); ok {
			all = ve.Violations
		}
	} else {
		all = timeline.Validate(doc)
	}
	var fresh []timeline.Violation
	for _, v := range all {
		if !known[v.Code+" "+v.Path] {
			fresh = append(fresh, v)
		}
	}
	return doc, fresh
}

// touchedObjects maps violations to the objects they name: the object at the path
// and the objects the violation says it involves.
func touchedObjects(tree map[string]any, vs []timeline.Violation) map[string]bool {
	out := map[string]bool{}
	id := projectID(timeline.RawDoc(tree))
	for _, v := range vs {
		parts := strings.Split(v.Path, ".")
		switch {
		case slices.Contains(collections, parts[0]) && len(parts) > 1:
			out[parts[0]+" "+strings.SplitN(parts[1], "[", 2)[0]] = true
		case parts[0] == "settings":
			out["settings "+id] = true
		default:
			out["project "+id] = true
		}
		for _, rel := range v.Related {
			if c := collectionOfPrefix(rel); c != "" {
				out[c+" "+rel] = true
			}
		}
	}
	return out
}

func objectKey(base timeline.RawDoc, p Primitive) string {
	if p.Collection == "settings" || p.Collection == "project" {
		return p.Collection + " " + projectID(base)
	}
	return p.Collection + " " + p.ID
}

// filter applies the rules that need no validation (SDD-005 §12.2). It walks the
// primitives in order against a scratch copy of the tree, so a put makes a later
// patch of the same object possible.
type filter struct {
	base    timeline.RawDoc
	scratch map[string]any
	guard   Guard
	live    []livePrimitive
	skipped []Skipped
}

func filterPrimitives(base timeline.RawDoc, prims []Primitive, guard Guard) ([]livePrimitive, []Skipped) {
	f := &filter{base: base, scratch: cloneMap(base), guard: guard}
	for i, p := range prims {
		p.Set, _ = normalizeTree(p.Set).(map[string]any)
		p.Object, _ = normalizeTree(p.Object).(map[string]any)
		f.one(i, p)
	}
	return f.live, f.skipped
}

func (f *filter) skip(i int, p Primitive, reason string, by *Touch, fields []string) {
	s := Skipped{PrimitiveIndex: i, Primitive: p, Reason: reason, Fields: fields}
	if by != nil {
		rev := by.Revision
		s.ByRevision, s.ByActor = &rev, by.Actor
	}
	f.skipped = append(f.skipped, s)
}

func (f *filter) changed(coll, id, field string) (*Touch, bool) {
	if f.guard == nil {
		return nil, false
	}
	if by, ok := f.guard.Changed(coll, id, field); ok {
		return &by, true
	}
	return nil, false
}

func (f *filter) keep(i int, p Primitive) {
	applyToTree(f.scratch, p)
	f.live = append(f.live, livePrimitive{index: i, p: p})
}

func (f *filter) one(i int, p Primitive) {
	if !validPrimitive(p) {
		f.skip(i, p, SkipInvalid, nil, nil)
		return
	}
	id := p.ID
	if p.Collection == "settings" || p.Collection == "project" {
		id = projectID(f.base)
	}
	exists := p.Collection == "settings" || p.Collection == "project" || objects(f.scratch, p.Collection)[id] != nil
	switch p.Type {
	case PrimitivePut:
		if exists {
			f.skip(i, p, SkipIDExists, nil, nil)
			return
		}
	case PrimitiveRemove:
		if !exists {
			f.skip(i, p, SkipObjectMissing, nil, nil)
			return
		}
		if by, ok := f.changed(p.Collection, id, ""); ok {
			f.skip(i, p, SkipObjectChanged, by, nil)
			return
		}
	case PrimitivePatch:
		f.patch(i, p, id, exists)
		return
	}
	f.keep(i, p)
}

// patch splits a patch: the fields another actor changed are skipped, the others
// applied.
func (f *filter) patch(i int, p Primitive, id string, exists bool) {
	if !exists {
		f.skip(i, p, SkipObjectMissing, nil, nil)
		return
	}
	if by, ok := f.changed(p.Collection, id, "*"); ok {
		f.skip(i, p, SkipObjectChanged, by, nil)
		return
	}
	rest, rejected, by := splitPatch(p, func(field string) (*Touch, bool) { return f.changed(p.Collection, id, field) })
	if len(rejected) > 0 {
		f.skip(i, restOf(p, rejected), SkipFieldChanged, by, rejected)
	}
	if rest.Set != nil || rest.Unset != nil {
		f.keep(i, rest)
	}
}

// restOf is the part of a patch that touches only the given fields.
func restOf(p Primitive, fields []string) Primitive {
	out := Primitive{Type: p.Type, Collection: p.Collection, ID: p.ID}
	for _, field := range fields {
		if v, ok := p.Set[field]; ok {
			if out.Set == nil {
				out.Set = map[string]any{}
			}
			out.Set[field] = v
		} else {
			out.Unset = append(out.Unset, field)
		}
	}
	return out
}

func validPrimitive(p Primitive) bool {
	switch p.Collection {
	case "settings", "project":
		return p.Type == PrimitivePatch
	case "assets", "tracks", "items", "fonts":
	default:
		return false
	}
	switch p.Type {
	case PrimitivePatch, PrimitivePut, PrimitiveRemove:
		return p.ID != "" && (p.Type != PrimitivePut || p.Object != nil)
	}
	return false
}

// splitPatch returns the patch without the fields another actor changed, those
// fields sorted, and who changed the first of them.
func splitPatch(p Primitive, changed func(field string) (*Touch, bool)) (Primitive, []string, *Touch) {
	out := Primitive{Type: p.Type, Collection: p.Collection, ID: p.ID}
	var rejected []string
	var by *Touch
	fields := slices.Sorted(maps.Keys(p.Set))
	fields = append(fields, p.Unset...)
	for _, f := range fields {
		if t, ok := changed(f); ok {
			rejected = append(rejected, f)
			if by == nil {
				by = t
			}
			continue
		}
		if v, ok := p.Set[f]; ok {
			if out.Set == nil {
				out.Set = map[string]any{}
			}
			out.Set[f] = v
		} else {
			out.Unset = append(out.Unset, f)
		}
	}
	return out, rejected, by
}

// applyToTree applies one primitive to a document tree.
func applyToTree(tree map[string]any, p Primitive) {
	switch p.Collection {
	case "settings":
		obj, _ := tree["settings"].(map[string]any)
		if obj == nil {
			obj = map[string]any{}
			tree["settings"] = obj
		}
		patchFields(obj, p)
	case "project":
		patchFields(tree, p)
	default:
		coll, _ := tree[p.Collection].(map[string]any)
		if coll == nil {
			coll = map[string]any{}
			tree[p.Collection] = coll
		}
		switch p.Type {
		case PrimitivePut:
			coll[p.ID] = cloneTree(p.Object)
		case PrimitiveRemove:
			delete(coll, p.ID)
		default:
			if obj, ok := coll[p.ID].(map[string]any); ok {
				patchFields(obj, p)
			}
		}
	}
}

func patchFields(obj map[string]any, p Primitive) {
	for k, v := range p.Set {
		obj[k] = cloneTree(v)
	}
	for _, k := range p.Unset {
		delete(obj, k)
	}
}

func (p Primitive) String() string {
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf("%s %s %s", p.Type, p.Collection, p.ID)
	}
	return string(b)
}
