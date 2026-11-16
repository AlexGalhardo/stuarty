package command

import (
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "marker.add",
		Summary: "Puts a marker on the timeline.",
		Description: "Adds a marker the person or the agent keeps at a frame of the timeline (not of an item), with an " +
			"optional label of up to 60 characters. At most 200 markers; one marker per frame. Markers have no " +
			"effect on the picture or the sound. The beat markers of a music item are separate (audio.mark_beats).",
		Example: `{"at": "12.5s", "label": "drop"}`,
		Errors:  []string{CodeValidation, CodeInvalidTime, CodeInvalidState, CodeLimit},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyMarkerAdd))

	Register(Define(Spec{
		Name:    "marker.set",
		Summary: "Moves or relabels a marker of the timeline.",
		Description: "Finds the marker at a frame and changes its frame (to) and or its label; a label of \"\" removes " +
			"the label.",
		Example: `{"at": "12.5s", "to": "13s", "label": "drop"}`,
		Errors:  []string{CodeValidation, CodeInvalidTime, CodeNotFound, CodeInvalidState},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyMarkerSet))

	Register(Define(Spec{
		Name:        "marker.remove",
		Summary:     "Removes a marker of the timeline.",
		Description: "Removes the marker at a frame; all removes every marker of the timeline.",
		Example:     `{"at": "12.5s"}`,
		Errors:      []string{CodeValidation, CodeInvalidTime, CodeNotFound},
		Scope:       ScopeEdit,
		Phase:       Beta,
	}, applyMarkerRemove))
}

type markerAddArgs struct {
	At    Time   `json:"at"              desc:"The position on the timeline."`
	Label string `json:"label,omitempty" desc:"Up to 60 characters; default none." max:"60"`
}

type markerSetArgs struct {
	At    Time    `json:"at"              desc:"The position of the marker to change."`
	To    Time    `json:"to,omitempty"    desc:"The new position."`
	Label *string `json:"label,omitempty" desc:"The new label, up to 60 characters; \"\" removes it." max:"60"`
}

type markerRemoveArgs struct {
	At  Time  `json:"at,omitempty"  desc:"The position of the marker to remove."`
	All *bool `json:"all,omitempty" desc:"true removes every marker of the timeline, instead of at."`
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
	frame, err := c.markerFrame("args.at", a.At)
	if err != nil {
		return err
	}
	if err := markerLabelCheck("args.label", a.Label); err != nil {
		return err
	}
	marks := slices.Clone(c.Doc.Markers)
	if markerIndex(marks, frame) >= 0 {
		return Errorf(CodeInvalidState, "The timeline already has a marker at frame %d.", frame).
			WithHint("Use marker.set to change it.").WithPath("args.at")
	}
	if len(marks) >= timeline.MaxMarkers {
		return Errorf(
			CodeLimit,
			"The timeline would have %d markers; the limit is %d.",
			len(marks)+1,
			timeline.MaxMarkers,
		).
			WithHint("Remove markers with marker.remove first.")
	}
	c.SetEffectiveTime("at", frame, string(a.At))
	c.setMarkers(append(marks, timeline.Marker{Frame: frame, Label: a.Label}))
	return nil
}

func applyMarkerSet(c *Context, a *markerSetArgs) error {
	at, err := c.markerFrame("args.at", a.At)
	if err != nil {
		return err
	}
	to := at
	if a.To != "" {
		if to, err = c.markerFrame("args.to", a.To); err != nil {
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
	marks := slices.Clone(c.Doc.Markers)
	i := markerIndex(marks, at)
	if i < 0 {
		return Errorf(CodeNotFound, "The timeline has no marker at frame %d.", at).
			WithHint("Read the markers with project_get detail full.").WithPath("args.at")
	}
	if to != at && markerIndex(marks, to) >= 0 {
		return Errorf(CodeInvalidState, "The timeline already has a marker at frame %d.", to).
			WithHint("Remove it first, or choose another position.").WithPath("args.to")
	}
	marks[i].Frame = to
	if a.Label != nil {
		marks[i].Label = *a.Label
	}
	c.SetEffectiveTime("at", to, cmpTime(a.To, a.At))
	c.setMarkers(marks)
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
	if all {
		c.setMarkers(nil)
		return nil
	}
	at, err := c.markerFrame("args.at", a.At)
	if err != nil {
		return err
	}
	marks := slices.Clone(c.Doc.Markers)
	i := markerIndex(marks, at)
	if i < 0 {
		return Errorf(CodeNotFound, "The timeline has no marker at frame %d.", at).
			WithHint("Read the markers with project_get detail full.").WithPath("args.at")
	}
	c.setMarkers(slices.Delete(marks, i, i+1))
	return nil
}
