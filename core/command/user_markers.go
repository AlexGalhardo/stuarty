package command

import (
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "marker.add",
		Summary: "Puts a marker on the timeline, or on one item.",
		Description: "Adds a marker at a frame of the timeline, with an optional label of up to 60 characters. With " +
			"item_id the marker belongs to that item (at must fall inside it) and moves with it, like the beat " +
			"markers of audio.mark_beats. At most 200 markers on the timeline and on each item; one marker per " +
			"frame. Markers have no effect on the picture or the sound.",
		Example: `{"at": "12.5s", "label": "drop"}`,
		Errors:  []string{CodeValidation, CodeInvalidTime, CodeInvalidState, CodeLimit},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyMarkerAdd))

	Register(Define(Spec{
		Name:    "marker.set",
		Summary: "Moves or relabels a marker of the timeline or of an item.",
		Description: "Finds the marker at a frame and changes its frame (to) and or its label; a label of \"\" removes " +
			"the label. With item_id it works on the markers of that item, and at and to fall inside it.",
		Example: `{"at": "12.5s", "to": "13s", "label": "drop"}`,
		Errors:  []string{CodeValidation, CodeInvalidTime, CodeNotFound, CodeInvalidState},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyMarkerSet))

	Register(Define(Spec{
		Name:    "marker.remove",
		Summary: "Removes a marker of the timeline or of an item.",
		Description: "Removes the marker at a frame; all removes every marker. With item_id it works on the markers " +
			"of that item, beat markers included.",
		Example: `{"at": "12.5s"}`,
		Errors:  []string{CodeValidation, CodeInvalidTime, CodeNotFound},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyMarkerRemove))
}

type markerAddArgs struct {
	ItemID string `json:"item_id,omitempty" desc:"An item to mark instead of the timeline; at must then fall inside it." pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	At     Time   `json:"at"                desc:"The position on the timeline."`
	Label  string `json:"label,omitempty"   desc:"Up to 60 characters; default none."                                                                                           max:"60"`
}

type markerSetArgs struct {
	ItemID string  `json:"item_id,omitempty" desc:"The item that holds the marker; at and to fall inside it." pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	At     Time    `json:"at"                desc:"The position of the marker to change."`
	To     Time    `json:"to,omitempty"      desc:"The new position."`
	Label  *string `json:"label,omitempty"   desc:"The new label, up to 60 characters; \"\" removes it."                                                             max:"60"`
}

type markerRemoveArgs struct {
	ItemID string `json:"item_id,omitempty" desc:"The item that holds the marker; all then clears its markers." pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	At     Time   `json:"at,omitempty"      desc:"The position of the marker to remove."`
	All    *bool  `json:"all,omitempty"     desc:"true removes every marker, instead of at."`
}

// markerSpace is where a marker command works: the timeline, in timeline frames, or one item, in
// the frames of the item (frame 0 is its first frame; audio.mark_beats writes the same).
type markerSpace struct {
	c      *Context
	item   *timeline.Item
	origin int
	limit  int
	marks  []timeline.Marker
}

func (c *Context) markerSpace(itemID string) (*markerSpace, *Error) {
	if itemID == "" {
		return &markerSpace{c: c, limit: timeline.MaxFrames, marks: slices.Clone(c.Doc.Markers)}, nil
	}
	if err := idForm("args.item_id", timeline.PrefixItem, itemID); err != nil {
		return nil, err
	}
	it, err := c.item(itemID)
	if err != nil {
		return nil, err
	}
	return &markerSpace{c: c, item: &it, origin: it.Start, limit: it.Duration, marks: slices.Clone(it.Markers)}, nil
}

// frame parses a timeline position and returns it in the frames of the space.
func (s *markerSpace) frame(path string, t Time) (int, *Error) {
	if s.item == nil {
		return s.c.markerFrame(path, t)
	}
	f, err := s.c.ParseTime(path, string(t))
	if err != nil {
		return 0, err
	}
	if f < s.origin || f >= s.origin+s.limit {
		return 0, invalidArg(path, "%s is frame %d, outside item %s (frames %d to %d).",
			path, f, s.item.ID, s.origin, s.origin+s.limit-1).
			WithHint("Use a position inside the item, or leave item_id out to mark the timeline.")
	}
	return f - s.origin, nil
}

func (s *markerSpace) where() string {
	if s.item == nil {
		return "The timeline"
	}
	return "Item " + s.item.ID
}

func (s *markerSpace) notFound(frame int) *Error {
	return Errorf(CodeNotFound, "%s has no marker at frame %d.", s.where(), frame+s.origin).
		WithHint("Read the markers with project_get detail full.").WithPath("args.at")
}

func (s *markerSpace) taken(frame int, path string) *Error {
	return Errorf(CodeInvalidState, "%s already has a marker at frame %d.", s.where(), frame+s.origin).
		WithPath(path)
}

// save stores the markers in order and records how many there are.
func (s *markerSpace) save(marks []timeline.Marker) {
	if s.item == nil {
		s.c.setMarkers(marks)
		return
	}
	slices.SortFunc(marks, func(a, b timeline.Marker) int { return a.Frame - b.Frame })
	s.item.Markers = nilIfEmpty(marks)
	s.c.Doc.Items[s.item.ID] = *s.item
	s.c.SetEffective("markers", len(marks))
}

func (c *Context) markerFrame(path string, t Time) (int, *Error) {
	frame, err := c.ParseTime(path, string(t))
	if err != nil {
		return 0, err
	}
	if frame < 0 || frame >= timeline.MaxFrames {
		return 0, invalidArg(path, "%s is frame %d, outside 0 to %d.", path, frame, timeline.MaxFrames-1).
			WithHint("Use a position inside the timeline limit.")
	}
	return frame, nil
}

func markerIndex(marks []timeline.Marker, frame int) int {
	return slices.IndexFunc(marks, func(m timeline.Marker) bool { return m.Frame == frame })
}

func markerLabelCheck(path, label string) *Error {
	if n := len([]rune(label)); n > 60 {
		return invalidArg(path, "%s has %d characters; the limit is 60.", path, n).WithHint("Shorten the label.")
	}
	return nil
}

func (c *Context) setMarkers(marks []timeline.Marker) {
	slices.SortFunc(marks, func(a, b timeline.Marker) int { return a.Frame - b.Frame })
	c.Doc.Markers = nilIfEmpty(marks)
	c.SetEffective("markers", len(marks))
}

func applyMarkerAdd(c *Context, a *markerAddArgs) error {
	sp, err := c.markerSpace(a.ItemID)
	if err != nil {
		return err
	}
	frame, err := sp.frame("args.at", a.At)
	if err != nil {
		return err
	}
	if err := markerLabelCheck("args.label", a.Label); err != nil {
		return err
	}
	if markerIndex(sp.marks, frame) >= 0 {
		return sp.taken(frame, "args.at").WithHint("Use marker.set to change it.")
	}
	if len(sp.marks) >= timeline.MaxMarkers {
		return Errorf(
			CodeLimit,
			"%s would have %d markers; the limit is %d.",
			sp.where(),
			len(sp.marks)+1,
			timeline.MaxMarkers,
		).
			WithHint("Remove markers with marker.remove first.")
	}
	c.SetEffectiveTime("at", frame+sp.origin, string(a.At))
	sp.save(append(sp.marks, timeline.Marker{Frame: frame, Label: a.Label}))
	return nil
}

func applyMarkerSet(c *Context, a *markerSetArgs) error {
	sp, err := c.markerSpace(a.ItemID)
	if err != nil {
		return err
	}
	at, err := sp.frame("args.at", a.At)
	if err != nil {
		return err
	}
	to := at
	if a.To != "" {
		if to, err = sp.frame("args.to", a.To); err != nil {
			return err
		}
	}
	if a.To == "" && a.Label == nil {
		return invalidArg(
			"args",
			"marker.set needs to or label.",
		).WithHint("Give the new position, the new label, or both.")
	}
	if a.Label != nil {
		if err := markerLabelCheck("args.label", *a.Label); err != nil {
			return err
		}
	}
	i := markerIndex(sp.marks, at)
	if i < 0 {
		return sp.notFound(at)
	}
	if to != at && markerIndex(sp.marks, to) >= 0 {
		return sp.taken(to, "args.to").WithHint("Remove it first, or choose another position.")
	}
	sp.marks[i].Frame = to
	if a.Label != nil {
		sp.marks[i].Label = *a.Label
	}
	c.SetEffectiveTime("at", to+sp.origin, cmpTime(a.To, a.At))
	sp.save(sp.marks)
	return nil
}

func cmpTime(to, at Time) string {
	if to != "" {
		return string(to)
	}
	return string(at)
}

func applyMarkerRemove(c *Context, a *markerRemoveArgs) error {
	all := a.All != nil && *a.All
	if all == (a.At != "") {
		return invalidArg("args", "marker.remove needs exactly one of at and all.").
			WithHint("Give the position of the marker, or all true.")
	}
	sp, err := c.markerSpace(a.ItemID)
	if err != nil {
		return err
	}
	if all {
		sp.save(nil)
		return nil
	}
	at, err := sp.frame("args.at", a.At)
	if err != nil {
		return err
	}
	i := markerIndex(sp.marks, at)
	if i < 0 {
		return sp.notFound(at)
	}
	sp.save(slices.Delete(sp.marks, i, i+1))
	return nil
}
