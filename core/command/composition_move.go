package command

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// composition.create_from_selection and composition.expand (SDD-002 C.79, C.80): the two directions between a
// part of a timeline and a timeline of its own that the first one uses as a clip.

func init() {
	Register(Define(Spec{
		Name:    "composition.create_from_selection",
		Summary: "Moves the selected items into a new timeline and puts one composition clip in their place.",
		Description: "Makes a new timeline of the project from the selected items (same tracks, same frame size and rate, " +
			"starting at frame 0) and replaces them with one composition of it, on the top-most visual track of the selection, " +
			"for the same frames. The inverse puts the items back; the new timeline stays in the project.",
		Example: `{"item_ids": ["itm_m1n2p3q4r5", "itm_c3d4e5f6g7"], "name": "Intro", "timeline_id": "prj_a1b2c3d4e5", "id": "itm_k1k2k3k4k5"}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeInvalidState, CodeIDExists, CodeOverlap, CodeLimit,
		},
		Scope: ScopeEdit,
	}, applyCompositionCreate))

	Register(Define(Spec{
		Name:    "composition.expand",
		Summary: "Puts the items of a composition's timeline back on this timeline in place of the composition.",
		Description: "The inverse of composition.create_from_selection, for a composition that only places its timeline: no opacity, " +
			"mask, effect, transform, keyframe, transition, blend or volume of its own, the whole timeline shown, and the same frame " +
			"size and rate. The nested timeline stays in the project.",
		Example: `{"item_id": "itm_k1k2k3k4k5"}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeIDExists, CodeLimit,
		},
		Scope: ScopeEdit,
	}, applyCompositionExpand))
}

type compositionCreateArgs struct {
	ItemIDs    []string `json:"item_ids"              desc:"The items to move, each once."                      min:"1" max:"100" pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Name       string   `json:"name,omitempty"        desc:"The name of the new timeline; default Composition." min:"1" max:"120"`
	TimelineID string   `json:"timeline_id,omitempty" desc:"The id of the new timeline."                                          pattern:"^prj_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	ID         string   `json:"id,omitempty"          desc:"The id of the composition item."                                      pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

// selection is the items to move, in the order of their ids, and what they cover.
type selection struct {
	ids        []string
	start, end int
	track      string // the top-most visual track of the selection
}

// selected reads and checks the items named by the arguments (preconditions 1 and 3 of C.79).
func (c *Context) selected(a *compositionCreateArgs) (*selection, *Error) {
	seen := map[string]bool{}
	sel := &selection{start: timeline.MaxFrames}
	topOrder := ""

	for i, id := range a.ItemIDs {
		p := fmt.Sprintf("args.item_ids[%d]", i)
		if err := idForm(p, timeline.PrefixItem, id); err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, Errorf(CodeValidation, "%s repeats %s.", "args.item_ids", id).
				WithHint("Name each item once.").WithPath(p)
		}
		seen[id] = true

		it, ok := c.Doc.Items[id]
		if !ok {
			return nil, notFound("item", id, p)
		}

		sel.ids = append(sel.ids, id)
		sel.start, sel.end = min(sel.start, it.Start), max(sel.end, itemEnd(&it))

		if tr := c.Doc.Tracks[it.Track]; tr.Kind == timeline.TrackVisual && (topOrder == "" || tr.Order < topOrder) {
			topOrder, sel.track = tr.Order, tr.ID
		}
	}

	slices.Sort(sel.ids)

	return sel, nil
}

// topVisualTrack is the visual track with the smallest order, for a selection of audio items only.
func (c *Context) topVisualTrack() string {
	best := ""
	for id := range c.Doc.Tracks {
		tr := c.Doc.Tracks[id]
		if tr.Kind == timeline.TrackVisual && (best == "" || tr.Order < c.Doc.Tracks[best].Order) {
			best = id
		}
	}

	return best
}

