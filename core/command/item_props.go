package command

import (
	"math"
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:        "item.rename",
		Summary:     "Sets the label of an item.",
		Description: "Sets the display name of the item; an empty name clears it.",
		Example:     `{"item_id": "itm_m1n2p3q4r5", "name": "Intro shot"}`,
		Errors:      []string{CodeValidation, CodeNotFound},
		Scope:       ScopeEdit,
		Phase:       Alpha,
	}, applyItemRename))

	Register(Define(Spec{
		Name:    "item.set_transform",
		Summary: "Sets the position, scale, rotation or opacity of a visual item.",
		Description: "Sets the given properties, rounded to 3 decimals. Scale 1, rotation 0 and opacity 1 clear the " +
			"stored value. A property that has keyframes cannot be set; use keyframe.set or keyframe.clear.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "x": 540, "scale": 1.5}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState},
		Scope:   ScopeEdit,
		Phase:   Alpha,
	}, applyItemSetTransform))

	Register(Define(Spec{
		Name:    "item.set_transition",
		Summary: "Sets or removes the transition on one edge of a visual item.",
		Description: "Sets a fade or a slide on the in or out edge, 0.5 seconds long unless duration is given; kind " +
			"null removes the transition. The two transitions together cannot be longer than the item.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "edge": "in", "kind": "fade"}`,
		Errors:  []string{CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState},
		Scope:   ScopeEdit,
		Phase:   Alpha,
	}, applyItemSetTransition))

	Register(Define(Spec{
		Name:    "item.set_audio",
		Summary: "Sets the volume, the fades or the mute flag of a clip or audio item.",
		Description: "Sets the given values. Volume 1, fades of 0 and audio_muted false clear the stored value. " +
			"audio_muted applies to clip items only. The two fades together cannot be longer than the item.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "volume": 0.8, "fade_in": "1s", "fade_out": "0.5s"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState,
		},
		Scope: ScopeEdit,
		Phase: Alpha,
	}, applyItemSetAudio))
}

type itemRenameArgs struct {
	ItemID string `json:"item_id" desc:"The item to label."`
	Name   string `json:"name"    desc:"The label, 0 to 120 characters; empty clears it." min:"0" max:"120"`
}

func applyItemRename(c *Context, a *itemRenameArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	it.Name = a.Name
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("name", a.Name)
	return nil
}

func isVisual(t timeline.ItemType) bool { return t != timeline.ItemAudio }

type itemTransformArgs struct {
	ItemID   string   `json:"item_id"            desc:"A clip, text, caption or shape item."`
	X        *float64 `json:"x,omitempty"        desc:"Centre of the item in pixels."`
	Y        *float64 `json:"y,omitempty"        desc:"Centre of the item in pixels."`
	Scale    *float64 `json:"scale,omitempty"    desc:"Uniform scale, greater than 0 and at most 20." max:"20"`
	Rotation *float64 `json:"rotation,omitempty" desc:"Degrees clockwise."`
	Opacity  *float64 `json:"opacity,omitempty"  desc:"0 to 1."                                       max:"1"  min:"0"`
}

func finite(path string, v *float64) *Error {
	if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0)) {
		return invalidArg(path, "%s must be a finite number.", path).WithHint("Send a number.")
	}
	return nil
}

