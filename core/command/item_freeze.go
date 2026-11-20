package command

import (
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// defaultFreezeDuration is how long a freeze frame is held when the caller gives no duration.
const defaultFreezeDuration = "3s"

func init() {
	Register(Define(Spec{
		Name:    "item.freeze_frame",
		Summary: "Holds one frame of a clip as a still inserted at that frame.",
		Description: "Splits the clip at a timeline frame and puts between the two parts a new clip that shows the " +
			"source frame at that point for duration (default 3s), with no sound. The right part and the later " +
			"items of the track move later by duration; other tracks are untouched. The freeze frame has no " +
			"keyframes. Not for linked items or items with a speed curve.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "at": "97f", "duration": "2s"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeIDExists, CodeLimit,
		},
		Scope: ScopeEdit,
	}, applyItemFreezeFrame))
}

type itemFreezeArgs struct {
	ItemID   string `json:"item_id"             desc:"A clip of a video asset."                              pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	At       Time   `json:"at"                  desc:"Absolute frame inside the clip whose picture is held."`
	Duration Time   `json:"duration,omitempty"  desc:"How long the frame is held; default 3s."`
	FreezeID string `json:"freeze_id,omitempty" desc:"Id for the freeze frame item."                         pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	NewID    string `json:"new_id,omitempty"    desc:"Id for the right part of the split."                   pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyItemFreezeFrame(c *Context, a *itemFreezeArgs) error {
	for _, f := range []struct{ path, id string }{
		{"args.item_id", a.ItemID}, {"args.freeze_id", a.FreezeID}, {"args.new_id", a.NewID},
	} {
		if err := idForm(f.path, timeline.PrefixItem, f.id); err != nil {
			return err
		}
	}
	at, err := c.ParseTime("args.at", string(a.At))
	if err != nil {
		return err
	}
	requested := string(a.Duration)
	if requested == "" {
		requested = defaultFreezeDuration
	}
	hold, err := c.length("args.duration", requested, false)
	if err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if it.Type != timeline.ItemClip {
		return Errorf(
			CodeTypeMismatch,
			"Item %s is %s item; a freeze frame needs a clip of a video asset.",
			a.ItemID,
			article(it.Type),
		).
			WithHint("Use a clip of a video asset.").
			WithPath("args.item_id")
	}
	if _, ok := c.sourceAsset(&it); !ok {
		return Errorf(CodeTypeMismatch, "Item %s shows an image, which is already a still.", a.ItemID).
			WithHint("Use a clip of a video asset.").WithPath("args.item_id")
	}
	if it.Freeze {
		return Errorf(CodeInvalidState, "Item %s is already a freeze frame.", a.ItemID).
			WithHint("Change its length with item.trim.").WithPath("args.item_id")
	}
	if it.Link != "" {
		return Errorf(
			CodeInvalidState,
			"Item %s is linked to other items, which a freeze frame would put out of step.",
			a.ItemID,
		).
			WithHint("Unlink it with item.unlink first.").
			WithPath("args.item_id")
	}
	if at <= it.Start || at >= itemEnd(&it) {
		return Errorf(
			CodeInvalidTime,
			"args.at \"%s\" is not inside item %s (frames %d-%d); it must be between %d and %d inclusive.",
			a.At, a.ItemID, it.Start, itemEnd(&it)-1, it.Start+1, itemEnd(&it)-1,
		).
			WithHint("Use at \"%df\" or later, up to \"%df\".", it.Start+1, itemEnd(&it)-1).
			WithPath("args.at")
	}
	if len(c.Doc.Items)+2 > timeline.MaxItems {
		return Errorf(CodeLimit, "The document has %d items; a freeze frame adds two and the limit is %d.",
			len(c.Doc.Items), timeline.MaxItems).
			WithHint("Delete unused items first.")
	}
	freezeID, err := c.newID("items", timeline.PrefixItem, a.FreezeID)
	if err != nil {
		return err
	}
	newID, err := c.NewID(timeline.PrefixItem, a.NewID)
	if err != nil {
		return err
	}
	if newID == freezeID {
		return Errorf(CodeValidation, "args.freeze_id and args.new_id are the same id.").
			WithHint("Use two different ids or omit them.").WithPath("args.new_id")
	}
	if _, used := c.Doc.Items[newID]; used {
		return Errorf(CodeIDExists, "Item id %s already exists.", newID).
			WithHint("Omit new_id or use an unused id.").WithPath("args.new_id")
	}
	if end := c.trackEnd(it.Track); end+hold > timeline.MaxFrames {
		return Errorf(
			CodeInvalidTime,
			"Holding the frame for %d frames would move the end of the track from frame %d to %d, past the limit %d.",
			hold,
			end,
			end+hold,
			timeline.MaxFrames,
		).
			WithHint("Use a duration of at most \"%df\".", max(timeline.MaxFrames-end, 1)).
			WithPath("args.duration")
	}

	tc := at - it.Start
	frame := it.SourceIn + it.SourceOffset(tc)
	left, right, err := c.splitParts(&it, at, newID)
	if err != nil {
		return err
	}
	freeze := it
	freeze.ID, freeze.Start, freeze.Duration = freezeID, at, hold
	freeze.Freeze, freeze.SourceIn = true, frame
	freeze.Speed, freeze.SpeedCurve, freeze.Reverse = timeline.Rational{Num: 1, Den: 1}, nil, false
	freeze.AudioMuted, freeze.FadeIn, freeze.FadeOut = true, 0, 0
	freeze.TransitionIn, freeze.TransitionOut, freeze.Keyframes, freeze.Slot, freeze.Markers = nil, nil, nil, nil, nil
	right.Start += hold

	end := itemEnd(&it)
	for id := range c.Doc.Items {
		other := c.Doc.Items[id]
		if other.Track == it.Track && other.ID != it.ID && other.Start >= end {
			other.Start += hold
			c.Doc.Items[id] = other
		}
	}
	c.Doc.Items[a.ItemID], c.Doc.Items[freezeID], c.Doc.Items[newID] = left, freeze, right
	c.SetEffectiveTime("at", at, string(a.At))
	c.SetEffectiveTime("duration", hold, requested)
	c.SetEffective("freeze_id", freezeID)
	c.SetEffective("new_id", newID)
	c.SetEffective("source_frame", frame)
	c.SetEffective("left", map[string]any{"id": a.ItemID, "start": left.Start, "duration": left.Duration})
	c.SetEffective("freeze", map[string]any{"id": freezeID, "start": freeze.Start, "duration": freeze.Duration})
	c.SetEffective("right", map[string]any{"id": newID, "start": right.Start, "duration": right.Duration})
	return nil
}

// trackEnd is the first frame after the last item of a track.
func (c *Context) trackEnd(track string) int {
	end := 0
	for id := range c.Doc.Items {
		if it := c.Doc.Items[id]; it.Track == track {
			end = max(end, itemEnd(&it))
		}
	}
	return end
}

// startOrTrackEnd is the start of an added item: the time given, or the end of the last item
// of the track when it is omitted, which is always a free position (VV-158).
func (c *Context) startOrTrackEnd(track string, start Time) (int, *Error) {
	if start == "" {
		return c.trackEnd(track), nil
	}
	return c.ParseTime("args.start", string(start))
}