// moveToTimeline builds the document of the new timeline from the selected items (postcondition 1 of C.79) and
// takes the items out of this one.
func (c *Context) moveToTimeline(sel *selection, tid, name string) *timeline.Document {
	inner := &timeline.Document{
		SchemaVersion: c.Doc.SchemaVersion, ID: tid, Name: name, Settings: c.Doc.Settings,
		Assets: map[string]timeline.Asset{}, Tracks: map[string]timeline.Track{}, Items: map[string]timeline.Item{},
	}

	moved := map[string]bool{}
	for _, id := range sel.ids {
		moved[id] = true
	}

	// A link group goes with its items only when every member goes.
	whole := map[string]bool{}
	members := map[string]int{}
	for id := range c.Doc.Items {
		if l := c.Doc.Items[id].Link; l != "" {
			members[l]++
		}
	}
	inGroup := map[string]int{}
	for _, id := range sel.ids {
		if l := c.Doc.Items[id].Link; l != "" {
			inGroup[l]++
		}
	}
	for l, n := range inGroup {
		whole[l] = n == members[l]
	}

	for _, id := range sel.ids {
		it := c.Doc.Items[id]
		it.Start -= sel.start
		it.Slot = nil
		if it.Link != "" && !whole[it.Link] {
			it.Link = ""
		}
		if it.LinkedItem != "" && !moved[it.LinkedItem] {
			it.LinkedItem = ""
		}

		inner.Items[id] = it
		inner.Tracks[it.Track] = c.Doc.Tracks[it.Track]

		for _, ref := range []string{it.Asset, lutOf(&it)} {
			if a, ok := c.Doc.Assets[ref]; ok {
				inner.Assets[ref] = a
			}
		}

		delete(c.Doc.Items, id)
	}

	if len(c.Doc.Fonts) > 0 {
		inner.Fonts = maps.Clone(c.Doc.Fonts)
	}

	return inner
}

func lutOf(it *timeline.Item) string {
	if it.Lut == nil {
		return ""
	}

	return it.Lut.Asset
}

func applyCompositionCreate(c *Context, a *compositionCreateArgs) error {
	if err := timelineIDForm("args.timeline_id", a.TimelineID); err != nil {
		return err
	}
	if err := idForm("args.id", timeline.PrefixItem, a.ID); err != nil {
		return err
	}
	sel, err := c.selected(a)
	if err != nil {
		return err
	}
	if c.Doc.Template != nil {
		return Errorf(
			CodeInvalidState,
			"A template cannot hold compositions: its items carry slots and colours of the template.",
		).
			WithHint("Make the composition in a timeline, not in a template.").
			WithPath("args.item_ids")
	}

	tid, err := c.newTimelineID(a.TimelineID)
	if err != nil {
		return err
	}
	id, err := c.newID("items", timeline.PrefixItem, a.ID)
	if err != nil {
		return err
	}

	track := sel.track
	if track == "" {
		if track = c.topVisualTrack(); track == "" {
			return Errorf(CodeInvalidState, "The timeline has no visual track for the composition.").
				WithHint("Add a visual track with track.add first.").WithPath("args.item_ids")
		}
	}

	if err := c.overlapsUnselected(sel, track); err != nil {
		return err
	}

	name := a.Name
	if name == "" {
		name = "Composition"
	}

	inner := c.moveToTimeline(sel, tid, name)
	if c.Env.Timelines != nil {
		if n := timeline.CheckNewTimeline(c.Env.Timelines, c.Doc.ID, inner); n != nil {
			return nestingError(n, c.Doc.ID, tid, "args.item_ids")
		}
	}

	it := timeline.NewItem(id, timeline.ItemComposition, track, sel.start, sel.end-sel.start)
	it.Timeline = tid
	c.Doc.Items[id] = it

	if vs := timeline.Validate(inner); len(vs) > 0 {
		return validationBug("composition.create_from_selection", vs)
	}

	c.Spawn(inner)
	c.SetEffective("id", id)
	c.SetEffective("timeline_id", tid)
	c.SetEffective("name", name)
	c.SetEffectiveTime("start", sel.start, "")
	c.SetEffectiveTime("duration", sel.end-sel.start, "")
	c.SetEffective("moved", sel.ids)

	return nil
}

// overlapsUnselected is the E_OVERLAP of precondition 3 of C.79: an item that stays, on the track of the
// composition, inside the range the composition covers.
func (c *Context) overlapsUnselected(sel *selection, track string) *Error {
	moved := map[string]bool{}
	for _, id := range sel.ids {
		moved[id] = true
	}

	for _, id := range c.trackItems(track) {
		it := c.Doc.Items[id]
		if moved[id] || it.Start >= sel.end || itemEnd(&it) <= sel.start {
			continue
		}

		return Errorf(CodeOverlap,
			"The composition would cover frames %d-%d of track %s, where item %s (frames %d-%d) is not selected.",
			sel.start, sel.end-1, track, id, it.Start, itemEnd(&it)-1).
			WithDetail("conflicting_item", id).
			WithHint("Select that item too, or move it to another track.").WithPath("args.item_ids")
	}

	return nil
}

