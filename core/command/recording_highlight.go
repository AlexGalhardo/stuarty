package command

import (
	"math"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "recording.set_click_highlight",
		Summary: "Highlights the clicks of a screen take.",
		Description: "Marks each click of the recording of the clip's asset with a short effect, a shape item on a Click " +
			"highlight track above the clip: style ring (a ring that fades), wave (a ring that grows while it fades) " +
			"or pulse (a disc that shrinks while it fades); none removes the highlights. Each lasts duration_ms and is " +
			"cut short by the next click. It is placed through the zoom and position the clip has when the command " +
			"runs, so run it after recording.auto_zoom and again after changing the zoom or moving the clip; each run " +
			"replaces the highlights of the clip. The clicks come from the pointer events of the recording and are " +
			"supplied by the service in resolved. The same clicks and options always give the same items.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "style": "wave", "color": "#33CCFF"}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeLimit,
		},
		Scope: ScopeEdit,
	}, applySetClickHighlight))
}

type setClickHighlightArgs struct {
	ItemID     string           `json:"item_id"               desc:"A clip of a screen recording."                                                           pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Style      string           `json:"style,omitempty"       desc:"ring (default), wave, pulse, or none to remove the highlights."                                                                                 enum:"ring,wave,pulse,none"`
	Color      string           `json:"color,omitempty"       desc:"A colour; default #FFC400."`
	Size       float64          `json:"size,omitempty"        desc:"Pixels of the frame, 16 to 400. Default a twelfth of the frame height."                                                                                                     min:"16"  max:"400"`
	DurationMS int              `json:"duration_ms,omitempty" desc:"How long one highlight lasts, 200 to 2000 ms. Default 600."                                                                                                                 min:"200" max:"2000"`
	Resolved   *ResolvedPointer `json:"resolved,omitempty"    desc:"Filled by the service from the pointer events of the recording; never sent by a caller."                                                                                                         internal:"true"`
}

const (
	defaultHighlightMS    = 600
	defaultHighlightColor = "#FFC400"
)

func applySetClickHighlight(c *Context, a *setClickHighlightArgs) error {
	t, err := c.takeClip(a.ItemID, "The click highlight")
	if err != nil {
		return err
	}

	size := a.Size
	if size == 0 {
		size = frameShare(c, 12, 16)
	}

	durMS := defaultHighlightMS
	if a.DurationMS != 0 {
		durMS = a.DurationMS
	}

	for _, f := range []struct {
		path   string
		v      float64
		lo, hi float64
	}{{"args.size", size, 16, 400}, {"args.duration_ms", float64(durMS), 200, 2000}} {
		if f.v < f.lo || f.v > f.hi {
			return invalidArg(f.path, "%s is %s, outside %s to %s.", f.path, trimFloat(f.v), trimFloat(f.lo),
				trimFloat(f.hi)).WithHint("Use a value inside the range.")
		}
	}

	color, cerr := c.optionalColour("args.color", a.Color)
	if cerr != nil {
		return cerr
	}

	if color == "" {
		color = defaultHighlightColor
	}

	c.removeOverlays(func(name string) bool { return overlayName(name, clickItemName, a.ItemID, true) })

	if a.Style == "none" {
		c.SetEffective("highlights", 0)

		return nil
	}

	track, perr := pointerTrack(a.Resolved)
	if perr != nil {
		return perr
	}

	starts, clicks := highlightClicks(&t, &track)
	if len(starts) == 0 {
		return noPointerData(
			a.ItemID,
			"no click",
			"Use a take recorded with pointer events and a clip that plays a part with clicks.",
		)
	}

	trackID, terr := c.overlayTrack(&t, HighlightTrackName, t.it.Start, t.it.Duration)
	if terr != nil {
		return terr
	}

	fps := c.FPS()
	frames := max(1, int(math.Round(float64(int64(durMS)*fps.Num)/float64(1000*fps.Den))))

	for i, at := range starts {
		if lerr := c.limit("items", len(c.Doc.Items), timeline.MaxItems); lerr != nil {
			return lerr
		}

		dur := min(frames, t.it.Duration-at)
		if i+1 < len(starts) {
			dur = min(dur, starts[i+1]-at)
		}

		it, ierr := c.newShape(
			trackID,
			clickItemName+a.ItemID+" "+strconv.Itoa(i+1),
			"ellipse",
			t.it.Start+at,
			dur,
			round3(size),
		)
		if ierr != nil {
			return ierr
		}

		scale, cx, cy := t.transformAt(c, at)
		ux, uy := t.srcMilli(int64(clicks[i].X)*1000+500, int64(clicks[i].Y)*1000+500, track.Width, track.Height)
		x, y := t.view.ProjectMilli(ux, uy, scale, cx, cy)
		px, py := round3(float64(x)/1000), round3(float64(y)/1000)
		it.X, it.Y = &px, &py

		ring := func() {
			it.Fill, it.StrokeColor, it.StrokeWidth = clearFill, color, math.Max(2, math.Round(size/10))
		}

		ease := func(from, to float64) []timeline.Keyframe {
			first := timeline.NewKeyframe(0, from)
			first.Easing = "ease_out"

			return []timeline.Keyframe{first, timeline.NewKeyframe(dur, to)}
		}

		switch a.Style {
		case "wave":
			ring()
			it.Keyframes = timeline.Keyframes{"scale": ease(0.4, 1.6), "opacity": ease(1, 0)}
		case "pulse":
			it.Fill, it.StrokeWidth = color, 0
			it.Keyframes = timeline.Keyframes{"scale": ease(1.2, 0.4), "opacity": ease(0.6, 0)}
		default:
			ring()
			it.Keyframes = timeline.Keyframes{"opacity": ease(1, 0)}
		}

		c.Doc.Items[it.ID] = it
	}

	c.SetEffective("highlights", len(starts))
	c.SetEffective("track_id", trackID)
	c.SetEffective("size", round3(size))

	return nil
}

// highlightClicks are the presses the highlights mark, in time order, each with the frame of the item it falls on;
// a press in the frame of the one before, or on the last frame of the clip, adds none.
func highlightClicks(t *takeClip, track *recording.PointerTrack) (starts []int, clicks []recording.PointerEvent) {
	for _, e := range track.Events {
		if e.Kind != recording.EventDown || e.TMS < t.fromMS || e.TMS >= t.toMS {
			continue
		}

		f := t.frame(e.TMS)
		if f >= t.it.Duration || (len(starts) > 0 && f <= starts[len(starts)-1]) {
			continue
		}

		starts, clicks = append(starts, f), append(clicks, e)
	}

	return starts, clicks
}
