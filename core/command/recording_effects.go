package command

import (
	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The commands that finish a screen take (SDD-002 C.93 to C.95, SDD-025 section 17) share what this file holds:
// the clip they work on, how it lies on the frame, and the pointer data the service resolves for them.

// takeClip is a clip of a screen take as the effects see it.
type takeClip struct {
	it    timeline.Item
	asset timeline.Asset
	view  recording.View
	fps   struct{ num, den int64 }
	// fromMS and toMS are the source range the clip plays, in milliseconds of the asset.
	fromMS, toMS int64
}

// frameMS is a count of project frames in milliseconds.
func (t *takeClip) frameMS(frames int64) int64 { return frames * 1000 * t.fps.den / t.fps.num }

// frame is the frame of the item, from its start, at which the clip plays the instant ms of its asset.
func (t *takeClip) frame(ms int64) int { return itemFrame(&t.it, ms, t.fps.num, t.fps.den) }

// takeClip finds the clip and checks it can be finished: a clip of a video asset with a size, played forward at a
// constant speed, with nothing that would move the picture off the geometry the effects assume.
func (c *Context) takeClip(id, what string) (takeClip, *Error) {
	var t takeClip

	it, err := c.clipItem(id, what)
	if err != nil {
		return t, err
	}

	asset, ok := c.Doc.Assets[it.Asset]
	if !ok {
		return t, Errorf(CodeNotFound, "Item %s has no asset in the project.", id).
			WithHint("Use a clip of a screen recording.").WithPath("args.item_id")
	}

	if asset.Kind != timeline.AssetVideo || asset.Width < 1 || asset.Height < 1 {
		return t, Errorf(CodeTypeMismatch, "Asset %s is not a video with a size.", asset.ID).
			WithHint("Use a clip of a screen recording.").WithPath("args.item_id")
	}

	switch {
	case it.Freeze || it.Reverse || len(it.SpeedCurve) > 0:
		return t, Errorf(CodeInvalidState, "Item %s is a freeze frame, plays backwards or has a speed curve.", id).
			WithHint("%s needs a clip that plays forward at a constant speed.", what).WithPath("args.item_id")
	case it.Crop != nil || it.Rotation != 0 || it.FlipX || it.FlipY:
		return t, Errorf(CodeInvalidState, "Item %s is cropped, rotated or flipped.", id).
			WithHint("Remove the crop, rotation and flip first (clip.set_framing, item.set_transform, clip.set_flip); " +
				"the effects of a screen take assume the picture is not moved.").
			WithPath("args.item_id")
	}

	fps := c.FPS()
	t.it, t.asset, t.fps.num, t.fps.den = it, asset, fps.Num, fps.Den
	t.fromMS = t.frameMS(int64(it.SourceIn))
	t.toMS = t.frameMS(int64(it.SourceIn + it.Duration*it.Speed.Num/it.Speed.Den))

	s := &c.Doc.Settings
	t.view = recording.View{FrameW: s.Width, FrameH: s.Height, SrcW: asset.Width, SrcH: asset.Height, Fit: it.Fit}

	return t, nil
}

// ResolvedPointerEvent is one pointer event of ResolvedPointer: the lines of SDD-025 section 16.6 with the time in
// milliseconds of the axis of the asset (the arguments of a command carry no 64-bit integer).
type ResolvedPointerEvent struct {
	TMS    int    `json:"t_ms"             desc:"Milliseconds on the time axis of the asset."   min:"0"`
	Kind   string `json:"kind"             desc:"move, down or up."                                     enum:"move,down,up"`
	Button string `json:"button,omitempty" desc:"left, right or middle, for down and up."               enum:"left,right,middle"`
	X      int    `json:"x"                desc:"Pixels of the captured source, from the left." min:"0"`
	Y      int    `json:"y"                desc:"Pixels of the captured source, from the top."  min:"0"`
}

// ResolvedPointer is the pointer data the service puts in `resolved` (SDD-002 A.9, SDD-025 section 17.2); its
// JSON is that of recording.PointerTrack.
type ResolvedPointer struct {
	Width  int                    `json:"width"  desc:"Width in pixels of the captured source."`
	Height int                    `json:"height" desc:"Height in pixels of the captured source."`
	Events []ResolvedPointerEvent `json:"events" desc:"The pointer events in the source range of the clip, in time order."`
}

func (r *ResolvedPointer) track() recording.PointerTrack {
	t := recording.PointerTrack{Width: r.Width, Height: r.Height, Events: make([]recording.PointerEvent, len(r.Events))}

	for i, e := range r.Events {
		t.Events[i] = recording.PointerEvent{
			TMS: int64(e.TMS), Kind: recording.EventKind(e.Kind), Button: e.Button, X: e.X, Y: e.Y,
		}
	}

	return t
}

// pointerTrack checks the pointer data the service resolved and returns it.
func pointerTrack(r *ResolvedPointer) (recording.PointerTrack, *Error) {
	if r == nil {
		return recording.PointerTrack{}, Errorf(CodeValidation,
			"args.resolved is missing: the pointer events of the recording are not known.").
			WithHint("The service fills it from the recording of the clip's asset; a recording made without " +
				"pointer events cannot be finished this way.").WithPath("args.resolved")
	}

	t := r.track()
	if err := t.Validate(); err != nil {
		return t, Errorf(CodeValidation, "args.resolved is not valid: %s.", err.Error()).
			WithHint("Do not send resolved: the service fills it.").WithPath("args.resolved")
	}

	return t, nil
}

// noPointerData is the answer for a take that has nothing in the range the clip plays.
func noPointerData(id, what, hint string) *Error {
	return Errorf(CodeInvalidState, "Item %s has %s in the part of the recording it plays.", id, what).
		WithHint("%s", hint).WithPath("args.item_id")
}
