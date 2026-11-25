package command

import (
	"maps"
	"math"

	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "recording.auto_zoom",
		Summary: "Zooms a screen take on its clicks.",
		Description: "Plans zooms from the clicks of the recording of the clip's asset and writes them as keyframes of x, y " +
			"and scale of the clip, in one undoable step; any x, y and scale keyframes the clip already has are replaced. " +
			"Clicks close in time and in space make one zoom, the view eases in before the first click, is held after " +
			"the last one and eases out; zooms that would rest for under a second between them become a pan. The level is " +
			"at most 3x and a zoom is held at least 0.5 s, so the defaults are gentle. The picture never leaves the " +
			"frame. The clicks come from the pointer events of the recording and are supplied by the service in " +
			"resolved; the command never starts a capture. The same clicks and options always give the same keyframes. " +
			"The clip must play at a constant speed, forward, and be neither cropped, rotated nor flipped. " +
			"Run recording.set_cursor and recording.set_click_highlight again after changing the zoom by hand.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "level": 1.8}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState,
		},
		Scope: ScopeEdit,
	}, applyAutoZoom))
}

type autoZoomArgs struct {
	ItemID   string           `json:"item_id"             desc:"A clip of a screen recording."                                                           pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Level    float64          `json:"level,omitempty"     desc:"How far the view zooms in, 1.1 to 3. Default 1.6."                                                                                              min:"1.1" max:"3"`
	LeadMS   *int             `json:"lead_ms,omitempty"   desc:"The full zoom is reached this long before the first click, 0 to 3000 ms. Default 400."                                                          min:"0"   max:"3000"`
	RampMS   int              `json:"ramp_ms,omitempty"   desc:"The length of the ease in and of the ease out, 200 to 3000 ms. Default 700."                                                                    min:"200" max:"3000"`
	HoldMS   int              `json:"hold_ms,omitempty"   desc:"The least time a zoom is held after its last click, 500 to 10000 ms. Default 1200."                                                             min:"500" max:"10000"`
	MergeMS  *int             `json:"merge_ms,omitempty"  desc:"Clicks closer than this, in one window, make one zoom, 0 to 10000 ms. Default 2500."                                                            min:"0"   max:"10000"`
	MaxZooms int              `json:"max_zooms,omitempty" desc:"The most zooms made, the first in time, 1 to 24. Default 24."                                                                                   min:"1"   max:"24"`
	Resolved *ResolvedPointer `json:"resolved,omitempty"  desc:"Filled by the service from the pointer events of the recording; never sent by a caller."                                                                              internal:"true"`
}

func (a *autoZoomArgs) options() recording.ZoomOptions {
	o := recording.DefaultZoomOptions()

	if a.Level != 0 {
		o.Level = int(math.Round(a.Level * 1000))
	}

	if a.LeadMS != nil {
		o.LeadMS = *a.LeadMS
	}

	if a.MergeMS != nil {
		o.MergeMS = *a.MergeMS
	}

	for _, f := range []struct {
		dst *int
		v   int
	}{{&o.RampMS, a.RampMS}, {&o.HoldMS, a.HoldMS}, {&o.MaxZooms, a.MaxZooms}} {
		if f.v != 0 {
			*f.dst = f.v
		}
	}

	return o
}

func applyAutoZoom(c *Context, a *autoZoomArgs) error {
	t, err := c.takeClip(a.ItemID, "Automatic zoom")
	if err != nil {
		return err
	}

	opts := a.options()
	if oerr := opts.Validate(); oerr != nil {
		return Errorf(CodeValidation, "%s.", oerr.Error()).WithPath("args")
	}

	track, perr := pointerTrack(a.Resolved)
	if perr != nil {
		return perr
	}

	vw, vh := t.view.Visible()
	zooms, dropped := recording.PlanZooms(track.Clicks(), opts, vw, vh, t.fromMS, t.toMS)

	if len(zooms) == 0 {
		return noPointerData(
			a.ItemID,
			"no click",
			"Use a take recorded with pointer events and a clip that plays a part with clicks.",
		)
	}

	s := &c.Doc.Settings
	centre := func(x, y *float64) (int64, int64) {
		cx, cy := int64(s.Width)*500, int64(s.Height)*500
		if x != nil {
			cx = int64(math.Round(*x * 1000))
		}

		if y != nil {
			cy = int64(math.Round(*y * 1000))
		}

		return cx, cy
	}

	restX, restY := centre(t.it.X, t.it.Y)
	restScale := int64(math.Round(t.it.Scale * 1000))
	zoomedScale := min(restScale*int64(opts.Level)/recording.ScaleOne, maxScale*recording.ScaleOne)

	type state struct{ scale, x, y int64 }

	rest := state{restScale, restX, restY}
	kfs := map[string][]timeline.Keyframe{"scale": nil, "x": nil, "y": nil}

	// add writes the state at a frame. A frame the previous keyframe already holds takes the later state, so a
	// ramp that rounds to nothing still ends where it was meant to.
	add := func(ms int64, st state, easing string) {
		f := t.frame(ms)

		for prop, v := range map[string]int64{"scale": st.scale, "x": st.x, "y": st.y} {
			list := kfs[prop]
			k := timeline.NewKeyframe(f, round3(float64(v)/1000))
			k.Easing = easing

			switch n := len(list); {
			case n > 0 && f < list[n-1].T:
				continue
			case n > 0 && f == list[n-1].T:
				list[n-1] = k
			default:
				list = append(list, k)
			}

			kfs[prop] = list
		}
	}

	for _, z := range zooms {
		zx, zy := t.view.Centre(z.FX, z.FY, zoomedScale)
		zoomed := state{zoomedScale, zx, zy}

		if !z.FromPrev {
			add(z.StartMS, rest, "ease_in_out")
		}

		add(z.FullMS, zoomed, "linear")
		add(z.ReleaseMS, zoomed, "ease_in_out")

		if !z.ToNext {
			add(z.EndMS, rest, "linear")
		}
	}

	ks := cloneKeyframes(t.it.Keyframes)
	maps.Copy(ks, kfs)

	it := t.it
	it.Keyframes = ks
	c.Doc.Items[a.ItemID] = it

	c.SetEffective("zooms", len(zooms))
	c.SetEffective("dropped", dropped)
	c.SetEffective("level", float64(opts.Level)/1000)
	c.SetEffective("keyframes", len(kfs["scale"]))

	return nil
}
