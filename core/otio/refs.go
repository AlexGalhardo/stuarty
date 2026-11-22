package otio

import (
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The identity of items in a file (VV-025). An id of the document is not an id of another project, so a
// file never carries one: Export writes labels (i1, i2, l1, ...) in the vukem metadata, numbered in the
// order the items are written, and Import reads them only to tell which items of the same file belong
// together. Every id of an imported item is a new one, so a file can be imported twice into one project, a
// label can never name an item that is not in the file, and the same document always writes the same file.

const maxLabelRunes = 64

// maxLinkGroup is the largest link group item.link makes (SDD-002).
const maxLinkGroup = 20

// refs is what Export knows about the identity of the items it writes.
type refs struct {
	written map[string]bool   // the ids of the items that are written (as a clip or as a gap with metadata)
	members map[string]int    // link group -> items of the document
	inFile  map[string]int    // link group -> items written
	itemTag map[string]string // item id -> its label, for an item that is linked or followed by a caption
	linkTag map[string]string // link group -> its label
}

// newRefs reads the identity of the items of the document; order lists the items in the order they are
// written, which is the order of the labels.
func newRefs(doc *timeline.Document, order []string) *refs {
	r := &refs{
		written: map[string]bool{}, members: map[string]int{}, inFile: map[string]int{},
		itemTag: map[string]string{}, linkTag: map[string]string{},
	}

	for id := range doc.Items {
		it := doc.Items[id]
		if it.Link != "" {
			r.members[it.Link]++
		}

		if writesItem(doc, &it) {
			r.written[id] = true

			if it.Link != "" {
				r.inFile[it.Link]++
			}
		}
	}

	targets := map[string]bool{}

	for id := range doc.Items {
		if it := doc.Items[id]; r.written[id] && it.Type == timeline.ItemCaption && r.written[it.LinkedItem] {
			targets[it.LinkedItem] = true
		}
	}

	for _, id := range order {
		it := doc.Items[id]
		if !r.written[id] {
			continue
		}

		linked := it.Link != "" && r.inFile[it.Link] >= 2

		if linked || targets[id] {
			r.itemTag[id] = fmt.Sprintf("i%d", len(r.itemTag)+1)
		}

		if _, has := r.linkTag[it.Link]; linked && !has {
			r.linkTag[it.Link] = fmt.Sprintf("l%d", len(r.linkTag)+1)
		}
	}

	return r
}

// writesItem is true when Export writes the item: a clip or an audio item whose asset is in the document, or
// a text, caption or shape item (a gap that carries it).
func writesItem(doc *timeline.Document, it *timeline.Item) bool {
	if unsupported(it) != "" {
		return gapMetadata(it) != nil
	}

	_, ok := doc.Assets[it.Asset]

	return ok
}

// attach adds the labels of the item to the vukem object of its metadata (made when there is none) and
// returns what a file cannot say about its links.
func (r *refs) attach(meta object, it *timeline.Item) []Lost {
	v, _ := meta[metaNamespace].(object)
	if v == nil {
		v = object{}
	}

	var lost []Lost

	add := func(feature string) { lost = append(lost, Lost{ID: it.ID, Name: it.Name, Feature: feature}) }

	if tag, ok := r.itemTag[it.ID]; ok {
		v["id"] = tag
	}

	if it.Link != "" {
		if tag, ok := r.linkTag[it.Link]; ok {
			v["link"] = tag
		}

		if r.inFile[it.Link] != r.members[it.Link] || r.inFile[it.Link] < 2 {
			add("link to an item that is not in the file")
		}
	}

	if it.Type == timeline.ItemCaption && it.LinkedItem != "" {
		if tag, ok := r.itemTag[it.LinkedItem]; ok {
			v["linked_item"] = tag
		} else {
			add("caption link to a clip that is not in the file")
		}
	}

	if len(v) > 0 {
		meta[metaNamespace] = v
	}

	return lost
}

// label reads a label of the vukem metadata: a short string with no control character. Anything else is
// not a label.
func label(raw any) (string, bool) {
	s, ok := raw.(string)
	if !ok || s == "" || utf8.RuneCountInString(s) > maxLabelRunes {
		return "", false
	}

	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return "", false
		}
	}

	return s, true
}

// itemRefs are the labels an item of a file carries.
type itemRefs struct {
	id, link, linked string
}

