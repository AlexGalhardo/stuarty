package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	pr := func(name, args string, b func() *B) { gc("text.apply_preset", name, b, op("text.apply_preset", args)) }
	pa := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemC, rest) }
	// The text item of the example of C.30: 90 frames on the video track.
	textItem := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().text(itemC, vidTr, 0, 90, mods...) }
	}
	longText := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().text(itemC, vidTr, 0, 200, mods...) }
	}

	pr("ok_fade_up_in_default_duration", pa(`"preset": "fade_up", "edge": "in"`), textItem())
	pr("ok_fade_up_out", pa(`"preset": "fade_up", "edge": "out"`), textItem())
	pr("ok_fade_down_in", pa(`"preset": "fade_down", "edge": "in"`), textItem())
	pr("ok_fade_down_out", pa(`"preset": "fade_down", "edge": "out"`), textItem())
	pr("ok_pop_in", pa(`"preset": "pop", "edge": "in"`), textItem())
	pr("ok_pop_out", pa(`"preset": "pop", "edge": "out"`), textItem())
	pr("ok_pop_n_1_no_overshoot_keyframe", pa(`"preset": "pop", "edge": "in", "duration": "1f"`), textItem())
	pr("ok_pop_n_2", pa(`"preset": "pop", "edge": "out", "duration": "2f"`), textItem())
	pr("ok_zoom_in_in", pa(`"preset": "zoom_in", "edge": "in"`), textItem())
	pr("ok_zoom_in_out", pa(`"preset": "zoom_in", "edge": "out"`), textItem())
	pr("ok_zoom_in_out_scale_capped_at_20", pa(`"preset": "zoom_in", "edge": "out"`),
		textItem(func(it *timeline.Item) { it.Scale = 18 }))
	pr("ok_slide_left_in", pa(`"preset": "slide_left", "edge": "in"`), textItem())
	pr("ok_slide_left_out", pa(`"preset": "slide_left", "edge": "out"`), textItem())
	pr("ok_slide_right_in", pa(`"preset": "slide_right", "edge": "in"`), textItem())
	pr("ok_slide_right_out_replaces_existing_transition", pa(`"preset": "slide_right", "edge": "out"`),
		textItem(func(it *timeline.Item) { it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 20} }))
	pr("ok_explicit_duration", pa(`"preset": "fade_up", "edge": "in", "duration": "20f"`), textItem())
	pr("ok_duration_equals_item_duration", pa(`"preset": "fade_up", "edge": "in", "duration": "90f"`), textItem())
	pr("ok_base_from_static_non_default_scale_and_opacity", pa(`"preset": "zoom_in", "edge": "in"`),
		textItem(func(it *timeline.Item) { it.Scale, it.Opacity = 1.5, 0.8 }))
	pr("ok_base_from_existing_keyframes_outside_range", pa(`"preset": "zoom_in", "edge": "in"`),
		textItem(func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{"scale": {kf(20, 1.5), kf(60, 2)}}
		}))
	pr("ok_replaces_keyframes_inside_range_keeps_outside", pa(`"preset": "fade_up", "edge": "in"`),
		textItem(func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{"opacity": {kf(5, 0.5), kf(40, 0.9), kf(80, 1)}}
		}))
	pr("ok_last_keyframe_inherits_easing", pa(`"preset": "fade_up", "edge": "in"`),
		textItem(func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{"opacity": {kf(0, 0.5, "ease_in_out"), kf(30, 1)}}
		}))
	pr("ok_in_and_out_presets_combined", pa(`"preset": "fade_up", "edge": "in"`),
		textItem(func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{
				"opacity": {kf(78, 1, "ease_in"), kf(90, 0)},
				"y":       {kf(78, 960, "ease_in"), kf(90, 864)},
			}
		}))
	pr("ok_clip_caption_shape_items", fmt.Sprintf(`{"item_id": %q, "preset": "pop", "edge": "in"}`, itemA), func() *B {
		return base().clip(itemA, vidTr, 0, 90, 0).caption(itemB, 100, 3).shape(itemC, capTr, 200, 60)
	})
	pr("ok_same_preset_twice_noop", pa(`"preset": "fade_up", "edge": "in"`), textItem(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{
			"opacity": {kf(0, 0, "ease_out"), kf(12, 1)},
			"y":       {kf(0, 1056, "ease_out"), kf(12, 960)},
		}
	}))
	pr("err_validation_unknown_preset", pa(`"preset": "bounce", "edge": "in"`), textItem())
	pr("err_invalid_time_zero_duration", pa(`"preset": "pop", "edge": "in", "duration": "0f"`), textItem())
	pr("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "preset": "pop", "edge": "in"}`, textItem())
	pr("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "preset": "pop", "edge": "in"}`, itemB),
		func() *B { return base().audio(itemB, 0, 90, 0) })
	pr(
		"err_invalid_state_duration_longer_than_item",
		pa(`"preset": "pop", "edge": "in", "duration": "120f"`),
		textItem(),
	)
	pr("err_invalid_state_slide_transitions_do_not_fit", pa(`"preset": "slide_left", "edge": "in"`),
		func() *B {
			return base().text(itemC, vidTr, 0, 20, func(it *timeline.Item) {
				it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 15}
			})
		})
	pr("err_limit_101_keyframes", pa(`"preset": "fade_up", "edge": "in"`), longText(func(it *timeline.Item) {
		var ks []timeline.Keyframe
		for t := 20; t < 119; t++ {
			ks = append(ks, kf(t, 0.5))
		}
		it.Keyframes = timeline.Keyframes{"opacity": ks}
	}))
}

