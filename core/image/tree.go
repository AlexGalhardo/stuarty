package image

import (
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// loc is where a layer sits: the slice of its siblings, its index in it and its group (nil at the top level).
type loc struct {
	siblings *[]Layer
	index    int
	parent   *Layer
}

func (l loc) layer() *Layer { return &(*l.siblings)[l.index] }

// locate finds a layer by id.
func locate(d *Document, id string) (loc, bool) {
	return locateIn(&d.Layers, nil, id)
}

func locateIn(ls *[]Layer, parent *Layer, id string) (loc, bool) {
	for i := range *ls {
		l := &(*ls)[i]
		if l.ID == id {
			return loc{siblings: ls, index: i, parent: parent}, true
		}

		if found, ok := locateIn(&l.Children, l, id); ok {
			return found, true
		}
	}

	return loc{}, false
}

// Find returns the layer with the id.
func Find(d *Document, id string) (*Layer, bool) {
	at, ok := locate(d, id)
	if !ok {
		return nil, false
	}

	return at.layer(), true
}

// ancestors lists the groups above a layer, nearest first.
func ancestors(d *Document, id string) []*Layer {
	var chain []*Layer

	var visit func(ls []Layer, trail []*Layer) bool

	visit = func(ls []Layer, trail []*Layer) bool {
		for i := range ls {
			l := &ls[i]
			if l.ID == id {
				chain = slices.Clone(trail)

				return true
			}

			if visit(l.Children, append(trail, l)) {
				return true
			}
		}

		return false
	}

	visit(d.Layers, nil)
	slices.Reverse(chain)

	return chain
}

// effectivelyLocked reports whether the layer or one of its groups is locked.
func effectivelyLocked(d *Document, id string) bool {
	l, ok := Find(d, id)
	if !ok {
		return false
	}

	if l.Locked {
		return true
	}

	for _, g := range ancestors(d, id) {
		if g.Locked {
			return true
		}
	}

	return false
}

// height is the number of levels of a subtree: 1 for a layer that is not a group.
func height(l *Layer) int {
	h := 0
	for i := range l.Children {
		h = max(h, height(&l.Children[i]))
	}

	return h + 1
}

// reorder gives a key to every layer that has none and leaves the others alone, so that an insertion or a move
// changes one object and never its siblings. A layer without a key sorts after the key of the layer before it
// and before the next key that is set.
func reorder(ls []Layer) {
	prev := ""

	for i := range ls {
		if ls[i].Order != "" && ls[i].Order > prev && timeline.ValidOrder(ls[i].Order) {
			prev = ls[i].Order

			continue
		}

		next := ""

		for j := i + 1; j < len(ls); j++ {
			if k := ls[j].Order; k != "" && k > prev && timeline.ValidOrder(k) {
				next = k

				break
			}
		}

		key := timeline.OrderBetween(prev, next)
		if key == "" {
			key = timeline.OrderBetween(prev, "")
		}

		ls[i].Order = key
		prev = key
	}

	for i := range ls {
		reorder(ls[i].Children)
	}
}

// ReorderAll fixes the keys of the whole document.
func ReorderAll(d *Document) { reorder(d.Layers) }

// insert puts layer l into siblings at index (negative or too large means on top).
func insert(siblings *[]Layer, index int, l Layer) {
	if index < 0 || index > len(*siblings) {
		index = len(*siblings)
	}

	*siblings = slices.Insert(*siblings, index, l)
}

// removeAt takes the layer at the location out of its siblings and returns a copy of it.
func removeAt(at loc) Layer {
	out := cloneLayer(at.layer())
	*at.siblings = slices.Delete(*at.siblings, at.index, at.index+1)

	return out
}

// usedAssets lists the hashes the image layers use, and the hashes of the lookup tables the adjustments use.
func usedAssets(d *Document) (images, luts map[string]bool) {
	images, luts = map[string]bool{}, map[string]bool{}

	Walk(d, func(l, _ *Layer, _ int) {
		if l.Kind == KindImage && l.Asset != "" {
			images[l.Asset] = true
		}

		if l.Adjust != nil && l.Adjust.LUT != nil {
			luts[l.Adjust.LUT.Asset] = true
		}
	})

	return images, luts
}

// pruneAssets drops from the table the files no layer uses.
func pruneAssets(d *Document) {
	images, luts := usedAssets(d)
	for h, a := range d.Assets {
		if (a.isLUT() && !luts[h]) || (!a.isLUT() && !images[h]) {
			delete(d.Assets, h)
		}
	}

	if len(d.Assets) == 0 {
		d.Assets = nil
	}
}
