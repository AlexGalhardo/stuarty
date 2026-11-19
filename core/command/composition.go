package command

import (
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "clip.add_composition",
		Summary: "Uses a timeline as a clip.",
		Description: "Adds a composition: the timeline named by timeline_id, used as a clip (RF-083). Nothing is copied, so " +
			"changing that timeline changes every composition that uses it. The default duration is the rest of the nested " +
			"timeline from source_in. A timeline cannot contain itself at any depth, and compositions nest at most 4 deep.",
		Example: `{"track_id": "trk_f6g7h8j9k0", "timeline_id": "prj_a1b2c3d4e5", "start": "0s", "id": "itm_m1n2p3q4r5"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeLimit, CodeIDExists,
			CodeOutOfSource, CodeOverlap,
		},
		Scope: ScopeEdit,
	}, applyClipAddComposition))
}

type clipAddCompositionArgs struct {
	TrackID    string `json:"track_id"            desc:"A visual track."                                                                           pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	TimelineID string `json:"timeline_id"         desc:"Another timeline of the same project."                                                     pattern:"^prj_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Start      Time   `json:"start,omitempty"     desc:"Absolute position; omitted, the end of the last item of the track (0s on an empty track)."`
	SourceIn   Time   `json:"source_in,omitempty" desc:"Where in the nested timeline to start, as a length from its start; default 0s."`
	Duration   Time   `json:"duration,omitempty"  desc:"A length; default the rest of the nested timeline from source_in."`
	Fit        string `json:"fit,omitempty"                                                                                                                                                               enum:"cover,contain,none"`
	ID         string `json:"id,omitempty"                                                                                                         pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

// Reasons of the errors of the nesting rules (SDD-001 section 3.5.1), in details.reason.
const (
	reasonCycle = string(timeline.NestingCycle)
	reasonDepth = string(timeline.NestingDepth)
)

func timelineNotFound(id, path string) *Error {
	e := Errorf(CodeNotFound, "Timeline %s is not in this production.", id).
		WithHint("Use the id of a timeline listed by project_get.").
		WithDetail("kind", "timeline").WithDetail("id", id)
	if path != "" {
		return e.WithPath(path)
	}
	return e
}

// timelineIDForm checks that a supplied timeline identifier has the form of its prefix.
func timelineIDForm(path, id string) *Error {
	if id == "" || timeline.CompositionIDOK(id) {
		return nil
	}
	return Errorf(
		CodeValidation,
		"%s %q is not a timeline id: an id is \"prj_\" followed by 10 characters from 0-9 and a-z "+
			"without i, l, o and u.",
		path,
		id,
	).
		WithHint("Use an id like \"prj_a1b2c3d4e5\".").
		WithPath(path)
}

// nestedDoc is the document of a timeline of the production, or the E_NOT_FOUND of precondition 2.
func (c *Context) nestedDoc(id, path string) (*timeline.Document, *Error) {
	if c.Env.Timelines != nil {
		if d, ok := c.Env.Timelines.Doc(id); ok {
			return d, nil
		}
	}
	return nil, timelineNotFound(id, path)
}

// nestingError turns a broken nesting rule into its error (SDD-001 section 3.5.1).
func nestingError(n *timeline.Nesting, host, child, path string) *Error {
	if n.Kind == timeline.NestingCycle {
		e := Errorf(CodeInvalidState, "Timeline %s holds this timeline, so placing it here would make a loop.", child)
		if child == host {
			e = Errorf(CodeInvalidState, "A timeline cannot contain itself.")
		}
		return e.WithHint("Use a timeline that does not contain this one.").
			WithDetail("reason", reasonCycle).WithDetail("path", n.Path).WithPath(path)
	}
	return Errorf(CodeLimit, "Placing this timeline would nest compositions %d deep; the limit is %d.",
		n.Depth, timeline.MaxCompositionDepth).
		WithHint("Flatten a level with composition.expand.").
		WithDetail("reason", reasonDepth).WithDetail("depth", n.Depth).
		WithDetail("limit", timeline.MaxCompositionDepth).WithPath(path)
}

// compositionLength is the length of a timeline in frames of the host: its end frame, converted with the
// frame rate of the host and rounded up (SDD-001 section 3.5.1), so that every frame of it can be shown.
func compositionLength(nested *timeline.Document, host timeline.Rational) int {
	end := 0
	for id := range nested.Items {
		it := nested.Items[id]
		end = max(end, it.Start+it.Duration)
	}
	n := nested.Settings.FPS
	if end == 0 || n.Num < 1 || n.Den < 1 || host.Num < 1 || host.Den < 1 {
		return end
	}
	return ceilDiv(end*host.Num*n.Den, host.Den*n.Num)
}

func applyClipAddComposition(c *Context, a *clipAddCompositionArgs) error {
	if err := idForm("args.track_id", timeline.PrefixTrack, a.TrackID); err != nil {
		return err
	}
	if err := timelineIDForm("args.timeline_id", a.TimelineID); err != nil {
		return err
	}
	if err := idForm("args.id", timeline.PrefixItem, a.ID); err != nil {
		return err
	}
	start, err := c.startOrTrackEnd(a.TrackID, a.Start)
	if err != nil {
		return err
	}
	if start >= timeline.MaxFrames {
		return Errorf(
			CodeInvalidTime,
			"args.start \"%s\" is at or after the timeline limit of %d frames.",
			a.Start,
			timeline.MaxFrames,
		).
			WithHint("Use a start before \"%df\".", timeline.MaxFrames).
			WithPath("args.start")
	}
	sourceIn := 0
	if a.SourceIn != "" {
		if sourceIn, err = c.length("args.source_in", string(a.SourceIn), true); err != nil {
			return err
		}
	}
	duration := 0
	if a.Duration != "" {
		if duration, err = c.length("args.duration", string(a.Duration), false); err != nil {
			return err
		}
	}

	track, err := c.track(a.TrackID, "args.track_id")
	if err != nil {
		return err
	}
	if track.Kind != timeline.TrackVisual {
		return Errorf(CodeTypeMismatch, "Track %s is an audio track; a composition goes on a visual track.", a.TrackID).
			WithHint("Use a visual track.").WithPath("args.track_id")
	}
	if a.TimelineID == c.Doc.ID {
		return nestingError(&timeline.Nesting{Kind: timeline.NestingCycle, Path: []string{a.TimelineID}},
			c.Doc.ID, a.TimelineID, "args.timeline_id")
	}
	nested, nerr := c.nestedDoc(a.TimelineID, "args.timeline_id")
	if nerr != nil {
		return nerr
	}
	if n := timeline.CheckPlacement(c.Env.Timelines, c.Doc.ID, a.TimelineID); n != nil {
		return nestingError(n, c.Doc.ID, a.TimelineID, "args.timeline_id")
	}
	total := compositionLength(nested, c.Doc.Settings.FPS)
	if total < 1 {
		return Errorf(CodeInvalidState, "Timeline %s is empty, so there is nothing to show.", a.TimelineID).
			WithHint("Add items to that timeline first.").WithPath("args.timeline_id")
	}

	id, err := c.newID("items", timeline.PrefixItem, a.ID)
	if err != nil {
		return err
	}
	if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
		return err
	}

	if sourceIn >= total {
		return Errorf(
			CodeOutOfSource,
			"The source_in %d is at or after the end of timeline %s, which has %d frames.",
			sourceIn,
			a.TimelineID,
			total,
		).
			WithHint("Use a source_in before \"%df\".", total).
			WithDetail("available", map[string]any{"timeline_duration": total, "source_in": sourceIn, "max_duration": 0}).
			WithPath("args.source_in")
	}
	byDefault := a.Duration == ""
	if byDefault {
		duration = min(total-sourceIn, timeline.MaxFrames-start)
	} else if sourceIn+duration > total {
		maxDur := total - sourceIn
		return Errorf(
			CodeOutOfSource,
			"The composition would show frames %d to %d of timeline %s, which has %d frames.",
			sourceIn,
			sourceIn+duration-1,
			a.TimelineID,
			total,
		).
			WithHint("Use a duration of at most \"%df\" (%s), the most this timeline allows from source_in %d.",
				maxDur, c.seconds(maxDur), sourceIn).
			WithDetail("available", map[string]any{"timeline_duration": total, "source_in": sourceIn, "max_duration": maxDur}).
			WithPath("args.duration")
	}

	it := timeline.NewItem(id, timeline.ItemComposition, a.TrackID, start, duration)
	it.Timeline, it.SourceIn = a.TimelineID, sourceIn
	if a.Fit != "" {
		it.Fit = a.Fit
	}
	if err := c.checkEnd("args.duration", start, it.Duration); err != nil {
		return err
	}
	if err := c.checkOverlapAppend(id, a.TrackID, start, it.Duration); err != nil {
		return err
	}
	c.Doc.Items[id] = it
	c.SetEffective("id", id)
	c.SetEffectiveTime("start", start, string(a.Start))
	c.SetEffectiveTime("source_in", it.SourceIn, string(a.SourceIn))
	c.SetEffectiveTime("duration", it.Duration, string(a.Duration))
	return nil
}
