package command

import (
	"fmt"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "item.link",
		Summary: "Links items so that they move, trim and split together.",
		Description: "Puts 2 to 20 clip, audio, text or shape items in one link group. item.move, item.trim and item.split " +
			"on one of them apply the same change to the others, and a locked track refuses the edit that would " +
			"change a linked item on it. An item belongs to one group; unlink it first to link it elsewhere.",
		Example: `{"item_ids": ["itm_m1n2p3q4r5", "itm_c3d4e5f6g7"]}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeIDExists,
		},
		Scope: ScopeEdit,
		Phase: Beta,
	}, applyItemLink))

	Register(Define(Spec{
		Name:    "item.unlink",
		Summary: "Removes items from their link group.",
		Description: "Takes each listed item out of its link group. A group left with one item is dissolved, so the " +
			"item that remains is unlinked too (listed in effective.also_unlinked).",
		Example: `{"item_ids": ["itm_c3d4e5f6g7"]}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeInvalidState},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyItemUnlink))

	Register(Define(Spec{
		Name:    "item.extract_audio",
		Summary: "Creates an audio item from the audio of a video clip, linked to it, and mutes the clip.",
		Description: "Adds an audio item on an audio track with the same asset, start, duration, source range, speed, " +
			"curve, reverse, volume, fades, noise reduction and volume keyframes as the clip, links the two, and sets " +
			"audio_muted on the clip so the sound is not heard twice.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "track_id": "trk_m1n2p3q4r5"}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeOverlap, CodeIDExists, CodeLimit,
		},
		Scope: ScopeEdit,
		Phase: Beta,
	}, applyItemExtractAudio))
}

// linkIDFrom is the link group id that goes with a new item id: the same ten characters under
// the link prefix. A command that makes a group while making an item takes its id from the item,
// so that the one generated id is all a replay needs (SDD-002 A.3 rule 4).
func linkIDFrom(itemID string) string {
	return timeline.PrefixLink + "_" + strings.TrimPrefix(itemID, timeline.PrefixItem+"_")
}