func applyItemSetTransform(c *Context, a *itemTransformArgs) error {
	if err := atLeastOne([]string{"x", "y", "scale", "rotation", "opacity"},
		a.X != nil, a.Y != nil, a.Scale != nil, a.Rotation != nil, a.Opacity != nil); err != nil {
		return err
	}
	for _, p := range []struct {
		path string
		v    *float64
	}{{"args.x", a.X}, {"args.y", a.Y}, {"args.rotation", a.Rotation}} {
		if err := finite(p.path, p.v); err != nil {
			return err
		}
	}
	if a.Scale != nil && *a.Scale <= 0 {
		return invalidArg(
			"args.scale",
			"args.scale must be greater than 0 and at most 20, got %s.",
			trimFloat(*a.Scale),
		).
			WithHint("Use scale 1 for the original size.")
	}
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if !isVisual(it.Type) {
		return Errorf(CodeTypeMismatch, "Item %s is an audio item and has no transform.", a.ItemID).
			WithHint("Use item.set_audio for audio items.").WithPath("args.item_id")
	}
	var animated []string
	for _, p := range []struct {
		name  string
		given bool
	}{
		{"x", a.X != nil},
		{"y", a.Y != nil},
		{"scale", a.Scale != nil},
		{"rotation", a.Rotation != nil},
		{"opacity", a.Opacity != nil},
	} {
		if p.given && len(it.Keyframes[p.name]) > 0 {
			animated = append(animated, p.name)
		}
	}
	if len(animated) > 0 {
		first := animated[0]
		return Errorf(CodeInvalidState, "Property %s of %s has %d keyframes, which define its value.",
			first, a.ItemID, len(it.Keyframes[first])).
			WithHint("Use keyframe.set to change a keyframe, or keyframe.clear to remove them first.").
			WithDetail("properties", animated).WithPath("args." + first)
	}
	if a.X != nil {
		v := round3(*a.X)
		it.X = &v
		c.SetEffective("x", v)
	}
	if a.Y != nil {
		v := round3(*a.Y)
		it.Y = &v
		c.SetEffective("y", v)
	}
	if a.Scale != nil {
		it.Scale = round3(*a.Scale)
		c.SetEffective("scale", it.Scale)
		if it.Scale <= 0 { // a value that rounds to 0 is still not a scale
			return invalidArg("args.scale", "args.scale %s rounds to 0.", trimFloat(*a.Scale)).
				WithHint("Use a scale of at least 0.001.")
		}
	}
	if a.Rotation != nil {
		it.Rotation = round3(*a.Rotation)
		c.SetEffective("rotation", it.Rotation)
	}
	if a.Opacity != nil {
		it.Opacity = round3(*a.Opacity)
		c.SetEffective("opacity", it.Opacity)
	}
	c.Doc.Items[a.ItemID] = it
	return nil
}

var transitionKinds = []string{"fade", "slide_left", "slide_right", "slide_up", "slide_down"}

type itemTransitionArgs struct {
	ItemID   string `json:"item_id"            desc:"A clip, text, caption or shape item."`
	Edge     string `json:"edge"               desc:"The edge: in or out."                                                    enum:"in,out"`
	Kind     any    `json:"kind"               schema:"transition_kind" desc:"fade, slide_left, slide_right, slide_up, slide_down, or null to remove."`
	Duration *Time  `json:"duration,omitempty" desc:"Length of the transition, at least 1 frame; default 0.5s."`
}

func applyItemSetTransition(c *Context, a *itemTransitionArgs) error {
	kind, isString := a.Kind.(string)
	switch {
	case a.Kind == nil:
		if a.Duration != nil {
			return invalidArg("args.duration", "args.duration is given with kind null, which removes the transition.").
				WithHint("Omit duration, or send a kind.")
		}
	case !isString || !slices.Contains(transitionKinds, kind):
		return invalidArg("args.kind", "args.kind %v is not a transition kind.", a.Kind).
			WithHint("Use fade, slide_left, slide_right, slide_up, slide_down or null.")
	}
	length, requested := 0, "0.5s"
	if a.Kind != nil {
		if a.Duration != nil {
			requested = string(*a.Duration)
		}
		var err *Error
		if length, err = c.length("args.duration", requested, false); err != nil {
			return err
		}
	}
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if !isVisual(it.Type) {
		return Errorf(CodeTypeMismatch, "Item %s is an audio item; transitions apply to visual items.", a.ItemID).
			WithHint("Use item.set_audio fade_in and fade_out instead.").WithPath("args.item_id")
	}
	edge, other := &it.TransitionIn, it.TransitionOut
	otherName := "transition_out"
	if a.Edge == "out" {
		edge, other = &it.TransitionOut, it.TransitionIn
		otherName = "transition_in"
	}
	c.SetEffective("edge", a.Edge)
	if a.Kind == nil {
		*edge = nil
		c.SetEffective("kind", nil)
	} else {
		otherLen := 0
		if other != nil {
			otherLen = other.Duration
		}
		if length+otherLen > it.Duration {
			room := max(it.Duration-otherLen, 0)
			return Errorf(
				CodeInvalidState,
				"transition_%s %d frames plus %s %d frames exceeds duration %d of %s.",
				a.Edge, length, otherName, otherLen, it.Duration, a.ItemID,
			).
				WithHint("Use a duration of at most \"%df\" (%s) or shorten %s first.",
					room, c.seconds(room), otherName).WithPath("args.duration")
		}
		*edge = &timeline.Transition{Kind: kind, Duration: length}
		c.SetEffective("kind", kind)
		c.SetEffectiveTime("duration", length, requested)
	}
	c.Doc.Items[a.ItemID] = it
	return nil
}

