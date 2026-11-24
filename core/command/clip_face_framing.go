package command

import (
	"github.com/AlexGalhardo/stuarty/core/facetrack"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "clip.face_frame",
		Summary: "Pans the crop to follow the speaker's face.",
		Description: "Sets fit cover and writes keyframes of crop_left and crop_right (and crop_top and crop_bottom when the " +
			"source is taller than the frame) so the visible window follows the face track of the clip's asset, " +
			"smoothed, with a dead zone and a maximum pan speed, over the source range the clip plays. Any crop " +
			"keyframes the clip already has are replaced, in one undoable step. The face track comes from a face_track " +
			"job and is supplied by the service in resolved; the command never starts a job. The same track and options " +
			"always give the same keyframes. The clip must play at a constant speed, forward, and not be a freeze frame.",
		Example: `{"item_id": "itm_m1n2p3q4r5"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState},
		Scope:   ScopeEdit,
	}, applyFaceFraming))
}

// ResolvedFaces is the face track the service puts in `resolved` (SDD-024 section 6).
type ResolvedFaces struct {
	Track facetrack.Track `json:"track" desc:"The samples of the facetrack artifact of the clip's asset, in asset milliseconds."`
}

type faceFramingArgs struct {
	ItemID      string         `json:"item_id"                desc:"A clip of a video asset."                                                                            pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	SmoothingMS int            `json:"smoothing_ms,omitempty" desc:"Time constant of the smoothing, 0 to 5000 ms. Default 400."                                                                                                 min:"0"   max:"5000"`
	DeadZone    int            `json:"dead_zone,omitempty"    desc:"How far the face may drift before the frame follows, in ten-thousandths of the window. Default 800."                                                        min:"0"   max:"5000"`
	MaxPan      int            `json:"max_pan,omitempty"      desc:"Fastest pan, in ten-thousandths of the source width per second. Default 6000."                                                                              min:"100" max:"100000"`
	HoldMS      int            `json:"hold_ms,omitempty"      desc:"How long the last position is held when no face is seen. Default 1500."                                                                                     min:"0"   max:"30000"`
	KeyframeMS  int            `json:"keyframe_ms,omitempty"  desc:"The least time between two keyframes, 50 to 5000 ms. Default 200."                                                                                          min:"50"  max:"5000"`
	Resolved    *ResolvedFaces `json:"resolved,omitempty"     desc:"Filled by the service from the face track of the asset; never sent by a caller."                                                                                                   internal:"true"`
}

func applyFaceFraming(c *Context, a *faceFramingArgs) error {
	it, err := c.clipItem(a.ItemID, "face framing")
	if err != nil {
		return err
	}

	if it.Freeze || it.Reverse || len(it.SpeedCurve) > 0 {
		return Errorf(CodeInvalidState, "Item %s is a freeze frame, plays backwards or has a speed curve.", a.ItemID).
			WithHint("Face framing needs a clip that plays forward at a constant speed.").WithPath("args.item_id")
	}

	opts := facetrack.Options{
		SmoothingMS: a.SmoothingMS, DeadZone: a.DeadZone, MaxPan: a.MaxPan, HoldMS: a.HoldMS, KeyframeMS: a.KeyframeMS,
	}.WithDefaults()
	if oerr := opts.Validate(); oerr != nil {
		return Errorf(CodeValidation, "%s.", oerr.Error()).WithPath("args")
	}

	if a.Resolved == nil {
		return Errorf(CodeValidation, "args.resolved is missing: the face track of the asset is not known.").
			WithHint("The service fills it from the face_track job of the asset; run the job first, then retry.").
			WithPath("args.resolved")
	}

	track := &a.Resolved.Track
	if terr := track.Validate(); terr != nil {
		return Errorf(CodeValidation, "args.resolved.track is not valid: %s.", terr.Error()).WithPath("args.resolved")
	}

	fps := c.FPS()
	frameMS := func(frames int64) int64 { return frames * 1000 * fps.Den / fps.Num }
	srcFrames := it.Duration * it.Speed.Num / it.Speed.Den
	from, to := frameMS(int64(it.SourceIn)), frameMS(int64(it.SourceIn+srcFrames))

	s := &c.Doc.Settings
	vw, vh := facetrack.Visible(track.Source, s.Width, s.Height)

	// A long clip would need more keyframes than a property may hold: the spacing grows with the length,
	// leaving room for the hold points the plan adds before a change.
	opts.KeyframeMS = max(opts.KeyframeMS, int((to-from)*2/(maxKeyframes-10))+1)

	plan := facetrack.Plan(track, opts, vw, vh, from, to)

	ks := cloneKeyframes(it.Keyframes)
	if ks == nil {
		ks = timeline.Keyframes{}
	}

	for _, p := range timeline.CropProps {
		delete(ks, p)
	}

	crop := timeline.Crop{}
	if it.Crop != nil {
		crop = *it.Crop
	}

	type axis struct {
		low, high string
		lowV      *float64
		highV     *float64
		visible   int
		value     func(w facetrack.Window) (int, int)
	}

	for _, ax := range []axis{
		{
			"crop_left", "crop_right", &crop.Left, &crop.Right, vw,
			func(w facetrack.Window) (int, int) { return w.Left, w.Right },
		},
		{
			"crop_top", "crop_bottom", &crop.Top, &crop.Bottom, vh,
			func(w facetrack.Window) (int, int) { return w.Top, w.Bottom },
		},
	} {
		if ax.visible >= facetrack.Unit {
			continue
		}

		low, high := make([]timeline.Keyframe, 0, len(plan)), make([]timeline.Keyframe, 0, len(plan))
		last := -1

		for _, k := range plan {
			f := itemFrame(&it, k.T, fps.Num, fps.Den)
			if f <= last {
				continue
			}

			last = f
			l, h := ax.value(facetrack.WindowAt(vw, vh, k.X, k.Y))
			low = append(low, timeline.NewKeyframe(f, fixed4(l)))
			high = append(high, timeline.NewKeyframe(f, fixed4(h)))
		}

		// The spacing chosen above keeps len(low) under maxKeyframes whatever the length of the clip: at
		// most one keyframe per spacing, and one more before each change (TestApplyFaceFramingKeeps...).
		ks[ax.low], ks[ax.high] = low, high
		*ax.lowV, *ax.highV = low[0].Value, high[0].Value
	}

	it.Fit = "cover"

	it.Keyframes = ks
	if len(ks) == 0 {
		it.Keyframes = nil
	}

	if crop.Left == 0 && crop.Top == 0 && crop.Right == 0 && crop.Bottom == 0 {
		it.Crop = nil
	} else {
		it.Crop = &crop
	}

	c.Doc.Items[a.ItemID] = it
	c.SetEffective("fit", it.Fit)
	c.SetEffective("keyframes", len(plan))
	c.SetEffective("visible_width", float64(vw)/facetrack.Unit)
	c.SetEffective("track_samples", len(track.Samples))

	return nil
}

// itemFrame is the frame of the item, from its start, at which the clip plays the instant ms of its asset,
// kept inside the item. The clip plays at a constant speed (checked before).
func itemFrame(it *timeline.Item, ms, num, den int64) int {
	src := ms * num / (1000 * den)
	rel := (src - int64(it.SourceIn)) * int64(it.Speed.Den) / int64(it.Speed.Num)

	return int(max(0, min(rel, int64(it.Duration))))
}

// fixed4 is a ten-thousandths value as the three-decimal fraction every crop value has.
func fixed4(v int) float64 { return round3(float64(v) / facetrack.Unit) }
