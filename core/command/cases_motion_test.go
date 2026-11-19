package command

// Golden cases of clip.replace_media (epic 33, SDD-002 C.78).

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const (
	vidShort  = "ast_k8m2n4p6q9" // video, 60 frames at 30 fps
	vidLong   = "ast_p4q5r6s7t8" // video, 1200 frames at 30 fps
	vidSame   = "ast_c2d3e4f5g6" // video, 900 frames at 30 fps, like vidAsset
	imgOther  = "ast_h7j8k9m2n3"
	spinAsset = "ast_a2n3b4m5g6" // an animated image of 12 frames
)

// withMotionAssets adds the assets of the cases of clip.replace_media: three videos (60, 900 and 1200 frames), a
// still and an animated image.
func withMotionAssets(d *timeline.Document) {
	{
		video := func(id string, frames int) timeline.Asset {
			return timeline.Asset{
				ID:   id,
				Hash: "sha256:" + strings.Repeat(fmt.Sprintf("%02x", frames%256), 32),
				Kind: timeline.AssetVideo,
				Name: id + ".mov",
				FrameRate: timeline.Rational{
					Num: 30,
					Den: 1,
				},
				FrameCount: frames,
				Width:      1080,
				Height:     1920,
				HasAlpha:   true,
			}
		}
		d.Assets[vidShort], d.Assets[vidLong], d.Assets[vidSame] = video(
			vidShort,
			60,
		), video(
			vidLong,
			1200,
		), video(
			vidSame,
			900,
		)
		d.Assets[imgOther] = timeline.Asset{
			ID: imgOther, Hash: "sha256:" + strings.Repeat("c3", 32), Kind: timeline.AssetImage, Name: "badge.png",
			Width: 256, Height: 256,
		}
		d.Assets[spinAsset] = timeline.Asset{
			ID: spinAsset, Hash: "sha256:" + strings.Repeat("d4", 32), Kind: timeline.AssetImage, Name: "spin.gif",
			Width: 128, Height: 128, FrameCount: 12, FrameDelays: []int{80, 80, 80, 80, 80, 80, 80, 80, 80, 80, 80, 80},
		}
	}
}

func init() {
	extra := withMotionAssets
	plain := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().with(extra).clip(itemA, vidTr, 0, 150, 0, mods...) }
	}
	still := func(it *timeline.Item) { it.Asset, it.SourceIn = imgAsset, 0 }
	rm := func(name, args string, b func() *B) {
		gc("clip.replace_media", name, b, op("clip.replace_media", args))
	}
	ra := func(asset string) string { return fmt.Sprintf(`{"item_id": %q, "asset_id": %q}`, itemA, asset) }

	rm("ok_video_for_video", ra(vidSame), plain())
	rm("ok_keeps_trim_speed_and_transform", ra(vidLong), plain(func(it *timeline.Item) {
		x, y := 120.0, -40.0
		it.SourceIn, it.Duration, it.Speed = 30, 100, timeline.Rational{Num: 2, Den: 1}
		it.X, it.Y, it.Scale, it.Rotation, it.Opacity = &x, &y, 1.5, 10, 0.8
		it.Fit, it.Volume, it.FadeIn = "contain", 0.5, 6
	}))
	rm("ok_keeps_keyframes_and_effects", ra(vidLong), plain(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(30, 2, "ease_out")}, "opacity": {kf(0, 0), kf(20, 1)}}
		half := 0.5
		it.Effects = []timeline.Effect{{Kind: "vignette", Amount: &half, Softness: &half}}
		it.Blur, it.Filter, it.FilterAmount = 4, "sepia", 0.5
	}))
	rm("ok_image_for_image", ra(imgOther), plain(still))
	rm("ok_animated_image_for_image", ra(spinAsset), plain(still))
	rm("ok_same_asset_noop", ra(vidAsset), plain())
	rm("ok_longer_video_keeps_duration", ra(vidLong), plain())
	rm("err_validation_bad_item_id", `{"item_id": "item1", "asset_id": "`+vidLong+`"}`, plain())
	rm("err_validation_placeholder_asset", ra("ast_placeholder"), plain())
	rm("err_not_found_item", `{"item_id": "itm_zzzzzzzzzz", "asset_id": "`+vidLong+`"}`, plain())
	rm("err_not_found_asset", ra("ast_zzzzzzzzzz"), plain())
	rm("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "asset_id": %q}`, itemB, vidLong),
		func() *B { return base().with(extra).audio(itemB, 0, 60, 0) })
	rm("err_type_mismatch_kind_differs", ra(imgOther), plain())
	rm("err_type_mismatch_audio_asset", ra(audAsset), plain())
	rm("err_invalid_state_slot_holder", ra(vidLong),
		func() *B { return base().template().with(extra).clip(itemA, vidTr, 0, 150, 0, placeholder) })
	rm("err_out_of_source_new_video_too_short", ra(vidShort), plain())
}