// linkPeers returns the ids of the other items of the link group of it, sorted.
func (c *Context) linkPeers(it *timeline.Item) []string {
	if it.Link == "" {
		return nil
	}
	var ids []string
	for id := range c.Doc.Items {
		if o := c.Doc.Items[id]; id != it.ID && o.Link == it.Link {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// checkItemIDs validates a list of item ids: the form, no repeats, and that each exists.
func (c *Context) checkItemIDs(ids []string) *Error {
	seen := map[string]bool{}
	for i, id := range ids {
		p := fmt.Sprintf("args.item_ids[%d]", i)
		if err := idForm(p, timeline.PrefixItem, id); err != nil {
			return err
		}
		if seen[id] {
			return Errorf(CodeValidation, "%s %q is listed twice.", p, id).
				WithHint("List each item id once.").WithPath(p)
		}
		seen[id] = true
		if _, ok := c.Doc.Items[id]; !ok {
			return notFound("item", id, p)
		}
	}
	return nil
}

type itemLinkArgs struct {
	ItemIDs []string `json:"item_ids"     desc:"The items to link, 2 to 20."                                               minitems:"2" maxitems:"20" pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	ID      string   `json:"id,omitempty" desc:"The id of the link group, lnk_ and 10 characters; generated when omitted."                            pattern:"^lnk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyItemLink(c *Context, a *itemLinkArgs) error {
	if len(a.ItemIDs) < 2 || len(a.ItemIDs) > 20 {
		return Errorf(CodeValidation, "args.item_ids has %d items; link 2 to 20.", len(a.ItemIDs)).
			WithHint("Send at least two item ids.").WithPath("args.item_ids")
	}
	if err := idForm("args.id", timeline.PrefixLink, a.ID); err != nil {
		return err
	}
	if err := c.checkItemIDs(a.ItemIDs); err != nil {
		return err
	}
	for i, id := range a.ItemIDs {
		it := c.Doc.Items[id]
		p := fmt.Sprintf("args.item_ids[%d]", i)
		switch {
		case it.Type == timeline.ItemCaption:
			return Errorf(
				CodeTypeMismatch,
				"Item %s is a caption; captions follow a clip with linked_item, not a link group.",
				id,
			).
				WithHint("Link clip, audio, text or shape items.").
				WithPath(p)
		case it.Link != "":
			return Errorf(CodeInvalidState, "Item %s is already linked (group %s).", id, it.Link).
				WithHint("Unlink it with item.unlink first.").WithDetail("link", it.Link).WithPath(p)
		}
	}
	link, err := c.NewID(timeline.PrefixLink, a.ID)
	if err != nil {
		return err
	}
	for _, id := range c.sortedItems() {
		if c.Doc.Items[id].Link == link {
			return Errorf(CodeIDExists, "Link id %s is already used by item %s.", link, id).
				WithHint("Omit id or use an unused id.").WithPath("args.id")
		}
	}
	for _, id := range a.ItemIDs {
		it := c.Doc.Items[id]
		it.Link = link
		c.Doc.Items[id] = it
	}
	c.SetEffective("link_id", link)
	c.SetEffective("item_ids", slices.Sorted(slices.Values(a.ItemIDs)))
	return nil
}

type itemUnlinkArgs struct {
	ItemIDs []string `json:"item_ids" desc:"The items to take out of their link group." minitems:"1" maxitems:"50" pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyItemUnlink(c *Context, a *itemUnlinkArgs) error {
	if len(a.ItemIDs) < 1 || len(a.ItemIDs) > 50 {
		return Errorf(CodeValidation, "args.item_ids has %d items; send 1 to 50.", len(a.ItemIDs)).
			WithPath("args.item_ids")
	}
	if err := c.checkItemIDs(a.ItemIDs); err != nil {
		return err
	}
	groups := map[string]bool{}
	for i, id := range a.ItemIDs {
		if it := c.Doc.Items[id]; it.Link == "" {
			return Errorf(CodeInvalidState, "Item %s is not linked.", id).
				WithHint("Only linked items can be unlinked.").WithPath(fmt.Sprintf("args.item_ids[%d]", i))
		}
	}
	for _, id := range a.ItemIDs {
		it := c.Doc.Items[id]
		groups[it.Link] = true
		it.Link = ""
		c.Doc.Items[id] = it
	}
	also := []string{}
	for _, id := range c.sortedItems() {
		o := c.Doc.Items[id]
		if !groups[o.Link] || o.Link == "" {
			continue
		}
		if len(c.linkPeers(&o)) == 0 {
			o.Link = ""
			c.Doc.Items[id] = o
			also = append(also, id)
		}
	}
	c.SetEffective("item_ids", slices.Sorted(slices.Values(a.ItemIDs)))
	c.SetEffective("also_unlinked", also)
	return nil
}

type itemExtractAudioArgs struct {
	ItemID  string `json:"item_id"      desc:"A clip of a video asset with audio."    pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	TrackID string `json:"track_id"     desc:"An audio track with room for the item." pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	ID      string `json:"id,omitempty" desc:"The id of the new audio item."          pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyItemExtractAudio(c *Context, a *itemExtractAudioArgs) error {
	for _, f := range []struct{ path, prefix, id string }{
		{"args.item_id", timeline.PrefixItem, a.ItemID},
		{"args.track_id", timeline.PrefixTrack, a.TrackID},
		{"args.id", timeline.PrefixItem, a.ID},
	} {
		if err := idForm(f.path, f.prefix, f.id); err != nil {
			return err
		}
	}
	it, err := c.clipItem(a.ItemID, "extracting audio")
	if err != nil {
		return err
	}
	tr, err := c.track(a.TrackID, "args.track_id")
	if err != nil {
		return err
	}
	if tr.Kind != timeline.TrackAudio {
		return Errorf(
			CodeTypeMismatch,
			"Track %s is a %s track; the audio item needs an audio track.",
			a.TrackID,
			tr.Kind,
		).
			WithHint("Use an audio track, for example the one from track.add.").
			WithPath("args.track_id")
	}
	if asset, ok := c.Doc.Assets[it.Asset]; !ok || asset.Kind != timeline.AssetVideo || !asset.HasAudio {
		return Errorf(CodeTypeMismatch, "The asset of clip %s has no audio to extract.", a.ItemID).
			WithHint("Choose a clip placed from a video asset with audio.").WithPath("args.item_id")
	}
	if it.AudioMuted {
		return Errorf(CodeInvalidState, "Clip %s is already muted: its audio was extracted or silenced.", a.ItemID).
			WithHint("Unmute it with item.set_audio audio_muted false first, or use its audio item.").
			WithPath("args.item_id")
	}
	id, err := c.newID("items", timeline.PrefixItem, a.ID)
	if err != nil {
		return err
	}
	if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
		return err
	}
	if err := c.checkOverlap(id, a.TrackID, it.Start, it.Duration); err != nil {
		return err.WithPath("args.track_id")
	}

	au := timeline.NewItem(id, timeline.ItemAudio, a.TrackID, it.Start, it.Duration)
	au.Asset, au.SourceIn, au.Speed, au.Reverse = it.Asset, it.SourceIn, it.Speed, it.Reverse
	au.SpeedCurve = slices.Clone(it.SpeedCurve)
	au.Volume, au.FadeIn, au.FadeOut, au.Denoise, au.Voice = it.Volume, it.FadeIn, it.FadeOut, it.Denoise, it.Voice
	if kfs, ok := it.Keyframes["volume"]; ok {
		au.Keyframes = timeline.Keyframes{"volume": slices.Clone(kfs)}
	}
	link := it.Link
	if link == "" {
		link = linkIDFrom(id)
	}
	it.AudioMuted, it.Link, au.Link = true, link, link
	c.Doc.Items[a.ItemID], c.Doc.Items[id] = it, au
	c.SetEffective("new_id", id)
	c.SetEffective("link", link)
	c.SetEffective("track_id", a.TrackID)
	return nil
}

// movePeers moves the linked items of it by delta frames, writing them into the document, and
// returns their ids. The group moves as one: a peer that would start before 0 or end after
// the limit refuses the move.
func (c *Context) movePeers(it *timeline.Item, delta int) ([]string, *Error) {
	peers := c.linkPeers(it)
	if delta == 0 || len(peers) == 0 {
		return nil, nil
	}
	for _, id := range peers {
		o := c.Doc.Items[id]
		if o.Start+delta < 0 || o.Start+delta+o.Duration > timeline.MaxFrames {
			return nil, Errorf(
				CodeInvalidTime,
				"Item %s is linked to %s and would move to frame %d, outside 0 to %d.",
				id, it.ID, o.Start+delta, timeline.MaxFrames,
			).
				WithHint("Move by less, or unlink %s with item.unlink.", id).WithPath("args.start")
		}
	}
	for _, id := range peers {
		o := c.Doc.Items[id]
		o.Start += delta
		c.Doc.Items[id] = o
	}
	return peers, nil
}

// trimPeers moves the same edge of the linked items of old by the same number of frames as the
// edge of old moved to `to`, writing them into the document, and returns their ids.
func (c *Context) trimPeers(old *timeline.Item, edge string, to int) ([]string, *Error) {
	peers := c.linkPeers(old)
	from := itemEnd(old)
	if edge == "start" {
		from = old.Start
	}
	delta := to - from
	if delta == 0 || len(peers) == 0 {
		return nil, nil
	}
	for _, id := range peers {
		o := c.Doc.Items[id]
		at := itemEnd(&o) + delta
		if edge == "start" {
			at = o.Start + delta
		}
		n, err := c.trimmed(&o, edge, at, fmt.Sprintf(
			"Trimming the %s of %s moves the %s of its linked item %s and", edge, old.ID, edge, id))
		if err != nil {
			return nil, err.WithHint("Trim by less, or unlink %s with item.unlink.", id).WithPath("args.to")
		}
		c.Doc.Items[id] = n
	}
	return peers, nil
}

// splitPeers splits the items linked to the one just split (it, in its state before the split;
// newID is the id of its right part) that the frame `at` falls strictly inside. The right parts
// form a new link group together; the left parts keep the old one. When no linked item is cut the
// right part of it is left unlinked, since a group of one links nothing.
func (c *Context) splitPeers(it *timeline.Item, at int, newID string, supplied []string) *Error {
	var cut []string
	for _, id := range c.linkPeers(it) {
		if o := c.Doc.Items[id]; at > o.Start && at < itemEnd(&o) {
			cut = append(cut, id)
		}
	}
	right := c.Doc.Items[newID]
	if len(cut) == 0 {
		right.Link = ""
		c.Doc.Items[newID] = right
		return nil
	}
	ids, _, err := c.takeNewIDs(supplied, len(cut))
	if err != nil {
		return err
	}
	group := linkIDFrom(newID)
	right.Link = group
	c.Doc.Items[newID] = right
	for i, id := range cut {
		if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
			return err
		}
		o := c.Doc.Items[id]
		l, r, err := c.splitParts(&o, at, ids[i])
		if err != nil {
			return err.WithHint("Item %s is linked to %s and is split with it; %s", id, it.ID, err.Hint)
		}
		r.Link = group
		c.Doc.Items[id], c.Doc.Items[ids[i]] = l, r
	}
	c.SetEffective("new_ids", ids)
	c.SetEffective("split_with", cut)
	return nil
}
