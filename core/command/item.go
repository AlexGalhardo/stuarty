package command

import (
	"fmt"
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "item.move",
		Summary: "Moves an item in time, to another track of the same kind, or both.",
		Description: "Sets the start of the item, its track, or both; at least one is required. Keyframes, words, " +
			"transitions and fades are relative to the item and travel with it.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "start": "5s"}`,
		Errors:  []string{CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeOverlap},
		Scope:   ScopeEdit,
		Phase:   Prototype,
	}, applyItemMove))

	Register(Define(Spec{
		Name:    "item.trim",
		Summary: "Moves the start edge or the end edge of an item to a timeline position.",
		Description: "Moves an edge to an absolute position and keeps the source range, the keyframes and the caption words " +
			"consistent. The end edge is exclusive: it is the first frame after the item.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "edge": "start", "to": "97f"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeInvalidState, CodeOutOfSource, CodeOverlap,
		},
		Scope: ScopeEdit,
		Phase: Prototype,
	}, applyItemTrim))

	Register(Define(Spec{
		Name:    "item.split",
		Summary: "Splits an item into two at a timeline frame.",
		Description: "The original item keeps its id and becomes the left part; a new item is the right part. Transitions, " +
			"fades, keyframes and caption words are divided so that the motion is unchanged.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "at": "97f", "new_id": "itm_s6t7v8w9x0"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeInvalidState, CodeIDExists, CodeLimit,
		},
		Scope: ScopeEdit,
		Phase: Prototype,
	}, applyItemSplit))

	Register(Define(Spec{
		Name:    "item.delete",
		Summary: "Removes items, and optionally closes the freed space.",
		Description: "Removes the items. With ripple track, every later item on each affected track moves earlier by the " +
			"length freed on that track; with ripple all, every later item on every track moves by the length freed, which " +
			"requires the deleted items to cover one contiguous range.",
		Example: `{"item_ids": ["itm_a1b2c3d4e5"], "ripple": "track"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeInvalidState, CodeOverlap},
		Scope:   ScopeEdit,
		Phase:   Prototype,
	}, applyItemDelete))
}

// acceptsType says whether a track kind holds an item type: visual types on visual
// tracks, audio on audio tracks.
func acceptsType(t timeline.Track, typ timeline.ItemType) bool {
	return (typ == timeline.ItemAudio) == (t.Kind == timeline.TrackAudio)
}

type itemMoveArgs struct {
	ItemID  string `json:"item_id"`
	Start   Time   `json:"start,omitempty"    desc:"The new absolute position of the first frame."`
	TrackID string `json:"track_id,omitempty" desc:"A track that accepts the item type."`
}

func applyItemMove(c *Context, a *itemMoveArgs) error {
	if err := atLeastOne([]string{"start", "track_id"}, a.Start != "", a.TrackID != ""); err != nil {
		return err
	}
	for _, f := range []struct{ path, prefix, id string }{
		{"args.item_id", timeline.PrefixItem, a.ItemID}, {"args.track_id", timeline.PrefixTrack, a.TrackID},
	} {
		if err := idForm(f.path, f.prefix, f.id); err != nil {
			return err
		}
	}
	start := -1
	if a.Start != "" {
		var err *Error
		if start, err = c.ParseTime("args.start", string(a.Start)); err != nil {
			return err
		}
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	target := it.Track
	if a.TrackID != "" {
		target = a.TrackID
	}
	tr, err := c.track(target, "args.track_id")
	if err != nil {
		return err
	}
	if !acceptsType(tr, it.Type) {
		return Errorf(CodeTypeMismatch, "Item %s is a %s; track %s is a %s track.", a.ItemID, it.Type, target, tr.Kind).
			WithHint("Move it to a %s track.", map[bool]string{true: "visual", false: "audio"}[it.Type != timeline.ItemAudio]).
			WithPath("args.track_id")
	}
	if start < 0 {
		start = it.Start
	}
	if err := c.checkEnd("args.start", start, it.Duration); err != nil {
		return err
	}
	if err := c.checkOverlap(a.ItemID, target, start, it.Duration); err != nil {
		return err
	}
	it.Start, it.Track = start, target
	c.Doc.Items[a.ItemID] = it
	req := string(a.Start)
	c.SetEffectiveTime("start", start, req)
	c.SetEffective("track_id", target)
	return nil
}

// sourceAsset returns the asset an item reads from when its source range matters: a
// clip or audio item of a real video or audio asset.
func (c *Context) sourceAsset(it *timeline.Item) (timeline.Asset, bool) {
	if it.Type != timeline.ItemClip && it.Type != timeline.ItemAudio {
		return timeline.Asset{}, false
	}
	a, ok := c.Doc.Assets[it.Asset]
	return a, ok && a.Kind != timeline.AssetImage
}

// ceilDiv returns ceil(n/d) for positive d.
func ceilDiv(n, d int) int { return (n + d - 1) / d }

// checkSource is R9: the source range of an item of the given duration and source_in
// must stay inside its asset (E_OUT_OF_SOURCE).
func (c *Context) checkSource(it *timeline.Item, duration, sourceIn int) *Error {
	a, ok := c.sourceAsset(it)
	if !ok {
		return nil
	}
	total, ok := timeline.AssetDuration(a, c.Doc.Settings.FPS)
	if !ok {
		return nil
	}
	sp := it.Speed
	need := sourceIn + ceilDiv(duration*sp.Num, sp.Den)
	if need <= total {
		return nil
	}
	maxDur := maxDur(total, sourceIn, sp)
	e := Errorf(
		CodeOutOfSource,
		"The range of %d frames from source frame %d would read to frame %d; asset %s has %d frames.",
		duration,
		sourceIn,
		need,
		it.Asset,
		total,
	).
		WithDetail("available", map[string]any{"asset_duration": total, "source_in": sourceIn, "max_duration": maxDur})
	if maxDur >= 1 {
		return e.WithHint(
			"Use a duration of at most \"%df\" (%s), the most this asset allows from source_in %d.",
			maxDur,
			c.seconds(maxDur),
			sourceIn,
		)
	}
	return e.WithHint("Use a source_in before the end of the asset (%d frames).", total)
}

type itemTrimArgs struct {
	ItemID string `json:"item_id"`
	Edge   string `json:"edge"    enum:"start,end"`
	To     Time   `json:"to"                       desc:"Absolute position of the edge; the end is exclusive."`
}

func applyItemTrim(c *Context, a *itemTrimArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	to, err := c.ParseTime("args.to", string(a.To))
	if err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	n := timeline.CloneItem(it)
	if a.Edge == "start" {
		err = c.trimStart(&it, &n, to)
	} else {
		err = c.trimEnd(&it, &n, to)
	}
	if err != nil {
		return err
	}
	if err := checkFit(&n, fmt.Sprintf("Trimming the %s of %s to frame %d", a.Edge, a.ItemID, to)); err != nil {
		return err
	}
	if err := c.trimOverlap(&n, a.Edge); err != nil {
		return err
	}
	c.Doc.Items[a.ItemID] = n
	c.SetEffectiveTime("to", to, string(a.To))
	c.SetEffective("start", n.Start)
	c.SetEffective("duration", n.Duration)
	if n.Type == timeline.ItemClip || n.Type == timeline.ItemAudio {
		c.SetEffective("source_in", n.SourceIn)
	}
	return nil
}

func (c *Context) trimStart(old, n *timeline.Item, to int) *Error {
	end := itemEnd(old)
	if to >= end {
		return Errorf(
			CodeInvalidState,
			"Trimming the start of %s to frame %d leaves no frame: the item ends at %d.",
			old.ID,
			to,
			end,
		).
			WithHint("Use a start before \"%df\", or delete the item.", end).
			WithPath("args.to")
	}
	d := to - old.Start
	if _, reads := c.sourceAsset(old); reads {
		sp := old.Speed
		switch {
		case d > 0:
			n.SourceIn = old.SourceIn + d*sp.Num/sp.Den
		case d < 0:
			n.SourceIn = old.SourceIn - ceilDiv(-d*sp.Num, sp.Den)
		}
		if n.SourceIn < 0 {
			earliest := old.Start - old.SourceIn*sp.Den/sp.Num
			return Errorf(
				CodeOutOfSource,
				"Moving the start of %s from frame %d to %d needs source frame %d at speed %d/%d (source_in %d); the asset begins at 0.",
				old.ID,
				old.Start,
				to,
				n.SourceIn,
				sp.Num,
				sp.Den,
				old.SourceIn,
			).
				WithHint("Use to \"%df\" (%s), the earliest start this clip allows at speed %d/%d.", earliest, c.seconds(earliest), sp.Num, sp.Den).
				WithDetail("available", map[string]any{"source_in": old.SourceIn, "earliest_to": earliest}).
				WithPath("args.to")
		}
	}
	n.Start, n.Duration = to, old.Duration-d
	if d != 0 {
		kShift(n, d)
		trimWordsFront(n, d)
	}
	if old.Type == timeline.ItemCaption && len(n.Words) == 0 {
		return Errorf(CodeInvalidState, "Trimming the start of caption %s to frame %d leaves no word.", old.ID, to).
			WithHint("Use a start before the last word, or delete the caption.").WithPath("args.to")
	}
	return nil
}

func (c *Context) trimEnd(old, n *timeline.Item, to int) *Error {
	if to > timeline.MaxFrames {
		return Errorf(
			CodeInvalidTime,
			"args.to \"%df\" is past the timeline limit of %d frames.",
			to,
			timeline.MaxFrames,
		).
			WithHint("Use a value up to \"%df\" (%s).", timeline.MaxFrames, c.seconds(timeline.MaxFrames)).
			WithPath("args.to")
	}
	if to <= old.Start {
		return Errorf(
			CodeInvalidState,
			"Trimming the end of %s to frame %d leaves no frame: the item starts at %d.",
			old.ID,
			to,
			old.Start,
		).
			WithHint("Use an end after \"%df\", or delete the item.", old.Start).
			WithPath("args.to")
	}
	n.Duration = to - old.Start
	switch {
	case n.Duration > old.Duration:
		if err := c.checkSource(old, n.Duration, old.SourceIn); err != nil {
			return err.WithPath("args.to")
		}
	case n.Duration < old.Duration:
		kCut(n, n.Duration)
		trimWordsBack(n, n.Duration)
		if old.Type == timeline.ItemCaption && len(n.Words) == 0 {
			return Errorf(CodeInvalidState, "Trimming the end of caption %s to frame %d leaves no word.", old.ID, to).
				WithHint("Use an end after the first word, or delete the caption.").WithPath("args.to")
		}
	}
	return nil
}

// trimOverlap is the overlap check of item.trim: next_free is the nearest value of
// `to` that avoids the overlap (R8).
func (c *Context) trimOverlap(n *timeline.Item, edge string) *Error {
	conflict := c.conflicts(n.Track, n.Start, n.Duration, n.ID)
	if len(conflict) == 0 {
		return nil
	}
	other := c.Doc.Items[conflict[0]]
	nearest := other.Start
	if edge == "start" {
		nearest = 0
		for _, id := range conflict {
			o := c.Doc.Items[id]
			nearest = max(nearest, itemEnd(&o))
		}
	}
	return Errorf(
		CodeOverlap,
		"Item %s would occupy frames %d-%d on track %s, where item %s already is (frames %d-%d).",
		n.ID,
		n.Start,
		itemEnd(n)-1,
		n.Track,
		conflict[0],
		other.Start,
		itemEnd(&other)-1,
	).
		WithHint("Use to \"%df\" (%s), the nearest %s that avoids the overlap.", nearest, c.seconds(nearest), edge).
		WithDetail("conflicting_item", conflict[0]).
		WithDetail("next_free", nearest).
		WithPath("args.to")
}

type itemSplitArgs struct {
	ItemID string `json:"item_id"`
	At     Time   `json:"at"               desc:"Absolute position strictly inside the item."`
	NewID  string `json:"new_id,omitempty" desc:"The id of the right part."`
}

func applyItemSplit(c *Context, a *itemSplitArgs) error {
	for _, f := range []struct{ path, id string }{{"args.item_id", a.ItemID}, {"args.new_id", a.NewID}} {
		if err := idForm(f.path, timeline.PrefixItem, f.id); err != nil {
			return err
		}
	}
	at, err := c.ParseTime("args.at", string(a.At))
	if err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if at <= it.Start || at >= itemEnd(&it) {
		return Errorf(
			CodeInvalidTime,
			"args.at \"%s\" is not inside item %s (frames %d-%d); it must be between %d and %d inclusive.",
			a.At,
			a.ItemID,
			it.Start,
			itemEnd(&it)-1,
			it.Start+1,
			itemEnd(&it)-1,
		).
			WithHint("Use at \"%df\" or later, up to \"%df\".", it.Start+1, itemEnd(&it)-1).
			WithPath("args.at")
	}
	newID, err := c.newID("items", timeline.PrefixItem, a.NewID)
	if err != nil {
		return err
	}
	if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
		return err
	}
	tc := at - it.Start
	_, reads := c.sourceAsset(&it)
	left, right := splitCore(&it, newID, tc, reads)
	if it.Type == timeline.ItemCaption {
		var lb, rb []int
		left.Words, right.Words, lb, rb = splitWords(it.Words, it.Breaks, tc)
		left.Breaks, right.Breaks = lb, rb
		for _, side := range []struct {
			name  string
			words []timeline.Word
		}{{"left", left.Words}, {"right", right.Words}} {
			if len(side.words) == 0 {
				return Errorf(
					CodeInvalidState,
					"Splitting caption %s at frame %d leaves the %s part with no word.",
					a.ItemID,
					at,
					side.name,
				).
					WithHint("Split at a word boundary with caption.split.").
					WithPath("args.at")
			}
		}
	}
	c.Doc.Items[a.ItemID], c.Doc.Items[newID] = left, right
	c.SetEffectiveTime("at", at, string(a.At))
	c.SetEffective("new_id", newID)
	c.SetEffective("left", map[string]any{"id": a.ItemID, "start": left.Start, "duration": left.Duration})
	c.SetEffective("right", map[string]any{"id": newID, "start": right.Start, "duration": right.Duration})
	return nil
}

type itemDeleteArgs struct {
	ItemIDs []string `json:"item_ids"         min:"1" max:"200"`
	Ripple  string   `json:"ripple,omitempty"                   enum:"none,track,all" desc:"none (default), track or all."`
}

func applyItemDelete(c *Context, a *itemDeleteArgs) error {
	seen := map[string]bool{}
	for i, id := range a.ItemIDs {
		p := fmt.Sprintf("args.item_ids[%d]", i)
		if err := idForm(p, timeline.PrefixItem, id); err != nil {
			return err
		}
		if seen[id] {
			return Errorf(
				CodeValidation,
				"%s %q is listed twice.",
				p,
				id,
			).WithHint("List each item id once.").
				WithPath(p)
		}
		seen[id] = true
	}
	var missing []string
	for _, id := range a.ItemIDs {
		if _, ok := c.Doc.Items[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return notFound("item", missing[0], "args.item_ids").WithDetail("missing", missing)
	}
	deleted := slices.Clone(a.ItemIDs)
	slices.Sort(deleted)
	removed := make([]timeline.Item, 0, len(deleted))
	for _, id := range deleted {
		removed = append(removed, c.Doc.Items[id])
	}

	var shifts map[string]int
	var freed any
	switch a.Ripple {
	case "track":
		shifts, freed = c.rippleTrack(removed)
	case "all":
		var err *Error
		if shifts, freed, err = c.rippleAll(removed, seen); err != nil {
			return err
		}
	}
	for _, id := range deleted {
		delete(c.Doc.Items, id)
	}
	shifted := make([]map[string]any, 0, len(shifts))
	for _, id := range sortedKeysInt(shifts) {
		it := c.Doc.Items[id]
		shifted = append(shifted, map[string]any{"id": id, "from": it.Start, "to": it.Start - shifts[id]})
		it.Start -= shifts[id]
		c.Doc.Items[id] = it
	}
	if a.Ripple == "all" {
		if err := c.overlapAfterShift(shifts); err != nil {
			return err
		}
	}
	c.SetEffective("deleted", deleted)
	c.SetEffective("shifted", shifted)
	if freed != nil {
		c.SetEffective("freed", freed)
	}
	return nil
}

func sortedKeysInt(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// rippleTrack: every item that stays on a track that lost items moves earlier by the
// durations of the deleted items of that track that end at or before its start.
func (c *Context) rippleTrack(removed []timeline.Item) (map[string]int, any) {
	gone := map[string]bool{}
	freed := map[string]int{}
	for i := range removed {
		gone[removed[i].ID] = true
		freed[removed[i].Track] += removed[i].Duration
	}
	shifts := map[string]int{}
	for _, id := range c.sortedItemIDs() {
		it := c.Doc.Items[id]
		if gone[id] || freed[it.Track] == 0 {
			continue
		}
		by := 0
		for i := range removed {
			if removed[i].Track == it.Track && itemEnd(&removed[i]) <= it.Start {
				by += removed[i].Duration
			}
		}
		if by > 0 {
			shifts[id] = by
		}
	}
	return shifts, freed
}

// rippleAll: the deleted items must cover one contiguous range [a, b); every item
// that stays and starts at or after b moves earlier by b - a.
func (c *Context) rippleAll(removed []timeline.Item, gone map[string]bool) (map[string]int, any, *Error) {
	byStart := slices.Clone(removed)
	slices.SortStableFunc(byStart, func(x, y timeline.Item) int { return x.Start - y.Start })
	a, cur := byStart[0].Start, byStart[0].Start
	for i := range byStart {
		if byStart[i].Start > cur {
			return nil, nil, Errorf(
				CodeInvalidState,
				"Deleted items cover frames %d-%d and %d-%d; frames %d-%d are not covered.",
				a,
				cur-1,
				byStart[i].Start,
				itemEnd(&byStart[i])-1,
				cur,
				byStart[i].Start-1,
			).
				WithHint("Add the item covering frames %d-%d to item_ids, or use ripple \"track\".", cur, byStart[i].Start-1).
				WithDetail("gap", map[string]any{"start": cur, "end": byStart[i].Start})
		}
		cur = max(cur, itemEnd(&byStart[i]))
	}
	shifts := map[string]int{}
	for _, id := range c.sortedItemIDs() {
		if !gone[id] && c.Doc.Items[id].Start >= cur {
			shifts[id] = cur - a
		}
	}
	return shifts, cur - a, nil
}

// overlapAfterShift reports the first overlap that a ripple all left on any track.
func (c *Context) overlapAfterShift(shifts map[string]int) *Error {
	tracks := map[string]bool{}
	for id := range shifts {
		tracks[c.Doc.Items[id].Track] = true
	}
	order := make([]string, 0, len(tracks))
	for t := range tracks {
		order = append(order, t)
	}
	slices.Sort(order)
	for _, t := range order {
		ids := c.trackItems(t)
		for i := 1; i < len(ids); i++ {
			prev, cur := c.Doc.Items[ids[i-1]], c.Doc.Items[ids[i]]
			if cur.Start < itemEnd(&prev) {
				return Errorf(
					CodeOverlap,
					"Item %s would occupy frames %d-%d on track %s, where item %s already is (frames %d-%d).",
					cur.ID,
					cur.Start,
					itemEnd(&cur)-1,
					t,
					prev.ID,
					prev.Start,
					itemEnd(&prev)-1,
				).
					WithHint("Use ripple \"track\", or delete the straddling item too.").
					WithDetail("conflicting_item", prev.ID).
					WithDetail("next_free", itemEnd(&prev))
			}
		}
	}
	return nil
}
