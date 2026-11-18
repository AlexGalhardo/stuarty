package command

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// Helpers shared by the commands: lookups with the errors of SDD-002 R1, identifier
// rules (R6), lengths (R7), the end of the timeline and the free position (R8).

var kindWords = map[string][2]string{
	"item":  {"Item", "an item"},
	"track": {"Track", "a track"},
	"asset": {"Asset", "an asset"},
}

func notFound(kind, id, path string) *Error {
	w := kindWords[kind]
	e := Errorf(CodeNotFound, "%s %s does not exist.", w[0], id).
		WithHint("Use the id of %s listed by project_get.", w[1]).
		WithDetail("kind", kind).WithDetail("id", id)
	if path != "" {
		return e.WithPath(path)
	}
	return e
}

func (c *Context) track(id, path string) (timeline.Track, *Error) {
	t, ok := c.Doc.Tracks[id]
	if !ok {
		return t, notFound("track", id, path)
	}
	return t, nil
}

func (c *Context) item(id string) (timeline.Item, *Error) {
	it, ok := c.Doc.Items[id]
	if !ok {
		return it, notFound("item", id, "args.item_id")
	}
	return it, nil
}

func (c *Context) asset(id, path string) (timeline.Asset, *Error) {
	a, ok := c.Doc.Assets[id]
	if !ok {
		return a, notFound("asset", id, path)
	}
	return a, nil
}

// idForm checks that a supplied identifier has the form of its prefix (R6).
func idForm(path, prefix, id string) *Error {
	if id == "" || timeline.ValidID(prefix, id) {
		return nil
	}
	return Errorf(CodeValidation, "%s %q is not %s id: an id is \"%s_\" followed by 10 characters from 0-9 and a-z "+
		"without i, l, o and u.", path, id, map[string]string{
		timeline.PrefixItem: "an item", timeline.PrefixTrack: "a track", timeline.PrefixAsset: "an asset",
	}[prefix], prefix).WithHint("Use an id like \"%s_%s\" or omit it.", prefix, "s6t7v8w9x0").WithPath(path)
}

// newID returns the identifier for a new object of a collection: the supplied one or
// a generated one; an id that is already used is E_ID_EXISTS (R6).
func (c *Context) newID(collection, prefix, supplied string) (string, *Error) {
	id, err := c.NewID(prefix, supplied)
	if err != nil {
		return "", err
	}
	used := false
	word := ""
	switch collection {
	case "items":
		_, used = c.Doc.Items[id]
		word = "Item"
	case "tracks":
		_, used = c.Doc.Tracks[id]
		word = "Track"
	case "assets":
		_, used = c.Doc.Assets[id]
		word = "Asset"
	}
	if used {
		return "", Errorf(CodeIDExists, "%s id %s already exists.", word, id).
			WithHint("Omit id or use an unused id.").WithPath("args.id")
	}
	return id, nil
}

func (c *Context) limit(what string, have, limit int) *Error {
	if have < limit {
		return nil
	}
	return Errorf(CodeLimit, "The document already has %d %s; the limit is %d.", have, what, limit).
		WithHint("Delete unused %s first.", what)
}

// length parses a time that is a length: at least 1 frame, or 0 when allowZero (R7).
func (c *Context) length(path, s string, allowZero bool) (int, *Error) {
	f, err := c.ParseTime(path, s)
	if err != nil {
		return 0, err
	}
	if f < 1 && !allowZero {
		return 0, Errorf(CodeInvalidTime, "%s %q is 0 frames.", path, s).WithHint(`Use "2s" or "60f".`).WithPath(path)
	}
	return f, nil
}

// checkEnd refuses an item that would end after frame 108,000 (R7).
func (c *Context) checkEnd(path string, start, duration int) *Error {
	if start+duration <= timeline.MaxFrames {
		return nil
	}
	latest := max(timeline.MaxFrames-duration, 0)
	return Errorf(
		CodeInvalidTime,
		"The item would occupy frames %d-%d, past the limit %d.",
		start,
		start+duration-1,
		timeline.MaxFrames,
	).
		WithHint("Use a start of at most \"%df\" for this %d-frame item.", latest, duration).
		WithPath(path)
}

