package command

import (
	"fmt"
	"maps"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The golden cases of the commands that frame a recording (SDD-002 C.96 and C.97): the screen take is a clip of
// a 1920 by 1080 video on the Video track, the webcam take a clip of a 1280 by 720 video on the Captions track
// above it, and the frame is the format of the case.

const (
	scrAsset = "ast_s5c6r7e8e9"
	camAsset = "ast_c4m3r4a5b6"
	lowTr    = "trk_w3r4t5r6k0" // a visual track below the Video track
	bgTr     = "trk_b4g5r6d7t8" // the Background track
)

var epic66Assets = map[string]timeline.Asset{
	scrAsset: {
		ID: scrAsset, Hash: "sha256:" + strings.Repeat("d4", 32), Kind: timeline.AssetVideo, Name: "capture.mp4",
		FrameRate: timeline.Rational{Num: 30, Den: 1}, FrameCount: 900, Width: 1920, Height: 1080,
	},
	camAsset: {
		ID: camAsset, Hash: "sha256:" + strings.Repeat("e5", 32), Kind: timeline.AssetVideo, Name: "camera.mp4",
		FrameRate: timeline.Rational{Num: 30, Den: 1}, FrameCount: 900, Width: 1280, Height: 720,
	},
}

// layoutDoc is a project of w by h with a screen clip (itemA, Video track) and a camera clip (itemB, on the track
// above it, or below it with below true).
func layoutDoc(w, h int, below bool, mods ...func(d *timeline.Document)) func() *B {
	return func() *B {
		b := base().with(func(d *timeline.Document) {
			d.Settings.Width, d.Settings.Height = w, h
			maps.Copy(d.Assets, epic66Assets)

			d.Tracks[lowTr] = timeline.Track{ID: lowTr, Kind: timeline.TrackVisual, Name: "Camera", Order: "a15"}
		}).clip(itemA, vidTr, 0, 300, 0, func(it *timeline.Item) { it.Asset = scrAsset })

		camTrack := capTr
		if below {
			camTrack = lowTr
		}

		b.clip(itemB, camTrack, 0, 300, 0, func(it *timeline.Item) { it.Asset = camAsset })

		for _, m := range mods {
			b.with(m)
		}

		return b
	}
}

func layoutArgs(layout string, rest ...string) string {
	return "{" + strings.Join(append([]string{
		fmt.Sprintf(`"layout": %q`, layout), fmt.Sprintf(`"screen_item_id": %q`, itemA),
	}, rest...), ", ") + "}"
}

func withCamera(rest ...string) []string {
	return append([]string{fmt.Sprintf(`"camera_item_id": %q`, itemB)}, rest...)
}

func init() {
	l := func(name string, b func() *B, layout string, rest ...string) {
		gc("recording.apply_layout", name, b, op("recording.apply_layout", layoutArgs(layout, rest...)))
	}

	wide, square, four5, tall := layoutDoc(
		1920,
		1080,
		false,
	), layoutDoc(
		1080,
		1080,
		false,
	), layoutDoc(
		1080,
		1350,
		false,
	),
		layoutDoc(
			1080,
			1920,
			false,
		)

	l("ok_pip_16_9", wide, "pip", withCamera()...)
	l("ok_pip_9_16", tall, "pip", withCamera()...)
	l("ok_pip_rect_top_left", square, "pip", withCamera(`"corner": "top_left"`, `"camera_shape": "rect"`)...)
	l("ok_split_16_9", wide, "split", withCamera()...)
	l("ok_split_1_1", square, "split", withCamera()...)
	l("ok_split_4_5", four5, "split", withCamera()...)
	l("ok_split_9_16", tall, "split", withCamera()...)
	l("ok_camera_focus_16_9", wide, "camera_focus", withCamera(`"corner": "top_right"`)...)
	l("ok_camera_focus_9_16", tall, "camera_focus", withCamera()...)
	l("ok_screen_only_with_padding", four5, "screen_only", `"padding": 0.05`)
	l("ok_camera_brought_in_front", layoutDoc(1920, 1080, true), "pip", withCamera()...)
	l("err_validation_screen_only_with_camera", wide, "screen_only", withCamera()...)
	l("err_validation_camera_missing", wide, "split")
	l("err_validation_padding_out_of_range", wide, "pip", withCamera(`"padding": 0.4`)...)
	l("err_validation_same_clip", wide, "pip", fmt.Sprintf(`"camera_item_id": %q`, itemA))
	l("err_not_found", wide, "pip", `"camera_item_id": "itm_zzzzzzzzzz"`)
	l("err_type_mismatch_not_a_clip", func() *B { return wide().audio(itemC, 0, 90, 0) }, "pip",
		fmt.Sprintf(`"camera_item_id": %q`, itemC))
	l("err_invalid_state_keyframed", layoutDoc(1920, 1080, false, func(d *timeline.Document) {
		it := d.Items[itemA]
		it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(60, 1.5)}}
		d.Items[itemA] = it
	}), "pip", withCamera()...)
	l("err_invalid_state_asset_without_size", layoutDoc(1920, 1080, false, func(d *timeline.Document) {
		a := d.Assets[camAsset]
		a.Width, a.Height = 0, 0
		d.Assets[camAsset] = a
	}), "pip", withCamera()...)
}