// newTimelineID is the id of the new timeline: the one given or a generated one, which is not that of a timeline
// of the production (R6).
func (c *Context) newTimelineID(supplied string) (string, *Error) {
	id, err := c.NewID(timeline.PrefixProject, supplied)
	if err != nil {
		return "", err
	}

	if id == c.Doc.ID {
		return "", Errorf(CodeIDExists, "Timeline id %s already exists.", id).
			WithHint("Omit timeline_id or use an unused id.").WithPath("args.timeline_id")
	}

	if c.Env.Timelines != nil {
		if _, ok := c.Env.Timelines.Doc(id); ok {
			return "", Errorf(CodeIDExists, "Timeline id %s already exists.", id).
				WithHint("Omit timeline_id or use an unused id.").WithPath("args.timeline_id")
		}
	}

	return id, nil
}

type compositionExpandArgs struct {
	ItemID string `json:"item_id" desc:"A composition item." pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

// expandable checks precondition 2 of C.80: the composition only places its timeline.
func (c *Context) expandable(it *timeline.Item, nested *timeline.Document) *Error {
	state := func(reason, format string, args ...any) *Error {
		return Errorf(CodeInvalidState, format, args...).WithDetail("reason", reason).WithPath("args.item_id")
	}

	if f := timeline.CompositionFeature(it); f != "" {
		return state(
			"composition_group_feature",
			"Item %s has its own %s, so it is drawn as one unit and cannot be expanded.",
			it.ID,
			f,
		).
			WithDetail("feature", f).
			WithHint("Remove it first, or keep the composition.")
	}
	if f := timeline.CompositionAudioFeature(it); f != "" {
		return state(
			"composition_group_feature",
			"Item %s has its own %s, which a timeline of items cannot keep.",
			it.ID,
			f,
		).
			WithDetail("feature", f).
			WithHint("Reset it with item.set_audio first, or keep the composition.")
	}

	total := compositionLength(nested, c.Doc.Settings.FPS)
	if it.SourceIn != 0 || it.Duration < total {
		return state(
			"composition_window",
			"Item %s shows frames %d to %d of a timeline of %d frames; only the whole of it can be expanded.",
			it.ID,
			it.SourceIn,
			it.SourceIn+it.Duration-1,
			total,
		).
			WithHint("Trim and slip the composition to show the whole timeline, or keep it.")
	}

	if !timeline.SameFrame(&c.Doc.Settings, &nested.Settings) {
		return state(
			"composition_geometry",
			"Timeline %s has another frame size or rate than this one, so its pictures would change.",
			it.Timeline,
		).
			WithHint("Keep the composition.")
	}

	return nil
}

// mergeNestedAssets adds the assets and fonts the items of the nested timeline use to this document
// (postcondition 1 of C.80). An asset id that is here with another file is a conflict.
func (c *Context) mergeNestedAssets(nested *timeline.Document) *Error {
	for _, id := range slices.Sorted(maps.Keys(nested.Items)) {
		it := nested.Items[id]
		for _, ref := range []string{it.Asset, lutOf(&it)} {
			a, ok := nested.Assets[ref]
			if !ok {
				continue
			}
			if have, ok := c.Doc.Assets[ref]; ok {
				if have.Hash != a.Hash {
					return Errorf(CodeInvalidState, "Asset %s is a different file in timeline %s.", ref, nested.ID).
						WithDetail("reason", "composition_asset_clash").
						WithHint("Keep the composition.").WithPath("args.item_id")
				}
				continue
			}
			if err := c.limit("assets", len(c.Doc.Assets), timeline.MaxAssets); err != nil {
				return err
			}
			c.Doc.Assets[ref] = a
		}
	}

	for _, family := range slices.Sorted(maps.Keys(nested.Fonts)) {
		if _, ok := c.Doc.Fonts[family]; ok {
			continue
		}
		if c.Doc.Fonts == nil {
			c.Doc.Fonts = map[string]timeline.ProjectFont{}
		}
		c.Doc.Fonts[family] = nested.Fonts[family]
	}

	return nil
}

// expandTracks adds the tracks of the nested timeline with new ids: the visual ones directly above the track of
// the composition, in their order, the audio ones after the last track (postcondition 1 of C.80). It returns the
// map from the nested track id to the new one, and the new ids.
func (c *Context) expandTracks(nested *timeline.Document, under timeline.Track) (map[string]string, []string, *Error) {
	used := map[string]bool{}
	for id := range nested.Items {
		used[nested.Items[id].Track] = true
	}

	if n := len(c.Doc.Tracks) + len(used); n > timeline.MaxTracks {
		return nil, nil, Errorf(
			CodeLimit,
			"Expanding would give the document %d tracks; the limit is %d.",
			n,
			timeline.MaxTracks,
		).
			WithHint("Delete unused tracks first.")
	}

	byOrder := func(kind timeline.TrackKind) []string {
		var ids []string
		for id := range nested.Tracks {
			if used[id] && nested.Tracks[id].Kind == kind {
				ids = append(ids, id)
			}
		}
		slices.SortFunc(
			ids,
			func(a, b string) int { return strings.Compare(nested.Tracks[a].Order, nested.Tracks[b].Order) },
		)

		return ids
	}

	above, last := "", ""
	for id := range c.Doc.Tracks {
		tr := c.Doc.Tracks[id]
		if tr.Kind == timeline.TrackVisual && tr.Order < under.Order && tr.Order > above {
			above = tr.Order
		}
		last = max(last, tr.Order)
	}

	mapped := map[string]string{}
	var added []string

	add := func(old, order string) *Error {
		nid, err := c.newID("tracks", timeline.PrefixTrack, "")
		if err != nil {
			return err
		}
		tr := nested.Tracks[old]
		tr.ID, tr.Order = nid, order
		c.Doc.Tracks[nid] = tr
		mapped[old] = nid
		added = append(added, nid)

		return nil
	}

	prev := above
	for _, old := range byOrder(timeline.TrackVisual) {
		order := timeline.OrderBetween(prev, under.Order)
		if err := add(old, order); err != nil {
			return nil, nil, err
		}
		prev = order
	}

	prev = last
	for _, old := range byOrder(timeline.TrackAudio) {
		order := timeline.OrderBetween(prev, "")
		if err := add(old, order); err != nil {
			return nil, nil, err
		}
		prev = order
	}

	return mapped, added, nil
}

func applyCompositionExpand(c *Context, a *compositionExpandArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if it.Type != timeline.ItemComposition {
		return Errorf(
			CodeTypeMismatch,
			"Item %s is %s item; composition.expand needs a composition.",
			a.ItemID,
			article(it.Type),
		).
			WithHint("Use it on a composition item.").
			WithPath("args.item_id")
	}
	nested, nerr := c.nestedDoc(it.Timeline, "args.item_id")
	if nerr != nil {
		return nerr
	}
	if err := c.expandable(&it, nested); err != nil {
		return err
	}

	for _, id := range slices.Sorted(maps.Keys(nested.Items)) {
		if _, ok := c.Doc.Items[id]; ok && id != a.ItemID {
			return Errorf(CodeIDExists, "Item id %s of the nested timeline already exists here.", id).
				WithHint("Rename the clash first.").WithPath("args.item_id")
		}
	}
	if n := len(c.Doc.Items) - 1 + len(nested.Items); n > timeline.MaxItems {
		return Errorf(CodeLimit, "Expanding would give the document %d items; the limit is %d.", n, timeline.MaxItems).
			WithHint("Delete unused items first.")
	}

	under := c.Doc.Tracks[it.Track]
	mapped, tracks, terr := c.expandTracks(nested, under)
	if terr != nil {
		return terr
	}
	if err := c.mergeNestedAssets(nested); err != nil {
		return err
	}

	delete(c.Doc.Items, a.ItemID)

	ids := slices.Sorted(maps.Keys(nested.Items))
	for _, id := range ids {
		moved := nested.Items[id]
		moved.Track = mapped[moved.Track]
		moved.Start += it.Start
		c.Doc.Items[id] = moved
	}

	if vs := timeline.Validate(c.Doc); len(vs) > 0 {
		return validationBug("composition.expand", vs)
	}

	c.SetEffective("item_id", a.ItemID)
	c.SetEffective("timeline_id", it.Timeline)
	c.SetEffective("tracks", tracks)
	c.SetEffective("items", ids)

	return nil
}