func itemEnd(it *timeline.Item) int { return it.Start + it.Duration }

// conflicts returns the items of the track that overlap [start, start+length),
// ignoring one, sorted by start.
func (c *Context) conflicts(track string, start, length int, ignore string) []string {
	var out []string
	for _, id := range c.trackItems(track) {
		it := c.Doc.Items[id]
		if id != ignore && it.Start < start+length && itemEnd(&it) > start {
			out = append(out, id)
		}
	}
	return out
}

// checkOverlap returns E_OVERLAP when the item would overlap another one of the
// track (R8).
func (c *Context) checkOverlap(itemID, track string, start, length int) *Error {
	return c.checkOverlapAt(itemID, track, start, length, false)
}

// checkOverlapAppend is checkOverlap for the commands whose start is optional: the
// hint also offers omitting start, which appends at the end of the track.
func (c *Context) checkOverlapAppend(itemID, track string, start, length int) *Error {
	return c.checkOverlapAt(itemID, track, start, length, true)
}

func (c *Context) checkOverlapAt(itemID, track string, start, length int, optionalStart bool) *Error {
	conflict := c.conflicts(track, start, length, itemID)
	if len(conflict) == 0 {
		return nil
	}
	other := c.Doc.Items[conflict[0]]
	e := Errorf(CodeOverlap, "Item %s would occupy frames %d-%d on track %s, where item %s already is (frames %d-%d).",
		itemID, start, start+length-1, track, conflict[0], other.Start, itemEnd(&other)-1).
		WithDetail("conflicting_item", conflict[0])
	if free, ok := c.nextFree(track, start, length, itemID); ok {
		if optionalStart {
			return e.WithDetail("next_free", free).
				WithHint("Use start \"%df\" (%s), the next free position, omit start to append at the end of the track, or move to another track.", free, c.seconds(free))
		}
		return e.WithDetail("next_free", free).
			WithHint("Use start \"%df\" (%s), the next free position on that track, or move to another track.", free, c.seconds(free))
	}
	return e.WithDetail("next_free", nil).WithHint("No free position remains on that track; use another track.")
}

// seconds formats a frame as seconds for a message.
func (c *Context) seconds(frame int) string {
	return c.FPS().FormatSeconds(int64(frame))
}

// round3 rounds a space value to 3 decimals (SDD-002 §A.10).
func round3(v float64) float64 {
	// Above this a float64 has no decimals left, and v*1000 could overflow to +Inf.
	if math.Abs(v) >= 1e12 {
		return v
	}
	return math.Round(v*1000) / 1000
}

// chars counts Unicode code points.
func chars(s string) int { return utf8.RuneCountInString(s) }

// sortedItemIDs returns every item id, sorted.
func (c *Context) sortedItemIDs() []string {
	ids := make([]string, 0, len(c.Doc.Items))
	for id := range c.Doc.Items {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func requireOne(names []string, given ...bool) *Error {
	n := 0
	for _, g := range given {
		if g {
			n++
		}
	}
	if n == 1 {
		return nil
	}
	if n == 0 {
		return Errorf(CodeValidation, "args: give exactly one of %s.", strings.Join(names, ", ")).
			WithHint("Send only one of %s.", strings.Join(names, ", ")).WithPath("args")
	}
	return Errorf(CodeValidation, "args: give only one of %s, not several.", strings.Join(names, ", ")).
		WithHint("Send only one of %s.", strings.Join(names, ", ")).WithPath("args")
}

func atLeastOne(names []string, given ...bool) *Error {
	if slices.Contains(given, true) {
		return nil
	}
	return Errorf(CodeValidation, "args: give at least one of %s.", strings.Join(names, ", ")).
		WithHint("Send for example %s.", fmt.Sprintf("{%q: ...}", names[0])).WithPath("args")
}
