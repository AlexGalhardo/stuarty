package command

// Golden cases of clip.set_effects (TASK-0278).

import (
	"fmt"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	se := func(name, args string, b func() *B) { gc("clip.set_effects", name, b, op("clip.set_effects", args)) }
	ea := func(effects string) string { return fmt.Sprintf(`{"item_id": %q, "effects": %s}`, itemA, effects) }
	plain := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 90, 0, mods...) }
	}
	f := func(v float64) *float64 { return &v }
	n := func(v int) *int { return &v }
	has := func(effects ...timeline.Effect) func(*timeline.Item) {
		return func(it *timeline.Item) { it.Effects = effects }
	}

	se("ok_each_kind_with_defaults", ea(`[{"kind": "motion_blur"}, {"kind": "glow"}, {"kind": "vignette"}, `+
		`{"kind": "grain"}, {"kind": "rgb_split"}, {"kind": "shake"}]`), plain())
	se("ok_motion_blur_parameters", ea(`[{"kind": "motion_blur", "amount": 0.8, "angle": -45}]`), plain())
	se("ok_glow_parameters", ea(`[{"kind": "glow", "amount": 1, "radius": 40, "threshold": 0.2}]`), plain())
	se("ok_vignette_parameters", ea(`[{"kind": "vignette", "amount": 0.9, "softness": 0}]`), plain())
	se("ok_grain_parameters", ea(`[{"kind": "grain", "amount": 0.6, "size": 3, "seed": 1234}]`), plain())
	se("ok_rgb_split_parameters", ea(`[{"kind": "rgb_split", "amount": 1, "angle": 180}]`), plain())
	se("ok_shake_parameters", ea(`[{"kind": "shake", "amount": 0.7, "speed": 30, "seed": 65535}]`), plain())
	se("ok_amount_0_is_stored", ea(`[{"kind": "vignette", "amount": 0}]`), plain())
	se("ok_decimals_rounded_to_3", ea(`[{"kind": "glow", "amount": 0.12345}]`), plain())
	se("ok_order_is_kept", ea(`[{"kind": "grain"}, {"kind": "vignette"}]`), plain())
	se("ok_replaces_the_whole_list", ea(`[{"kind": "glow"}]`),
		plain(has(timeline.Effect{Kind: "vignette", Amount: f(0.5), Softness: f(0.5)},
			timeline.Effect{Kind: "grain", Amount: f(0.3), Size: n(1), Seed: n(0)})))
	se("ok_empty_list_removes_them", ea(`[]`),
		plain(has(timeline.Effect{Kind: "vignette", Amount: f(0.5), Softness: f(0.5)})))
	se("ok_empty_list_without_effects_noop", ea(`[]`), plain())
	se("ok_same_list_noop", ea(`[{"kind": "vignette", "amount": 0.5, "softness": 0.5}]`),
		plain(has(timeline.Effect{Kind: "vignette", Amount: f(0.5), Softness: f(0.5)})))
	se("err_validation_unknown_kind", ea(`[{"kind": "sparkle"}]`), plain())
	se("err_validation_kind_listed_twice", ea(`[{"kind": "glow"}, {"kind": "glow", "radius": 5}]`), plain())
	se("err_validation_parameter_of_another_kind", ea(`[{"kind": "vignette", "radius": 5}]`), plain())
	se("err_validation_amount_above_1", ea(`[{"kind": "grain", "amount": 1.5}]`), plain())
	se("err_validation_angle_above_180", ea(`[{"kind": "motion_blur", "angle": 200}]`), plain())
	se("err_validation_radius_below_1", ea(`[{"kind": "glow", "radius": 0.5}]`), plain())
	se("err_validation_size_above_8", ea(`[{"kind": "grain", "size": 9}]`), plain())
	se("err_validation_speed_0", ea(`[{"kind": "shake", "speed": 0}]`), plain())
	se("err_validation_seed_above_65535", ea(`[{"kind": "grain", "seed": 70000}]`), plain())
	se(
		"err_validation_seven_effects",
		ea(`[{"kind": "glow"}, {"kind": "grain"}, {"kind": "vignette"}, {"kind": "shake"}, `+
			`{"kind": "rgb_split"}, {"kind": "motion_blur"}, {"kind": "glow"}]`),
		plain(),
	)
	se("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "effects": []}`, plain())
	se("err_type_mismatch_text_item", fmt.Sprintf(`{"item_id": %q, "effects": []}`, itemC),
		func() *B { return base().text(itemC, vidTr, 0, 60) })
}
