package command

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "item.duplicate",
		Summary: "Copies an item to another position, optionally on another track.",
		Description: "Adds a copy of the item at start, on the same track or another track of the same kind. Keyframes, " +
			"words and the name are copied; the slot mark is not. Without start the copy goes to the first free " +
			"position at or after the end of the source on the target track. With start it fails when the copy " +
			"would overlap an item of the target track.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "start": "3s", "new_id": "itm_s6t7v8w9x0"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeIDExists, CodeLimit, CodeOverlap,
		},
		Scope: ScopeEdit,
		Phase: Alpha,
	}, applyItemDuplicate))

	Register(Define(Spec{
		Name:    "item.crossfade",
		Summary: "Sets a matching fade-out and fade-in on two overlapping visual items of different tracks.",
		Description: "The item that leaves gets a fade transition out and the item that arrives a fade transition in, " +
			"both of the given length, plus the matching audio fades for clips. The two items must overlap by at " +
			"least that length, and the arriving item must start after the leaving one. For two neighbours that " +
			"touch on the same track give track_id, another visual track: the arriving item moves there and starts " +
			"duration earlier, using the source it has before the cut, so they overlap by duration.",
		Example: `{"from_item_id": "itm_m1n2p3q4r5", "to_item_id": "itm_s6t7v8w9x0", "duration": "1s"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeOutOfSource,
			CodeOverlap,
		},
		Scope: ScopeEdit,
		Phase: Alpha,
	}, applyItemCrossfade))

	Register(Define(Spec{
		Name:    "timeline.close_gap",
		Summary: "Closes the empty range that contains a given frame on a track.",
		Description: "Moves the items after the gap earlier by the length of the gap: those of the track (ripple " +
			"track, the default) or those of every track (ripple all); ripple all also moves the timeline markers after the " +
			"gap and removes those inside it.",
		Example: `{"track_id": "trk_f6g7h8j9k0", "at": "70f"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeInvalidState, CodeOverlap,
		},
		Scope: ScopeEdit,
		Phase: Alpha,
	}, applyCloseGap))
}