type itemAudioArgs struct {
	ItemID     string   `json:"item_id"               desc:"A clip or audio item."`
	Volume     *float64 `json:"volume,omitempty"      desc:"Linear gain, 0 to 4."            min:"0" max:"4"`
	FadeIn     *Time    `json:"fade_in,omitempty"     desc:"Fade-in length; 0s removes it."`
	FadeOut    *Time    `json:"fade_out,omitempty"    desc:"Fade-out length; 0s removes it."`
	AudioMuted *bool    `json:"audio_muted,omitempty" desc:"Drop the audio of a clip."`
}

func applyItemSetAudio(c *Context, a *itemAudioArgs) error {
	if err := atLeastOne([]string{"volume", "fade_in", "fade_out", "audio_muted"},
		a.Volume != nil, a.FadeIn != nil, a.FadeOut != nil, a.AudioMuted != nil); err != nil {
		return err
	}
	var fadeIn, fadeOut int
	var err *Error
	if a.FadeIn != nil {
		if fadeIn, err = c.length("args.fade_in", string(*a.FadeIn), true); err != nil {
			return err
		}
	}
	if a.FadeOut != nil {
		if fadeOut, err = c.length("args.fade_out", string(*a.FadeOut), true); err != nil {
			return err
		}
	}
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if it.Type != timeline.ItemClip && it.Type != timeline.ItemAudio {
		return Errorf(CodeTypeMismatch, "Item %s is a %s item and has no audio.", a.ItemID, it.Type).
			WithHint("Use item.set_transform for visual properties.").WithPath("args.item_id")
	}
	if a.AudioMuted != nil && it.Type != timeline.ItemClip {
		return Errorf(CodeTypeMismatch, "Item %s is an audio item; audio_muted applies to clip items.", a.ItemID).
			WithHint("Use volume 0 to silence an audio item.").WithPath("args.audio_muted")
	}
	if a.Volume != nil && len(it.Keyframes["volume"]) > 0 {
		return Errorf(CodeInvalidState, "Property volume of %s has %d keyframes, which define its value.",
			a.ItemID, len(it.Keyframes["volume"])).
			WithHint("Use keyframe.set to change a keyframe, or keyframe.clear to remove them first.").
			WithDetail("properties", []string{"volume"}).WithPath("args.volume")
	}
	if a.FadeIn != nil {
		it.FadeIn = fadeIn
	}
	if a.FadeOut != nil {
		it.FadeOut = fadeOut
	}
	if it.FadeIn+it.FadeOut > it.Duration {
		name, other, otherLen := "fade_out", "fade_in", it.FadeIn
		given := it.FadeOut
		if a.FadeIn != nil {
			name, other, otherLen = "fade_in", "fade_out", it.FadeOut
			given = it.FadeIn
		}
		room := max(it.Duration-otherLen, 0)
		return Errorf(
			CodeInvalidState,
			"%s %d frames plus %s %d frames exceeds duration %d of %s.",
			name, given, other, otherLen, it.Duration, a.ItemID,
		).
			WithHint("Use %s of at most \"%df\" (%s) or shorten %s first.", name, room, c.seconds(room), other).
			WithPath("args." + name)
	}
	if a.Volume != nil {
		it.Volume = round3(*a.Volume)
	}
	if a.AudioMuted != nil {
		it.AudioMuted = *a.AudioMuted
	}
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("volume", it.Volume)
	c.SetEffective("audio_muted", it.AudioMuted)
	c.SetEffective("fade_in", TimeEffective(c.FPS(), it.FadeIn, requestedOf(a.FadeIn)))
	c.SetEffective("fade_out", TimeEffective(c.FPS(), it.FadeOut, requestedOf(a.FadeOut)))
	return nil
}

func requestedOf(t *Time) string {
	if t == nil {
		return ""
	}
	return string(*t)
}