func init() {
	sa := func(name, args string, b func() *B) {
		// A case that reaches the id of the new item without supplying one draws it from the generator.
		needsID := !strings.Contains(args, `"id"`) && (strings.HasPrefix(name, "ok_") ||
			name == "err_overlap" || name == "err_limit_500_items")
		if needsID {
			gcIDs("shape.add", name, b, op("shape.add", args), "itm_s6t7v8w9x0")
			return
		}
		gc("shape.add", name, b, op("shape.add", args))
	}
	add := func(rest string) string {
		return fmt.Sprintf(`{"track_id": %q, "start": "0s", "duration": "3s", %s}`, vidTr, rest)
	}
	empty := base

	sa("ok_rect_minimal", add(`"shape": "rect", "width": 400, "height": 200`), empty)
	sa("ok_ellipse", add(`"shape": "ellipse", "width": 300, "height": 300`), empty)
	sa("ok_rounded_rect_with_radius", add(`"shape": "rounded_rect", "width": 400, "height": 200, "radius": 24`), empty)
	sa("ok_fill_lower_case_stored_upper_case",
		add(`"shape": "rect", "width": 400, "height": 200, "fill": "#ff0000", "x": 540, "y": 960`), empty)
	sa(
		"ok_fill_equal_to_default_not_stored",
		add(`"shape": "rect", "width": 400, "height": 200, "fill": "#ffffff"`),
		empty,
	)
	sa("ok_stroke",
		add(`"shape": "rect", "width": 400, "height": 200, "stroke_color": "#00ff00", "stroke_width": 4`), empty)
	sa("ok_template_color_reference", add(`"shape": "rect", "width": 400, "height": 200, "fill": "$accent"`),
		func() *B { return base().template() })
	sa("ok_supplied_id", add(`"shape": "rect", "width": 400, "height": 200, "id": "itm_s6t7v8w9x0"`), empty)
	sa("err_validation_width_0", add(`"shape": "rect", "width": 0, "height": 200`), empty)
	sa("err_validation_radius_on_rect", add(`"shape": "rect", "width": 400, "height": 200, "radius": 8`), empty)
	sa("err_validation_bad_color", add(`"shape": "rect", "width": 400, "height": 200, "fill": "red"`), empty)
	sa(
		"err_invalid_time_zero_duration",
		fmt.Sprintf(
			`{"track_id": %q, "start": "0s", "duration": "0s", "shape": "rect", "width": 400, "height": 200}`,
			vidTr,
		),
		empty,
	)
	sa(
		"err_not_found_track",
		`{"track_id": "trk_zzzzzzzzzz", "start": "0s", "duration": "3s", "shape": "rect", "width": 400, "height": 200}`,
		empty,
	)
	sa(
		"err_type_mismatch_audio_track",
		fmt.Sprintf(
			`{"track_id": %q, "start": "0s", "duration": "3s", "shape": "rect", "width": 400, "height": 200}`,
			audTr,
		),
		empty,
	)
	sa("err_id_exists", add(fmt.Sprintf(`"shape": "rect", "width": 400, "height": 200, "id": %q`, itemA)),
		func() *B { return base().clip(itemA, vidTr, 200, 60, 0) })
	sa("err_limit_500_items", add(`"shape": "rect", "width": 400, "height": 200`),
		func() *B { return base().manyItems(vidTr, timeline.MaxItems) })
	sa("err_overlap", add(`"shape": "rect", "width": 400, "height": 200`),
		func() *B { return base().clip(itemA, vidTr, 0, 60, 0) })
}

func init() {
	ss := func(name, args string, b func() *B) { gc("shape.set", name, b, op("shape.set", args)) }
	args := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemC, rest) }
	shapeDoc := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B {
			b := base().shape(itemC, capTr, 0, 90)
			it := b.d.Items[itemC]
			it.Width, it.Height, it.Fill = 400, 200, "#FF0000"
			for _, m := range mods {
				m(&it)
			}
			b.d.Items[itemC] = it
			return b
		}
	}
	rounded := func(it *timeline.Item) { it.Shape, it.Radius = "rounded_rect", 24 }

	ss("ok_width_height", args(`"width": 600, "height": 300.5`), shapeDoc())
	ss("ok_fill_and_stroke", args(`"fill": "#00ff00", "stroke_color": "#0000ff", "stroke_width": 3`), shapeDoc())
	ss("ok_to_rounded_rect_with_radius", args(`"shape": "rounded_rect", "radius": 24, "width": 600`), shapeDoc())
	ss("ok_change_to_ellipse_removes_radius", args(`"shape": "ellipse"`), shapeDoc(rounded))
	ss("ok_radius_on_rounded_rect", args(`"radius": 40`), shapeDoc(rounded))
	ss("ok_default_value_removes_field", args(`"fill": "#ffffff", "stroke_width": 0`),
		shapeDoc(func(it *timeline.Item) { it.StrokeWidth = 4 }))
	ss("ok_same_values_noop", args(`"width": 400, "fill": "#ff0000"`), shapeDoc())
	ss("err_validation_no_field", args(`"shape": null`), shapeDoc())
	ss("err_validation_height_0", args(`"height": 0`), shapeDoc())
	ss(
		"err_validation_radius_on_ellipse",
		args(`"radius": 5`),
		shapeDoc(func(it *timeline.Item) { it.Shape = "ellipse" }),
	)
	ss("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "width": 10}`, shapeDoc())
	ss("err_type_mismatch_clip_item", fmt.Sprintf(`{"item_id": %q, "width": 10}`, itemA),
		func() *B { return base().clip(itemA, vidTr, 0, 60, 0) })
}
