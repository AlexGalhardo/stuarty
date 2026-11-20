package command

// Golden cases of the colour properties of keyframe.set and their guard in clip.set_color (TASK-0254).

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	clip := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 90, 0, mods...) }
	}
	keys := func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"brightness": {kf(0, 0), kf(60, 0.5)}}
	}
	set := func(name, rest string, b func() *B) {
		gc("keyframe.set", name, b, op("keyframe.set", fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest)))
	}

	set("ok_colour_property_on_a_clip", `"property": "saturation", "at": "30f", "value": 1.5`, clip())
	set("ok_colour_property_at_its_limits", `"property": "hue", "at": "60f", "value": -180`, clip(keys))
	set("err_validation_colour_above_range", `"property": "contrast", "at": "30f", "value": 2.5`, clip())
	set("err_type_mismatch_colour_on_text", `"property": "hue", "at": "60f", "value": 10`,
		func() *B { return base().text(itemA, vidTr, 0, 90) })

	gc("keyframe.clear", "ok_colour_property_returns_to_static", clip(keys),
		op("keyframe.clear", fmt.Sprintf(`{"item_id": %q, "property": "brightness"}`, itemA)))
	gc("clip.set_color", "err_invalid_state_property_has_keyframes", clip(keys),
		op("clip.set_color", fmt.Sprintf(`{"item_id": %q, "brightness": 0.2}`, itemA)))
}
