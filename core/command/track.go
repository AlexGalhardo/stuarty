package command

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "track.add",
		Summary: "Adds an empty track at a place in the track order.",
		Description: "Adds a visual or audio track. A visual track goes on top by default and an audio track at the bottom, " +
			"below the visual ones; position top or bottom, above or below another track places it elsewhere. The first track in the order is the top layer and covers the ones below it, " +
			"so a visual track added last covers the earlier ones: add the background first, or place a later track with below or position bottom. " +
			"The document keeps tracks in a map by id, so read the stacking from the order field (ascending, the smallest is the top layer), not from the position of a track in the listing. " +
			"The result says the layer of a visual track in stacking.",
		Example: `{"kind": "visual", "name": "Captions", "id": "trk_a1b2c3d4e5"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeIDExists, CodeLimit},
		Scope:   ScopeEdit,
	}, applyTrackAdd))

	Register(Define(Spec{
		Name:    "track.update",
		Summary: "Sets the name, hidden, muted, locked, solo or magnet flag or the volume of a track.",
		Description: "Sets the given fields. hidden applies to visual tracks, the others to both kinds. locked refuses " +
			"every edit of the items of the track (E_INVALID_STATE) until it is unlocked. solo plays only the soloed " +
			"tracks: the picture of visual tracks when one is solo, and the sound of every track. volume is a factor " +
			"from 0 to 4 on the volume of the items of the track; 1 clears it. magnet packs the items of the track from frame " +
			"0 with no gap, now and after every later edit (a locked track is not repacked).",
		Example: `{"track_id": "trk_f6g7h8j9k0", "hidden": true}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
	}, applyTrackUpdate))

	Register(Define(Spec{
		Name:        "track.move",
		Summary:     "Changes the place of a track in the track order.",
		Description: "Moves a track to the top or the bottom, or directly above or below another track. Items are untouched.",
		Example:     `{"track_id": "trk_m1n2p3q4r5", "below": "trk_a1b2c3d4e5"}`,
		Errors:      []string{CodeValidation, CodeNotFound, CodeInvalidState},
		Scope:       ScopeEdit,
	}, applyTrackMove))

	Register(Define(Spec{
		Name:    "track.remove",
		Summary: "Removes a track, and its items when with_items is true.",
		Description: "Removes the track. A track with items is refused with E_NOT_EMPTY unless with_items is true, in " +
			"which case its items are removed too.",
		Example: `{"track_id": "trk_f6g7h8j9k0", "with_items": true}`,
		Errors:  []string{CodeNotFound, CodeNotEmpty},
		Scope:   ScopeEdit,
	}, applyTrackRemove))

	Register(Define(Spec{
		Name:    "track.set_caption_style",
		Summary: "Merges a partial caption style into a track, or replaces it.",
		Description: "Sets the caption style of a visual track. The style is stored exactly as given, including keys equal " +
			"to the default. A null value removes a key when merging. highlight is \"none\" or \"word\" (the active word is coloured with highlight_color). A bottom position anchors to the bottom of the safe area, not of the frame.",
		Example: `{"track_id": "trk_a1b2c3d4e5", "style": {"size": 72, "position": "middle"}}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeFontUnknown},
		Scope:   ScopeEdit,
	}, applyTrackSetCaptionStyle))
}

// tracksByOrder returns the track ids sorted by order, without one.
func (c *Context) tracksByOrder(exclude string) []string {
	ids := make([]string, 0, len(c.Doc.Tracks))
	for id := range c.Doc.Tracks {
		if id != exclude {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b string) int { return strings.Compare(c.Doc.Tracks[a].Order, c.Doc.Tracks[b].Order) })
	return ids
}

// placeOrder computes the order for a track placed at top or bottom, or above or
// below ref, among the other tracks (SDD-002 C.6).
func (c *Context) placeOrder(exclude, position, above, below string) (string, *Error) {
	ids := c.tracksByOrder(exclude)
	order := func(i int) string {
		if i < 0 || i >= len(ids) {
			return ""
		}
		return c.Doc.Tracks[ids[i]].Order
	}
	var lo, hi string
	switch {
	case above != "":
		i := slices.Index(ids, above)
		lo, hi = order(i-1), order(i)
	case below != "":
		i := slices.Index(ids, below)
		lo, hi = order(i), order(i+1)
	case position == "bottom":
		lo = order(len(ids) - 1)
	default:
		hi = order(0)
	}
	got := timeline.OrderBetween(lo, hi)
	if got == "" {
		return "", Errorf(CodeInternal, "no track order fits between %q and %q.", lo, hi)
	}
	return got, nil
}

// visualTrackFor is the track an add command with no track_id puts an item on: the highest
// visual track where [start, start+length) is free, or a new visual track on top when
// every one is taken (SDD-002 C.6). auto is true when it chose the track.
func (c *Context) visualTrackFor(given string, start, length int) (track string, auto bool, err *Error) {
	if given != "" {
		return given, false, nil
	}
	for _, id := range c.tracksByOrder("") {
		if c.Doc.Tracks[id].Kind == timeline.TrackVisual && len(c.conflicts(id, start, length, "")) == 0 {
			return id, true, nil
		}
	}
	id, err := c.newID("tracks", timeline.PrefixTrack, "")
	if err != nil {
		return "", false, err
	}
	if err := c.limit("tracks", len(c.Doc.Tracks), timeline.MaxTracks); err != nil {
		return "", false, err
	}
	order, err := c.placeOrder("", "top", "", "")
	if err != nil {
		return "", false, err
	}
	c.Doc.Tracks[id] = timeline.Track{ID: id, Kind: timeline.TrackVisual, Order: order}
	return id, true, nil
}

type trackAddArgs struct {
	Kind     string `json:"kind"               enum:"visual,audio"`
	Name     string `json:"name,omitempty"                         max:"60"`
	ID       string `json:"id,omitempty"                                    pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Position string `json:"position,omitempty" enum:"top,bottom"                                                                   desc:"top (default) or bottom."`
	Above    string `json:"above,omitempty"                                 pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$" desc:"Place the track directly over this track."`
	Below    string `json:"below,omitempty"                                 pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$" desc:"Place the track directly under this track."`
}

func applyTrackAdd(c *Context, a *trackAddArgs) error {
	placements := 0
	for _, s := range []string{a.Position, a.Above, a.Below} {
		if s != "" {
			placements++
		}
	}
	if placements > 1 {
		return Errorf(
			CodeValidation,
			"args: position and above cannot be used together; give only one of position, above and below.",
		).
			WithHint("Send only one of position, above and below.").
			WithPath("args")
	}
	for _, f := range []struct{ path, id string }{{"args.id", a.ID}, {"args.above", a.Above}, {"args.below", a.Below}} {
		if err := idForm(f.path, timeline.PrefixTrack, f.id); err != nil {
			return err
		}
	}
	for _, f := range []struct{ path, id string }{{"args.above", a.Above}, {"args.below", a.Below}} {
		if f.id != "" {
			if _, err := c.track(f.id, f.path); err != nil {
				return err
			}
		}
	}
	id, err := c.newID("tracks", timeline.PrefixTrack, a.ID)
	if err != nil {
		return err
	}
	if err := c.limit("tracks", len(c.Doc.Tracks), timeline.MaxTracks); err != nil {
		return err
	}
	position := a.Position
	if position == "" && a.Above == "" && a.Below == "" && a.Kind == string(timeline.TrackAudio) {
		position = "bottom"
	}
	order, err := c.placeOrder("", position, a.Above, a.Below)
	if err != nil {
		return err
	}
	c.Doc.Tracks[id] = timeline.Track{ID: id, Kind: timeline.TrackKind(a.Kind), Name: a.Name, Order: order}
	c.SetEffective("track_id", id)
	c.SetEffective("order", order)
	if a.Kind == string(timeline.TrackVisual) {
		c.SetEffective("stacking", c.stackingOf(id))
	}
	return nil
}

// stackingOf says where a visual track lies in the picture, so that an agent does not find out from the
// preview that a track it added last covers the others (VV-203).
func (c *Context) stackingOf(id string) string {
	layer, total := 0, 0
	for _, other := range c.tracksByOrder("") {
		if c.Doc.Tracks[other].Kind != timeline.TrackVisual {
			continue
		}
		total++
		if other == id {
			layer = total
		}
	}
	return fmt.Sprintf(
		"layer %d of %d visual tracks, counting from the top; a track with a lower number covers the ones with a higher number.",
		layer,
		total,
	)
}

type trackUpdateArgs struct {
	TrackID string   `json:"track_id"         pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Name    *string  `json:"name,omitempty"                                                          max:"60" desc:"The track name; \"\" clears it."`
	Hidden  *bool    `json:"hidden,omitempty"                                                                 desc:"Visual tracks only."`
	Muted   *bool    `json:"muted,omitempty"`
	Locked  *bool    `json:"locked,omitempty"                                                                 desc:"true refuses every edit of the items of the track until it is false again."`
	Solo    *bool    `json:"solo,omitempty"                                                                   desc:"true plays only the soloed tracks (the picture of visual tracks, the sound of all)."`
	Magnet  *bool    `json:"magnet,omitempty"                                                                 desc:"true keeps the items of the track packed from frame 0 with no gap, closing the gaps now and after every edit."`
	Volume  *float64 `json:"volume,omitempty"                                                        max:"4"  desc:"Factor on the volume of the items of the track, 0 to 4; 1 is neutral."                                         min:"0"`
}

func applyTrackUpdate(c *Context, a *trackUpdateArgs) error {
	if err := atLeastOne(
		[]string{"name", "hidden", "muted", "locked", "solo", "magnet", "volume"},
		a.Name != nil,
		a.Hidden != nil,
		a.Muted != nil,
		a.Locked != nil,
		a.Solo != nil,
		a.Magnet != nil,
		a.Volume != nil,
	); err != nil {
		return err
	}
	if err := idForm("args.track_id", timeline.PrefixTrack, a.TrackID); err != nil {
		return err
	}
	t, err := c.track(a.TrackID, "args.track_id")
	if err != nil {
		return err
	}
	if a.Hidden != nil && t.Kind != timeline.TrackVisual {
		return Errorf(CodeTypeMismatch, "Track %s is an audio track; hidden applies to visual tracks only.", a.TrackID).
			WithHint("Use muted to silence an audio track.").WithPath("args.hidden")
	}
	if a.Name != nil {
		t.Name = *a.Name
		c.SetEffective("name", t.Name)
	}
	if a.Hidden != nil {
		t.Hidden = *a.Hidden
		c.SetEffective("hidden", t.Hidden)
	}
	if a.Muted != nil {
		t.Muted = *a.Muted
		c.SetEffective("muted", t.Muted)
	}
	if a.Locked != nil {
		t.Locked = *a.Locked
		c.SetEffective("locked", t.Locked)
	}
	if a.Solo != nil {
		t.Solo = *a.Solo
		c.SetEffective("solo", t.Solo)
	}
	if a.Magnet != nil {
		t.Magnet = *a.Magnet
		c.SetEffective("magnet", t.Magnet)
	}
	if a.Volume != nil {
		if *a.Volume < 0 || *a.Volume > 4 {
			return invalidArg("args.volume", "The value of args.volume is %v, outside the accepted range.", *a.Volume).
				WithHint("Use a value between 0 and 4.")
		}
		v := round3(*a.Volume)
		t.Volume = &v
		if v == 1 {
			t.Volume = nil
		}
		c.SetEffective("volume", v)
	}
	c.Doc.Tracks[a.TrackID] = t
	return nil
}

type trackMoveArgs struct {
	TrackID  string `json:"track_id"           pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Position string `json:"position,omitempty"                                                        enum:"top,bottom"`
	Above    string `json:"above,omitempty"    pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Below    string `json:"below,omitempty"    pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyTrackMove(c *Context, a *trackMoveArgs) error {
	if err := requireOne(
		[]string{"position", "above", "below"},
		a.Position != "",
		a.Above != "",
		a.Below != "",
	); err != nil {
		return err
	}
	if err := idForm("args.track_id", timeline.PrefixTrack, a.TrackID); err != nil {
		return err
	}
	t, err := c.track(a.TrackID, "args.track_id")
	if err != nil {
		return err
	}
	ref, refPath := a.Above+a.Below, "args.above"
	if a.Below != "" {
		refPath = "args.below"
	}
	if ref != "" {
		if err := idForm(refPath, timeline.PrefixTrack, ref); err != nil {
			return err
		}
		if _, err := c.track(ref, refPath); err != nil {
			return err
		}
		if ref == a.TrackID {
			word := "above"
			if a.Below != "" {
				word = "below"
			}
			return Errorf(CodeInvalidState, "Track %s cannot be placed %s itself.", a.TrackID, word).
				WithHint("Name a different reference track, or use position.").WithPath(refPath)
		}
	}
	all := c.tracksByOrder("")
	i := slices.Index(all, a.TrackID)
	inPlace := a.Position == "top" && i == 0 || a.Position == "bottom" && i == len(all)-1 ||
		a.Above != "" && i+1 < len(all) && all[i+1] == a.Above || a.Below != "" && i > 0 && all[i-1] == a.Below
	if !inPlace {
		order, err := c.placeOrder(a.TrackID, a.Position, a.Above, a.Below)
		if err != nil {
			return err
		}
		t.Order = order
		c.Doc.Tracks[a.TrackID] = t
	}
	c.SetEffective("track_id", a.TrackID)
	c.SetEffective("order", t.Order)
	return nil
}

type trackRemoveArgs struct {
	TrackID   string `json:"track_id"             pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	WithItems bool   `json:"with_items,omitempty"                                                        desc:"Remove the items of the track too."`
}

func applyTrackRemove(c *Context, a *trackRemoveArgs) error {
	if err := idForm("args.track_id", timeline.PrefixTrack, a.TrackID); err != nil {
		return err
	}
	if _, err := c.track(a.TrackID, "args.track_id"); err != nil {
		return err
	}
	var items []string
	for _, id := range c.sortedItemIDs() {
		if c.Doc.Items[id].Track == a.TrackID {
			items = append(items, id)
		}
	}
	if len(items) > 0 && !a.WithItems {
		return Errorf(CodeNotEmpty, "Track %s has %d items: %s.", a.TrackID, len(items), strings.Join(items, ", ")).
			WithHint("Send \"with_items\": true to remove the items with the track, or move them first.").
			WithDetail("items", items)
	}
	for _, id := range items {
		delete(c.Doc.Items, id)
	}
	delete(c.Doc.Tracks, a.TrackID)
	if items == nil {
		items = []string{}
	}
	c.SetEffective("track_id", a.TrackID)
	c.SetEffective("removed_items", items)
	return nil
}

type trackCaptionStyleArgs struct {
	TrackID string         `json:"track_id"          pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Style   map[string]any `json:"style"                                                                    schema:"caption_style_merge" desc:"A partial CaptionStyle; null removes a key when merging."`
	Replace bool           `json:"replace,omitempty"                                                                                     desc:"Replace the whole style instead of merging."`
}

func applyTrackSetCaptionStyle(c *Context, a *trackCaptionStyleArgs) error {
	if err := idForm("args.track_id", timeline.PrefixTrack, a.TrackID); err != nil {
		return err
	}
	if err := c.validateStyle(a.Style, true, true); err != nil {
		return err
	}
	t, err := c.track(a.TrackID, "args.track_id")
	if err != nil {
		return err
	}
	if t.Kind != timeline.TrackVisual {
		return Errorf(CodeTypeMismatch, "Track %s is an audio track and cannot hold captions.", a.TrackID).
			WithHint("Use a visual track.").WithPath("args.track_id")
	}
	merged, err := mergeStyle(t.CaptionStyle, a.Style, a.Replace)
	if err != nil {
		return err
	}
	if err := c.checkFont(merged); err != nil {
		return err
	}
	t.CaptionStyle = merged
	c.Doc.Tracks[a.TrackID] = t
	stored := timeline.StyleToMap(merged)
	if stored == nil {
		stored = map[string]any{}
	}
	c.SetEffective("caption_style", stored)
	return nil
}

// packMagnetTracks closes every gap on the tracks with magnet: the items, in order of start, are laid
// end to end from frame 0 (SDD-001 §3.3). A locked track is left as it is. Run after every command, so the
// result of any edit is packed and one undo step holds both the edit and the repack.
func packMagnetTracks(doc *timeline.Document) {
	for id, t := range doc.Tracks {
		if !t.Magnet || t.Locked {
			continue
		}
		var ids []string
		for itemID := range doc.Items {
			if doc.Items[itemID].Track == id {
				ids = append(ids, itemID)
			}
		}
		slices.SortFunc(ids, func(a, b string) int {
			return cmp.Or(cmp.Compare(doc.Items[a].Start, doc.Items[b].Start), cmp.Compare(a, b))
		})
		at := 0
		for _, itemID := range ids {
			it := doc.Items[itemID]
			it.Start = at
			doc.Items[itemID] = it
			at += it.Duration
		}
	}
}