const (
	bgNewTrack = "trk_n3w7r4ck0e"
	bgNewItem  = "itm_b9g1t3m5k7"
	bgOldItem  = "itm_p1d3b5g7m9"
)

// withBackground is a take that already has a background: the Background track and one shape on it, and a shadow.
func withBackground() *B {
	return layoutDoc(1920, 1080, false)().with(func(d *timeline.Document) {
		d.Tracks[bgTr] = timeline.Track{ID: bgTr, Kind: timeline.TrackVisual, Name: BackgroundTrackName, Order: "a3"}
		it := timeline.NewItem(bgOldItem, timeline.ItemShape, bgTr, 0, 300)
		it.Name, it.Shape, it.Width, it.Height, it.Fill = backgroundItemName+itemA, "rect", 1920, 1080, "#FF0000"
		d.Items[bgOldItem] = it
		clip := d.Items[itemA]
		clip.Shadow = &timeline.Shadow{Color: "#000000", Opacity: 0.5, DY: 13, Blur: 32}
		d.Items[itemA] = clip
	})
}

func backgroundArgs(rest ...string) string {
	return cmdArgs(itemA, rest...)
}

func init() {
	b := func(name string, build func() *B, rest ...string) {
		gc("recording.set_background", name, build, op("recording.set_background", backgroundArgs(rest...)))
	}
	bIDs := func(name string, build func() *B, rest ...string) {
		gcIDs("recording.set_background", name, build, op("recording.set_background", backgroundArgs(rest...)),
			bgNewTrack, bgNewItem)
	}

	plain := layoutDoc(1920, 1080, false)

	bIDs("ok_color", plain, `"kind": "color"`, `"color": "#1E293B"`)
	bIDs("ok_image", plain, `"kind": "image"`, fmt.Sprintf(`"asset_id": %q`, imgAsset))
	bIDs("ok_color_without_shadow", plain, `"kind": "color"`, `"shadow": false`)
	gcIDs("recording.set_background", "ok_replaces_the_previous_background", withBackground,
		op("recording.set_background", backgroundArgs(`"kind": "color"`, `"color": "#00FF88"`)), bgNewItem)
	b("ok_none_removes_background_and_shadow", withBackground, `"kind": "none"`)
	b("err_validation_color_with_asset", plain, `"kind": "color"`, fmt.Sprintf(`"asset_id": %q`, imgAsset))
	b("err_validation_image_without_asset", plain, `"kind": "image"`)
	gc("recording.set_background", "err_not_found", plain,
		op("recording.set_background", cmdArgs("itm_zzzzzzzzzz", `"kind": "color"`)))
	gc("recording.set_background", "err_type_mismatch_not_a_clip", func() *B { return base().audio(itemA, 0, 90, 0) },
		op("recording.set_background", backgroundArgs(`"kind": "color"`)))
	b("err_type_mismatch_asset_not_an_image", plain, `"kind": "image"`, fmt.Sprintf(`"asset_id": %q`, scrAsset))
	gcIDs("recording.set_background", "err_limit_items", func() *B {
		return plain().with(func(d *timeline.Document) {
			d.Tracks["trk_fe00000001"] = timeline.Track{ID: "trk_fe00000001", Kind: timeline.TrackVisual, Order: "a9"}
		}).manyItems("trk_fe00000001", timeline.MaxItems-2)
	}, op("recording.set_background", backgroundArgs(`"kind": "color"`)), bgNewTrack, bgNewItem)
}
