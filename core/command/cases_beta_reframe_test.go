package command

import "github.com/AlexGalhardo/vukem-video/core/timeline"

// Golden cases of settings.reframe (SDD-002 C.60). The case documents are 1080x1920.
func init() {
	r := func(name, args string, b func() *B) { gc("settings.reframe", name, b, op("settings.reframe", args)) }

	at := func(x, y float64) func(*timeline.Item) {
		return func(it *timeline.Item) { it.X, it.Y = &x, &y }
	}
	scaled := func(s float64) func(*timeline.Item) { return func(it *timeline.Item) { it.Scale = s } }
	both := func(mods ...func(*timeline.Item)) func(*timeline.Item) {
		return func(it *timeline.Item) {
			for _, m := range mods {
				m(it)
			}
		}
	}
	fitNone := func(it *timeline.Item) { it.Fit = "none" }
	scene := func() *B {
		return base().
			clip(itemA, vidTr, 0, 150, 0).
			text(itemB, capTr, 0, 90, both(at(540, 1500), scaled(1.5))).
			shape("itm_x000000002", capTr, 90, 60).
			clip("itm_x000000003", capTr, 150, 90, 0, both(image, fitNone, at(900, 200))).
			caption("itm_x000000004", 300, 3).
			audio("itm_x000000005", 0, 300, 0)
	}

	r("ok_9_16_to_16_9_scales_overlays_by_the_height_ratio", `{"aspect": "16:9"}`, scene)
	r("ok_9_16_to_1_1", `{"aspect": "1:1"}`, scene)
	r("ok_9_16_to_4_5", `{"aspect": "4:5"}`, scene)
	r("ok_16_9_to_9_16_scales_overlays_by_the_width_ratio", `{"aspect": "9:16"}`, func() *B {
		return scene().with(func(d *timeline.Document) { d.Settings.Width, d.Settings.Height = 1920, 1080 })
	})
	r("ok_width_and_height", `{"width": 720, "height": 1280}`, scene)
	r("ok_scale_keyframes_follow", `{"aspect": "1:1"}`, func() *B {
		return base().text(itemA, vidTr, 0, 60, func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(30, 2)}}
		})
	})
	r("ok_cover_and_contain_clips_keep_their_scale", `{"aspect": "1:1"}`, func() *B {
		return base().
			clip(itemA, vidTr, 0, 150, 0, scaled(1.2)).
			clip(itemB, capTr, 0, 150, 0, func(it *timeline.Item) { it.Asset, it.Fit = imgAsset, "contain" })
	})
	r("ok_same_size_noop", `{"aspect": "9:16"}`, scene)
	r("err_validation_no_argument", `{}`, scene)
	r("err_validation_aspect_and_size", `{"aspect": "1:1", "width": 1080, "height": 1080}`, scene)
	r("err_validation_width_without_height", `{"width": 1080}`, scene)
	r("err_validation_odd_height", `{"width": 1080, "height": 1081}`, scene)
	r("err_validation_unknown_aspect", `{"aspect": "21:9"}`, scene)
	r("err_validation_scale_would_pass_20", `{"width": 2160, "height": 3840}`, func() *B {
		return base().text(itemA, vidTr, 0, 60, scaled(15))
	})
}