type itemDuplicateArgs struct {
	ItemID  string `json:"item_id"            desc:"The item to copy."                                                             pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Start   Time   `json:"start,omitempty"    desc:"Absolute start of the copy; default the first free position after the source."`
	TrackID string `json:"track_id,omitempty" desc:"The track of the copy; default the source's track."                            pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	NewID   string `json:"new_id,omitempty"   desc:"The id of the copy."                                                           pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyItemDuplicate(c *Context, a *itemDuplicateArgs) error {
	for _, f := range []struct{ path, prefix, id string }{
		{"args.item_id", timeline.PrefixItem, a.ItemID},
		{"args.track_id", timeline.PrefixTrack, a.TrackID},
		{"args.new_id", timeline.PrefixItem, a.NewID},
	} {
		if err := idForm(f.path, f.prefix, f.id); err != nil {
			return err
		}
	}
	src, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	trackID := a.TrackID
	if trackID == "" {
		trackID = src.Track
	}
	track, err := c.track(trackID, "args.track_id")
	if err != nil {
		return err
	}
	if !acceptsType(track, src.Type) {
		want, have := "a visual", "an audio"
		if src.Type == timeline.ItemAudio {
			want, have = "an audio", "a visual"
		}
		return Errorf(
			CodeTypeMismatch,
			"Item %s is a %s item; track %s is %s track.",
			a.ItemID,
			src.Type,
			trackID,
			have,
		).
			WithHint("Use %s track.", want).
			WithPath("args.track_id")
	}
	var start int
	if a.Start == "" {
		start, _ = c.nextFree(trackID, itemEnd(&src), src.Duration, "")
	} else if start, err = c.ParseTime("args.start", string(a.Start)); err != nil {
		return err
	}
	id, err := c.newID("items", timeline.PrefixItem, a.NewID)
	if err != nil {
		return err
	}
	if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
		return err
	}
	if src.Type == timeline.ItemCaption {
		if err := c.checkWordLimits("", len(src.Words)); err != nil {
			return err
		}
	}
	if err := c.checkEnd("args.start", start, src.Duration); err != nil {
		return err
	}
	if err := c.checkOverlap(id, trackID, start, src.Duration); err != nil {
		return err
	}
	cp := timeline.CloneItem(src)
	cp.ID, cp.Start, cp.Track, cp.Slot, cp.Link = id, start, trackID, nil, ""
	c.Doc.Items[id] = cp
	c.SetEffective("new_id", id)
	requested := string(a.Start)
	if requested == "" {
		requested = strconv.Itoa(start) + "f"
	}
	c.SetEffectiveTime("start", start, requested)
	c.SetEffective("track_id", trackID)
	return nil
}

type itemCrossfadeArgs struct {
	From     string `json:"from_item_id"       desc:"The item that leaves."                                                                             pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	To       string `json:"to_item_id"         desc:"The item that arrives."                                                                            pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Duration Time   `json:"duration"           desc:"Length of the crossfade, at least 1 frame."`
	TrackID  string `json:"track_id,omitempty" desc:"Only for two neighbours that touch on one track: another visual track the arriving item moves to." pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyItemCrossfade(c *Context, a *itemCrossfadeArgs) error {
	for _, f := range []struct{ path, id string }{{"args.from_item_id", a.From}, {"args.to_item_id", a.To}} {
		if err := idForm(f.path, timeline.PrefixItem, f.id); err != nil {
			return err
		}
	}
	length, err := c.length("args.duration", string(a.Duration), false)
	if err != nil {
		return err
	}
	from, ok := c.Doc.Items[a.From]
	if !ok {
		return notFound("item", a.From, "args.from_item_id")
	}
	to, ok := c.Doc.Items[a.To]
	if !ok {
		return notFound("item", a.To, "args.to_item_id")
	}
	for _, it := range []*timeline.Item{&from, &to} {
		if !isVisual(it.Type) {
			return Errorf(CodeTypeMismatch, "Item %s is an audio item; crossfade needs visual items.", it.ID).
				WithHint("Use item.set_audio fades for audio items.")
		}
	}
	if from.Track == to.Track {
		moved, err := c.crossfadeNeighbours(&from, &to, length, a.TrackID)
		if err != nil {
			return err
		}
		to = moved
	} else if a.TrackID != "" {
		return Errorf(
			CodeValidation,
			"args.track_id applies only to two items of the same track; %s and %s are on tracks %s and %s.",
			from.ID,
			to.ID,
			from.Track,
			to.Track,
		).
			WithHint("Leave track_id out.").
			WithPath("args.track_id")
	}
	if from.Start >= to.Start {
		return Errorf(CodeInvalidState, "Item %s starts at frame %d, not after %s at frame %d.",
			to.ID, to.Start, from.ID, from.Start).
			WithHint("Use the item that starts first as from_item_id.")
	}
	overlap := min(itemEnd(&from), itemEnd(&to)) - to.Start
	if overlap < length {
		e := Errorf(CodeInvalidState, "Items overlap by %d frames (%d-%d); a %d-frame crossfade needs %d.",
			max(overlap, 0), to.Start, to.Start+max(overlap, 0)-1, length, length).
			WithDetail("overlap", max(overlap, 0))
		if overlap >= 1 {
			return e.WithHint("Use duration \"%df\" or less, or move %s earlier with item.move.", overlap, to.ID)
		}
		return e.WithHint(
			"Move %s to start before frame %d with item.move, so the items overlap.",
			to.ID,
			itemEnd(&from),
		)
	}
	if err := crossfadeRoom(&from, from.TransitionIn, from.FadeIn, length, "transition_in", "fade_in"); err != nil {
		return err
	}
	if err := crossfadeRoom(&to, to.TransitionOut, to.FadeOut, length, "transition_out", "fade_out"); err != nil {
		return err
	}
	from.TransitionOut = &timeline.Transition{Kind: "fade", Duration: length}
	to.TransitionIn = &timeline.Transition{Kind: "fade", Duration: length}
	if from.Type == timeline.ItemClip {
		from.FadeOut = length
	}
	if to.Type == timeline.ItemClip {
		to.FadeIn = length
	}
	c.Doc.Items[from.ID], c.Doc.Items[to.ID] = from, to
	c.SetEffectiveTime("duration", length, string(a.Duration))
	c.SetEffective("overlap", overlap)
	return nil
}

// crossfadeNeighbours prepares a crossfade of two items that touch on one track (VV-042): the
// arriving item moves to another visual track and starts `length` frames earlier, reading the
// source it has before the cut, so the two overlap by `length` and the common rules apply.
func (c *Context) crossfadeNeighbours(from, to *timeline.Item, length int, trackID string) (timeline.Item, *Error) {
	if err := idForm("args.track_id", timeline.PrefixTrack, trackID); err != nil {
		return *to, err
	}
	if trackID == "" {
		return *to, Errorf(CodeInvalidState, "Items %s and %s are on the same track %s; a crossfade needs two tracks.",
			from.ID, to.ID, from.Track).
			WithHint("Send track_id, a visual track for %s, or move it to another track with item.move first.", to.ID)
	}
	if to.Start != itemEnd(from) {
		return *to, Errorf(CodeInvalidState, "Items %s and %s do not touch: %s ends at frame %d and %s starts at %d.",
			from.ID, to.ID, from.ID, itemEnd(from), to.ID, to.Start).
			WithHint("Use the neighbour that starts at frame %d as to_item_id.", itemEnd(from))
	}
	tr, err := c.track(trackID, "args.track_id")
	if err != nil {
		return *to, err
	}
	if trackID == from.Track || !acceptsType(tr, to.Type) {
		return *to, Errorf(CodeTypeMismatch, "Track %s is not another visual track than %s.", trackID, from.Track).
			WithHint("Use a visual track other than %s, adding one with track.add if needed.", from.Track).
			WithPath("args.track_id")
	}
	start := to.Start - length
	if start < 0 {
		return *to, Errorf(CodeInvalidState, "A %d-frame crossfade would start %s before frame 0 (it starts at %d).",
			length, to.ID, to.Start).
			WithHint("Use duration \"%df\" or less.", to.Start).WithPath("args.duration")
	}
	moved, terr := c.trimmed(to, "start", start, fmt.Sprintf("A %d-frame crossfade of %s", length, to.ID))
	if terr != nil {
		if terr.Code == CodeOutOfSource {
			terr = terr.WithHint(
				"Use a shorter duration (%s has less source before the cut), or move it to another track with item.move first.",
				to.ID,
			)
		}
		return *to, terr.WithPath("args.duration")
	}
	moved.Track = trackID
	if oerr := c.checkOverlap(to.ID, trackID, moved.Start, moved.Duration); oerr != nil {
		return *to, oerr.WithPath("args.track_id")
	}
	c.SetEffective("to_track_id", trackID)
	c.SetEffective("to_start", moved.Start)
	return moved, nil
}

// crossfadeRoom is R14 for one side of a crossfade: the new fade of the given length
// and the transition and audio fade of the other edge must fit in the item.
func crossfadeRoom(it *timeline.Item, other *timeline.Transition, fade, length int, trName, fadeName string) *Error {
	if other != nil && length+other.Duration > it.Duration {
		return Errorf(CodeInvalidState, "A %d-frame fade plus %s %d frames exceeds duration %d of %s.",
			length, trName, other.Duration, it.Duration, it.ID).
			WithHint("Use duration \"%df\" or less, or shorten %s with item.set_transition first.",
				max(it.Duration-other.Duration, 0), trName)
	}
	if it.Type == timeline.ItemClip && length+fade > it.Duration {
		return Errorf(CodeInvalidState, "A %d-frame fade plus %s %d frames exceeds duration %d of %s.",
			length, fadeName, fade, it.Duration, it.ID).
			WithHint("Use duration \"%df\" or less, or shorten %s with item.set_audio first.",
				max(it.Duration-fade, 0), fadeName)
	}
	return nil
}

type closeGapArgs struct {
	TrackID string `json:"track_id"         desc:"The track with the gap."                                       pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	At      Time   `json:"at"               desc:"Absolute frame inside the gap."`
	Ripple  string `json:"ripple,omitempty" desc:"track (default) or all: all moves later items of every track."                                                        enum:"track,all"`
}

func applyCloseGap(c *Context, a *closeGapArgs) error {
	if err := idForm("args.track_id", timeline.PrefixTrack, a.TrackID); err != nil {
		return err
	}
	at, err := c.ParseTime("args.at", string(a.At))
	if err != nil {
		return err
	}
	if _, err := c.track(a.TrackID, "args.track_id"); err != nil {
		return err
	}
	g0, g1 := 0, -1
	ids := c.trackItems(a.TrackID)
	for _, id := range ids {
		it := c.Doc.Items[id]
		switch {
		case it.Start <= at && at < itemEnd(&it):
			e := Errorf(CodeInvalidState, "Frame %d is inside item %s (frames %d-%d); there is no gap there.",
				at, id, it.Start, itemEnd(&it)-1).WithPath("args.at")
			if next := c.itemAfter(ids, &it); next != nil && next.Start > itemEnd(&it) {
				return e.WithHint("Use an at between \"%df\" and \"%df\", the gap after that item.",
					itemEnd(&it), next.Start-1)
			}
			return e.WithHint("Use an at in an empty range of the track.")
		case itemEnd(&it) <= at:
			g0 = max(g0, itemEnd(&it))
		case it.Start > at && (g1 < 0 || it.Start < g1):
			g1 = it.Start
		}
	}
	if g1 < 0 {
		return Errorf(CodeInvalidState, "Frame %d has no item after it on track %s; there is no gap to close.",
			at, a.TrackID).
			WithHint("Use an at before the last item of the track.").WithPath("args.at")
	}
	gap := g1 - g0
	shifts := map[string]int{}
	var moved []map[string]any
	for _, id := range c.sortedItemIDs() {
		it := c.Doc.Items[id]
		if it.Start >= g1 && (a.Ripple == "all" || it.Track == a.TrackID) {
			shifts[id] = gap
			moved = append(moved, map[string]any{"id": id, "from": it.Start, "to": it.Start - gap})
			it.Start -= gap
			c.Doc.Items[id] = it
		}
	}
	if a.Ripple == "all" {
		if err := c.overlapAfterShiftHint(shifts, "Use ripple \"track\" to move only this track."); err != nil {
			return err
		}
		c.rippleMarkers(g0, g1)
	}
	c.SetEffectiveTime("at", at, string(a.At))
	c.SetEffective("gap", map[string]any{"start": g0, "end": g1, "length": gap})
	c.SetEffective("moved", moved)
	return nil
}

// itemAfter returns the item that follows it in ids (sorted by start), or nil.
func (c *Context) itemAfter(ids []string, it *timeline.Item) *timeline.Item {
	i := slices.Index(ids, it.ID)
	if i < 0 || i+1 >= len(ids) {
		return nil
	}
	next := c.Doc.Items[ids[i+1]]
	return &next
}