// readRefs reads the labels of the vukem metadata of an item. A label that is not valid is reported and
// ignored. Labels are file-local: the first item with a label owns it, and a second one with the same label
// loses it.
func (r *reader) readRefs(v map[string]any, name string) itemRefs {
	var out itemRefs

	for _, f := range []struct {
		key string
		to  *string
	}{{"id", &out.id}, {"link", &out.link}, {"linked_item", &out.linked}} {
		raw, present := v[f.key]
		if !present {
			continue
		}

		if l, ok := label(raw); ok {
			*f.to = l
		} else {
			r.lose("vukem property " + f.key + " of " + name + " is not a valid value")
		}
	}

	if out.id != "" {
		if r.ids[out.id] {
			r.lose("vukem property id of " + name + " is used by another item of the file")

			out.id = ""
		} else {
			r.ids[out.id] = true
		}
	}

	return out
}

// linkGroups returns the groups of item ids (new ids) that share a link label, in the order the labels
// first appear, and reports a group that cannot be made.
func (b *batch) linkGroups() [][]string {
	var order []string

	for l := range b.links {
		order = append(order, l)
	}

	slices.SortFunc(order, func(x, y string) int { return b.linkOrder[x] - b.linkOrder[y] })

	var out [][]string

	for _, l := range order {
		ids := b.links[l]

		switch {
		case len(ids) < 2:
			b.lost = append(b.lost, Lost{Feature: "link group with fewer than two items that could be imported"})
		case len(ids) > maxLinkGroup:
			b.lost = append(b.lost, Lost{Feature: "link group of more than 20 items"})
		default:
			out = append(out, ids)
		}
	}

	return out
}

// reserve gives the items that carry an id label their new ids before any item is added, so that a caption
// can follow an item that comes later in the file. An item that will not be added (a clip whose reference has
// no asset) gets none, and what points at it is reported when it is read.
func (b *batch) reserve(tracks []importTrack) {
	for i := range tracks {
		for j := range tracks[i].entries {
			e := &tracks[i].entries[j]
			if e.refs.id == "" {
				continue
			}

			if _, has := b.assets[e.ref]; e.clip && !has {
				continue
			}

			b.pre[e.refs.id] = b.newID(timeline.PrefixItem)
		}
	}
}

// itemID is the id of an item that is about to be added: the one reserved for its label, or a new one. It
// also puts the item in its link group.
func (b *batch) itemID(e *entry) string {
	id, ok := b.pre[e.refs.id]
	if e.refs.id == "" || !ok {
		id = b.newID(timeline.PrefixItem)
	}

	if l := e.refs.link; l != "" {
		if _, seen := b.linkOrder[l]; !seen {
			b.linkOrder[l] = len(b.linkOrder)
		}

		b.links[l] = append(b.links[l], id)
	}

	return id
}

// templateSpec is the template information of a file: the description and the named colours.
type templateSpec struct {
	description *string
	colors      map[string]any
}

const maxTemplateDescription = 500

// templateInfo reads the template of the vukem metadata of the timeline.
func (r *reader) templateInfo(root map[string]any) {
	v := vukemMeta(root)

	raw, present := v["template"]
	if !present {
		return
	}

	obj, ok := raw.(map[string]any)
	if !ok {
		r.lose("vukem property template is not a valid value")

		return
	}

	spec := &templateSpec{colors: map[string]any{}}

	if d, has := obj["description"]; has {
		s, isStr := d.(string)
		if isStr && utf8.RuneCountInString(s) <= maxTemplateDescription {
			spec.description = &s
		} else {
			r.lose("vukem property template description is not a valid value")
		}
	}

	if c, has := obj["colors"]; has {
		list, isMap := c.(map[string]any)
		if !isMap {
			r.lose("vukem property template colors is not a valid value")
		}

		for name, val := range list {
			s, isStr := val.(string)
			if !slotNameRe.MatchString(name) || !isStr || !hexColourRe.MatchString(s) {
				r.lose("template colour " + clipRunes(name, 40) + " is not a valid value")

				continue
			}

			spec.colors[name] = s
		}
	}

	if spec.description != nil || len(spec.colors) > 0 {
		r.im.template = spec
	}
}

// template pushes the template information first, so that the styles that name a colour as "$name" find it.
func (b *batch) template(t *templateSpec) {
	if t == nil {
		return
	}

	args := map[string]any{}
	if t.description != nil {
		args["description"] = *t.description
	}

	if len(t.colors) > 0 {
		args["colors"] = t.colors
	}

	b.push("template.set_info", args)
}
