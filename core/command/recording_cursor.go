package command

import (
	"math"

	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "recording.set_cursor",
		Summary: "Draws a smooth cursor on a screen take.",
		Description: "Draws the pointer of the recording of the clip's asset, smoothed, as shape items on a Cursor track " +
			"above the clip: the capture never contains the cursor. The path is the pointer events through a filter of " +
			"time constant smoothing_ms (0 follows the raw path), sampled every keyframe_ms and mapped through the zoom " +
			"and position the clip has when the command runs, so run it after recording.auto_zoom and again after " +
			"changing the zoom or moving the clip; each run replaces the cursor items of the clip. style is dot or " +
			"ring, size is in pixels of the frame and stays the same when the clip is zoomed; visible false removes " +
			"the cursor. The positions come from the pointer events of the recording and are supplied by the service " +
			"in resolved. The same events and options always give the same items.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "style": "ring", "smoothing_ms": 200}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeLimit,
		},
		Scope: ScopeEdit,
	}, applySetCursor))
}

type setCursorArgs struct {
	ItemID      string           `json:"item_id"                desc:"A clip of a screen recording."                                                           pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Visible     *bool            `json:"visible,omitempty"      desc:"false removes the cursor of the clip. Default true."`
	Style       string           `json:"style,omitempty"        desc:"dot (default) or ring."                                                                                                                         enum:"dot,ring"`
	Size        float64          `json:"size,omitempty"         desc:"Pixels of the frame, 8 to 128. Default a fortieth of the frame height, at least 16."                                                                            min:"8"  max:"128"`
	Color       string           `json:"color,omitempty"        desc:"A colour; default #FFFFFF."`
	SmoothingMS *int             `json:"smoothing_ms,omitempty" desc:"Time constant of the smoothing, 0 to 2000 ms; 0 follows the raw path. Default 150."                                                                             min:"0"  max:"2000"`
	KeyframeMS  int              `json:"keyframe_ms,omitempty"  desc:"The time between two keyframes, 50 to 1000 ms. Default 100."                                                                                                    min:"50" max:"1000"`
	Resolved    *ResolvedPointer `json:"resolved,omitempty"     desc:"Filled by the service from the pointer events of the recording; never sent by a caller."                                                                                            internal:"true"`
}

const (
	// cursorSegment is the most keyframes of one cursor item: a property holds 100, and the item shares its last
	// keyframe with the next one.
	cursorSegment = 90

	defaultCursorSmoothing = 150
	defaultCursorStep      = 100
	dotStroke              = "#1F1F1F"
	clearFill              = "#00000000"
)

// frameShare is the size of a default overlay: the height of the frame over divisor, at least least pixels.
func frameShare(c *Context, divisor int, least float64) float64 {
	return math.Max(least, float64(c.Doc.Settings.Height/divisor))
}

type cursorPoint struct {
	frame int
	x, y  float64
}

// cursorOptions are the options of the cursor with their defaults, checked.
type cursorOptions struct {
	smoothing, step int
	size            float64
	color           timeline.Color
	ring            bool
}

func (a *setCursorArgs) options(c *Context) (cursorOptions, *Error) {
	o := cursorOptions{
		smoothing: defaultCursorSmoothing,
		step:      defaultCursorStep,
		size:      a.Size,
		ring:      a.Style == "ring",
	}

	if a.SmoothingMS != nil {
		o.smoothing = *a.SmoothingMS
	}

	if a.KeyframeMS != 0 {
		o.step = a.KeyframeMS
	}

	if o.size == 0 {
		o.size = frameShare(c, 40, 16)
	}

	for _, f := range []struct {
		path   string
		v      float64
		lo, hi float64
	}{
		{"args.smoothing_ms", float64(o.smoothing), 0, 2000},
		{"args.keyframe_ms", float64(o.step), 50, 1000},
		{"args.size", o.size, 8, 128},
	} {
		if f.v < f.lo || f.v > f.hi {
			return o, invalidArg(f.path, "%s is %s, outside %s to %s.", f.path, trimFloat(f.v), trimFloat(f.lo),
				trimFloat(f.hi)).WithHint("Use a value inside the range.")
		}
	}

	color, err := c.optionalColour("args.color", a.Color)
	if err != nil {
		return o, err
	}

	o.color = cmpColour(color, "#FFFFFF")

	return o, nil
}

func cmpColour(v, fallback timeline.Color) timeline.Color {
	if v == "" {
		return fallback
	}

	return v
}

// cursorPath is the smoothed pointer of the part of the take the clip plays, mapped to the frame at every sample
// through the transform the clip has there, one point per frame at most.
func cursorPath(c *Context, t *takeClip, track *recording.PointerTrack, o cursorOptions) []cursorPoint {
	path := recording.SmoothPath(track.Events, t.fromMS, t.toMS, o.step, o.smoothing)
	points := make([]cursorPoint, 0, len(path))

	for _, p := range path {
		f := t.frame(p.TMS)
		scale, cx, cy := t.transformAt(c, f)
		ux, uy := t.srcMilli(p.XM+500, p.YM+500, track.Width, track.Height)
		x, y := t.view.ProjectMilli(ux, uy, scale, cx, cy)
		pt := cursorPoint{f, round3(float64(x) / 1000), round3(float64(y) / 1000)}

		if n := len(points); n > 0 && points[n-1].frame == f {
			points[n-1] = pt
		} else {
			points = append(points, pt)
		}
	}

	return points
}

// cursorItem is one segment of the cursor: a shape item that holds the points, with the keyframes of x and y.
func cursorItem(c *Context, t *takeClip, trackID string, seg []cursorPoint, o cursorOptions) (timeline.Item, *Error) {
	first := seg[0].frame

	it, err := c.newShape(trackID, cursorItemName+t.it.ID, "ellipse", t.it.Start+first, seg[len(seg)-1].frame-first,
		round3(o.size))
	if err != nil {
		return it, err
	}

	if o.ring {
		it.Fill, it.StrokeColor, it.StrokeWidth = clearFill, o.color, math.Max(2, math.Round(o.size/8))
	} else {
		it.Fill, it.StrokeColor, it.StrokeWidth = o.color, dotStroke, math.Max(1, math.Round(o.size/12))
	}

	xs, ys := make([]timeline.Keyframe, len(seg)), make([]timeline.Keyframe, len(seg))
	for i, p := range seg {
		xs[i], ys[i] = timeline.NewKeyframe(p.frame-first, p.x), timeline.NewKeyframe(p.frame-first, p.y)
	}

	x0, y0 := seg[0].x, seg[0].y
	it.X, it.Y = &x0, &y0
	it.Keyframes = timeline.Keyframes{"x": xs, "y": ys}

	return it, nil
}

func applySetCursor(c *Context, a *setCursorArgs) error {
	t, err := c.takeClip(a.ItemID, "The cursor")
	if err != nil {
		return err
	}

	o, oerr := a.options(c)
	if oerr != nil {
		return oerr
	}

	c.removeOverlays(func(name string) bool { return overlayName(name, cursorItemName, a.ItemID, false) })

	if a.Visible != nil && !*a.Visible {
		c.SetEffective("segments", 0)

		return nil
	}

	track, perr := pointerTrack(a.Resolved)
	if perr != nil {
		return perr
	}

	if len(track.Events) == 0 {
		return noPointerData(a.ItemID, "no pointer position", "Use a take recorded with pointer events.")
	}

	points := cursorPath(c, &t, &track, o)
	if len(points) < 2 {
		return noPointerData(a.ItemID, "no time", "Use a clip that plays at least a frame of the take.")
	}

	trackID, terr := c.overlayTrack(&t, CursorTrackName, t.it.Start, t.it.Duration)
	if terr != nil {
		return terr
	}

	segments := 0

	for from := 0; from < len(points)-1; from += cursorSegment - 1 {
		if lerr := c.limit("items", len(c.Doc.Items), timeline.MaxItems); lerr != nil {
			return lerr
		}

		it, ierr := cursorItem(c, &t, trackID, points[from:min(from+cursorSegment, len(points))], o)
		if ierr != nil {
			return ierr
		}

		c.Doc.Items[it.ID] = it
		segments++
	}

	c.SetEffective("segments", segments)
	c.SetEffective("keyframes", len(points))
	c.SetEffective("track_id", trackID)
	c.SetEffective("size", round3(o.size))

	return nil
}
