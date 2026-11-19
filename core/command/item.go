package command

import (
	"fmt"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "item.move",
		Summary: "Moves an item in time, to another track of the same kind, or both.",
		Description: "Sets the start of the item, its track, or both; at least one is required. Keyframes, words, " +
			"transitions and fades are relative to the item and travel with it. With slide true the neighbours " +
			"that touch the item are trimmed so they keep touching it (a slide edit).",
		Example: `{"item_id": "itm_m1n2p3q4r5", "start": "5s"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeOutOfSource,
			CodeOverlap,
		},
		Scope: ScopeEdit,
	}, applyItemMove))

	Register(Define(Spec{
		Name:    "item.trim",
		Summary: "Moves the start edge or the end edge of an item to a timeline position.",
		Description: "Moves an edge to an absolute position and keeps the source range, the keyframes and the caption words " +
			"consistent. The end edge is exclusive: it is the first frame after the item. With ripple track the later " +
			"items of the track follow the change in length (a start trim then keeps the start of the item). With roll " +
			"true the touching edge of the neighbouring item moves to the same frame (a roll edit).",
		Example: `{"item_id": "itm_m1n2p3q4r5", "edge": "start", "to": "97f"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeInvalidState, CodeOutOfSource, CodeOverlap,
		},
		Scope: ScopeEdit,
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
	}, applyItemSplit))

	Register(Define(Spec{
		Name:    "item.delete",
		Summary: "Removes items, and optionally closes the freed space.",
		Description: "Removes the items. With ripple track, every later item on each affected track moves earlier by the " +
			"length freed on that track; with ripple all, every later item on every track moves by the length freed, which " +
			"requires the deleted items to cover one contiguous range; ripple all also moves the timeline markers after " +
			"the range earlier by that length and removes those inside it (effective.markers_removed, markers_moved).",
		Example: `{"item_ids": ["itm_a1b2c3d4e5"], "ripple": "track"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeInvalidState, CodeOverlap},
		Scope:   ScopeEdit,
	}, applyItemDelete))
}

// acceptsType says whether a track kind holds an item type: visual types on visual
// tracks, audio on audio tracks.
func acceptsType(t timeline.Track, typ timeline.ItemType) bool {
	return (typ == timeline.ItemAudio) == (t.Kind == timeline.TrackAudio)
}

type itemMoveArgs struct {
	ItemID  string `json:"item_id"            pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Start   Time   `json:"start,omitempty"                                                           desc:"The new absolute position of the first frame."`
	TrackID string `json:"track_id,omitempty" pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$" desc:"A track that accepts the item type."`
	Slide   bool   `json:"slide,omitempty"                                                           desc:"Slide edit: the neighbours that touch the item keep touching it, the one before ends and the one after starts where the item now does. Needs start; not with track_id."`
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
	if a.Slide {
		if err := c.slideNeighbours(&it, start, a.Start != "", a.TrackID != ""); err != nil {
			return err
		}
	}
	moved, err := c.movePeers(&it, start-it.Start)
	if err != nil {
		return err
	}
	if err := c.checkOverlap(a.ItemID, target, start, it.Duration); err != nil {
		return err
	}
	delta := start - it.Start
	it.Start, it.Track = start, target
	c.Doc.Items[a.ItemID] = it
	for _, id := range moved {
		o := c.Doc.Items[id]
		if err := c.checkOverlap(id, o.Track, o.Start, o.Duration); err != nil {
			return err.WithHint("Item %s is linked to %s and moves %d frames with it; move by less, or unlink it with item.unlink.",
				id, a.ItemID, delta).
				WithPath("args.start")
		}
	}
	if len(moved) > 0 {
		c.SetEffective("moved_with", moved)
	}
	req := string(a.Start)
	c.SetEffectiveTime("start", start, req)
	c.SetEffective("track_id", target)
	return nil
}

// slideNeighbours is the slide edit: the item that ends where the moved item starts gets its end
// moved to the new start, and the item that starts where it ends gets its start moved to the new end.
// The neighbours are written into the document so the overlap check of the move sees them.
func (c *Context) slideNeighbours(it *timeline.Item, start int, hasStart, hasTrack bool) *Error {
	if !hasStart || hasTrack {
		return Errorf(CodeValidation, "args: slide needs start and does not take track_id.").
			WithHint("Send {\"item_id\": \"%s\", \"start\": \"5s\", \"slide\": true}.", it.ID).WithPath("args.slide")
	}
	var slid []string
	for _, id := range c.trackItems(it.Track) {
		o := c.Doc.Items[id]
		var (
			edge string
			to   int
		)
		switch {
		case id == it.ID:
			continue
		case itemEnd(&o) == it.Start:
			edge, to = "end", start
		case o.Start == itemEnd(it):
			edge, to = "start", start+it.Duration
		default:
			continue
		}
		n, err := c.trimmed(
			&o,
			edge,
			to,
			fmt.Sprintf("Sliding %s to frame %d moves the %s of %s and", it.ID, start, edge, id),
		)
		if err != nil {
			return err.WithHint("Slide by less, or trim the neighbour %s with item.trim first.", id).
				WithPath("args.start")
		}
		c.Doc.Items[id] = n
		slid = append(slid, id)
	}
	// Every neighbour is trimmed first, then checked: the moved item is checked on its own after the
	// slide, and a neighbour that grows must not run into anyone else.
	for _, id := range slid {
		n := c.Doc.Items[id]
		for _, other := range c.conflicts(n.Track, n.Start, n.Duration, id) {
			if other == it.ID {
				continue
			}
			ot := c.Doc.Items[other]
			return Errorf(
				CodeOverlap,
				"Sliding %s to frame %d makes %s occupy frames %d-%d, where item %s already is (frames %d-%d).",
				it.ID, start, id, n.Start, itemEnd(&n)-1, other, ot.Start, itemEnd(&ot)-1,
			).
				WithDetail("conflicting_item", other).
				WithHint("Slide by less, or move item %s out of the way first.", other).
				WithPath("args.start")
		}
	}
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

// rejectCurve refuses an edit that maps frames of the timeline to frames of the source
// inside an item with a speed curve (SDD-002 C.25): the curve is defined over the whole
// item, so the mapping would no longer be the one the agent asked for.
func rejectCurve(it *timeline.Item, what string) *Error {
	if len(it.SpeedCurve) == 0 {
		return nil
	}
	return Errorf(CodeInvalidState, "Item %s has a speed curve, which %s cannot follow.", it.ID, what).
		WithHint("Remove the curve first with clip.set_speed and a constant speed.").WithPath("args.item_id")
}

// rejectRemap is rejectCurve for the commands that read the source in the forward direction
// (silences, speech, beats, ducking): they also refuse an item that plays backwards.
func rejectRemap(it *timeline.Item, what string) *Error {
	if it.Reverse {
		return Errorf(CodeInvalidState, "Item %s plays backwards, which %s cannot follow.", it.ID, what).
			WithHint("Play it forwards first with clip.set_speed and reverse false.").WithPath("args.item_id")
	}
	return rejectCurve(it, what)
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
	need := sourceIn + it.SourceFrames(duration)
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
	ItemID string `json:"item_id"          pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Edge   string `json:"edge"                                                                    enum:"start,end"`
	To     Time   `json:"to"                                                                                        desc:"Absolute position of the edge; the end is exclusive."`
	Ripple string `json:"ripple,omitempty"                                                        enum:"none,track" desc:"none (default) or track: the later items of the track follow the change in length; with the start edge the item keeps its start."`
	Roll   bool   `json:"roll,omitempty"                                                                            desc:"Roll edit: the touching edge of the neighbouring item moves to the same frame, so the cut moves and nothing else changes. Not with ripple track."`
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
	if a.Roll && a.Ripple == "track" {
		return Errorf(CodeValidation, "args: roll and ripple track cannot be combined.").
			WithHint("Use roll alone to move a cut, or ripple track alone to close or open a gap.").
			WithPath("args.roll")
	}
	n, err := c.trimmed(&it, a.Edge, to, fmt.Sprintf("Trimming the %s of %s to frame %d", a.Edge, a.ItemID, to))
	if err != nil {
		return err
	}
	if a.Roll {
		if err := c.rollNeighbour(&it, a.Edge, to); err != nil {
			return err
		}
	}
	if a.Ripple == "track" {
		if err := c.rippleTrim(&it, &n, a.Edge); err != nil {
			return err
		}
	}
	trimmedWith, err := c.trimPeers(&it, a.Edge, to)
	if err != nil {
		return err
	}
	if err := c.trimOverlap(&n, a.Edge); err != nil {
		return err
	}
	c.Doc.Items[a.ItemID] = n
	for _, id := range trimmedWith {
		o := c.Doc.Items[id]
		if err := c.trimOverlap(&o, a.Edge); err != nil {
			return err.WithPath("args.to")
		}
	}
	if len(trimmedWith) > 0 {
		c.SetEffective("trimmed_with", trimmedWith)
	}
	c.SetEffectiveTime("to", to, string(a.To))
	c.SetEffective("start", n.Start)
	c.SetEffective("duration", n.Duration)
	if n.Type == timeline.ItemClip || n.Type == timeline.ItemAudio {
		c.SetEffective("source_in", n.SourceIn)
	}
	return nil
}

// trimmed returns a copy of the item with one edge moved to `to`, fades and transitions checked.
func (c *Context) trimmed(it *timeline.Item, edge string, to int, what string) (timeline.Item, *Error) {
	n := timeline.CloneItem(*it)
	var err *Error
	if edge == "start" {
		err = c.trimStart(it, &n, to)
	} else {
		err = c.trimEnd(it, &n, to)
	}
	if err == nil {
		err = checkFit(&n, what)
	}
	return n, err
}

// rollNeighbour moves the edge of the item that touches the trimmed one to the same frame, writing
// the neighbour into the document so the overlap check of the trimmed item sees it.
func (c *Context) rollNeighbour(it *timeline.Item, edge string, to int) *Error {
	at, nbEdge := itemEnd(it), "start"
	if edge == "start" {
		at, nbEdge = it.Start, "end"
	}
	for _, id := range c.trackItems(it.Track) {
		o := c.Doc.Items[id]
		if id == it.ID || (nbEdge == "start" && o.Start != at) || (nbEdge == "end" && itemEnd(&o) != at) {
			continue
		}
		n, err := c.trimmed(
			&o,
			nbEdge,
			to,
			fmt.Sprintf("Rolling the cut to frame %d moves the %s of %s and", to, nbEdge, id),
		)
		if err != nil {
			return err.WithPath("args.to")
		}
		c.Doc.Items[id] = n
		c.SetEffective("rolled_item", id)
		return nil
	}
	return Errorf(CodeInvalidState, "No item of track %s touches the %s of %s at frame %d.", it.Track, edge, it.ID, at).
		WithHint("Roll needs a neighbour that starts exactly where this item ends (or ends where it starts); use item.trim without roll.").
		WithPath("args.roll")
}

// rippleTrim closes or opens the gap a trim leaves: the later items of the track shift by the
// change in length, and a start trim keeps the start of the item where it was.
func (c *Context) rippleTrim(old, n *timeline.Item, edge string) *Error {
	shift := n.Duration - old.Duration
	if edge == "start" {
		n.Start = old.Start
	}
	for _, id := range c.trackItems(old.Track) {
		if o := c.Doc.Items[id]; id != old.ID && o.Start >= itemEnd(old) {
			o.Start += shift
			c.Doc.Items[id] = o
		}
	}
	for _, id := range c.trackItems(old.Track) {
		if o := c.Doc.Items[id]; id != old.ID && itemEnd(&o) > timeline.MaxFrames {
			return Errorf(CodeInvalidTime, "Item %s would end at frame %d; the limit is %d.", id, itemEnd(&o),
				timeline.MaxFrames).
				WithHint("Trim less, or use ripple none.").WithPath("args.ripple")
		}
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
	if _, reads := c.sourceAsset(old); reads && !old.Freeze {
		if len(old.SpeedCurve) > 0 && d != 0 {
			return rejectCurve(old, "moving the start of the item")
		}
		sp := old.Speed
		switch {
		case old.Reverse:
			// The start of the output is the end of the source range: source_in stays and the range
			// loses or gains frames at its end (SDD-004 section 3.12).
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
	if old.Reverse && d < 0 {
		if err := c.checkSource(n, n.Duration, n.SourceIn); err != nil {
			return err.WithPath("args.to")
		}
	}
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
	if _, reads := c.sourceAsset(old); reads && old.Reverse && n.Duration != old.Duration {
		// The end of the output is the start of the source range: the range keeps its end, so
		// source_in moves by the change in the number of source frames read.
		n.SourceIn = old.SourceIn + old.SourceSpan() - n.SourceSpan()
		if n.SourceIn < 0 {
			return Errorf(
				CodeOutOfSource,
				"Moving the end of %s to frame %d needs source frame %d before the start of asset %s.",
				old.ID, to, n.SourceIn, old.Asset,
			).
				WithHint("Use a shorter end, or play the clip forwards with clip.set_speed and reverse false.").
				WithPath("args.to")
		}
	}
	switch {
	case n.Duration > old.Duration && !old.Reverse:
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
	ItemID string   `json:"item_id"           pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	At     Time     `json:"at"                                                                       desc:"Absolute position strictly inside the item."`
	NewID  string   `json:"new_id,omitempty"  pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$" desc:"The id of the right part."`
	NewIDs []string `json:"new_ids,omitempty" pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$" desc:"The ids of the right parts of the linked items that the split cuts, in the order of the ids of those items."`
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
	left, right, err := c.splitParts(&it, at, newID)
	if err != nil {
		return err
	}
	c.Doc.Items[a.ItemID], c.Doc.Items[newID] = left, right
	if err := c.splitPeers(&it, at, newID, a.NewIDs); err != nil {
		return err
	}
	c.SetEffectiveTime("at", at, string(a.At))
	c.SetEffective("new_id", newID)
	c.SetEffective("left", map[string]any{"id": a.ItemID, "start": left.Start, "duration": left.Duration})
	c.SetEffective("right", map[string]any{"id": newID, "start": right.Start, "duration": right.Duration})
	return nil
}

// splitParts is S(X, Tc) of R17 for one item: the left and right parts at the absolute frame `at`
// (strictly inside the item), with the caption words divided.
func (c *Context) splitParts(it *timeline.Item, at int, newID string) (left, right timeline.Item, err *Error) {
	tc := at - it.Start
	_, reads := c.sourceAsset(it)
	if reads {
		if err := rejectCurve(it, "a split"); err != nil {
			return left, right, err
		}
	}
	left, right = splitCore(it, newID, tc, reads)
	if it.Type != timeline.ItemCaption {
		return left, right, nil
	}
	var lb, rb []int
	left.Words, right.Words, lb, rb = splitWords(it.Words, it.Breaks, tc)
	left.Breaks, right.Breaks = lb, rb
	for _, side := range []struct {
		name  string
		words []timeline.Word
	}{{"left", left.Words}, {"right", right.Words}} {
		if len(side.words) == 0 {
			return left, right, Errorf(
				CodeInvalidState,
				"Splitting caption %s at frame %d leaves the %s part with no word.",
				it.ID,
				at,
				side.name,
			).
				WithHint("Split at a word boundary with caption.split.").
				WithPath("args.at")
		}
	}
	return left, right, nil
}

type itemDeleteArgs struct {
	ItemIDs []string `json:"item_ids"         min:"1" max:"200" pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Ripple  string   `json:"ripple,omitempty"                                                                          enum:"none,track,all" desc:"none (default), track or all."`
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
		if length, ok := freed.(int); ok {
			c.rippleMarkers(rippleStart(removed), rippleStart(removed)+length)
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

// rippleStart is the first frame of the deleted items.
func rippleStart(removed []timeline.Item) int {
	start := removed[0].Start
	for i := range removed {
		start = min(start, removed[i].Start)
	}
	return start
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
	return c.overlapAfterShiftHint(shifts, "Use ripple \"track\", or delete the straddling item too.")
}

// overlapAfterShiftHint is overlapAfterShift with the hint of the calling command.
func (c *Context) overlapAfterShiftHint(shifts map[string]int, hint string) *Error {
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
					WithHint("%s", hint).
					WithDetail("conflicting_item", prev.ID).
					WithDetail("next_free", itemEnd(&prev))
			}
		}
	}
	return nil
}
