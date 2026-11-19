package command

import (
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// trackItems returns the ids of the items of a track sorted by start, then id.
func (c *Context) trackItems(track string) []string {
	var ids []string
	for id := range c.Doc.Items {
		if c.Doc.Items[id].Track == track {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b string) int {
		if d := c.Doc.Items[a].Start - c.Doc.Items[b].Start; d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	return ids
}

// nextFree is R8: the smallest frame f at or after from such that [f, f+length)
// overlaps no item of the track except ignore; ok is false when f+length would pass
// the end of the timeline.
func (c *Context) nextFree(track string, from, length int, ignore string) (int, bool) {
	f := from
	for moved := true; moved; {
		moved = false
		for _, id := range c.trackItems(track) {
			it := c.Doc.Items[id]
			if id != ignore && it.Start < f+length && itemEnd(&it) > f {
				f, moved = itemEnd(&it), true
			}
		}
	}
	return f, f+length <= timeline.MaxFrames
}

// maxDur is R9: the longest duration an item reading from sourceIn can have at the
// given speed when the asset has total frames; 0 when none fits.
func maxDur(total, sourceIn int, sp timeline.Rational) int {
	return max((total-sourceIn)*sp.Den/sp.Num, 0)
}
