package command

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Golden cases of clip.blur_region (SDD-002 C.90). The clip is 150 frames at frame 0 on the video track,
// which lies below the caption track (order a0 above a1).

const blurTr = "trk_b2c3d4e5f6"

func init() {
	blur := func(name, args string, b func() *B) { gc("clip.blur_region", name, b, op("clip.blur_region", args)) }
	region := func(rest string) string {
		s := fmt.Sprintf(`{"item_id": %q, "x": 0.1, "y": 0.4, "width": 0.8, "height": 0.1`, itemA)
		if rest != "" {
			s += ", " + rest
		}
		return s + "}"
	}
	withIDs := func(rest string) string {
		return region(fmt.Sprintf(`"new_id": %q`, itemC) + ", " + rest)
	}
	newTrack := fmt.Sprintf(`"new_track_id": %q`, blurTr)
	clipA := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 150, 0, mods...) }
	}
	blurTrack := func(d *timeline.Document) {
		d.Tracks[blurTr] = timeline.Track{ID: blurTr, Kind: timeline.TrackVisual, Name: BlurTrackName, Order: "a05"}
	}
	rate2 := timeline.Rational{Num: 2, Den: 1}
	one := timeline.Rational{Num: 1, Den: 1}

	blur("ok_whole_clip", withIDs(newTrack), clipA())
	blur("ok_range_in_the_middle", withIDs(newTrack+`, "start": "1s", "duration": "2s"`), clipA())
	blur("ok_creates_the_blur_track", withIDs(newTrack+`, "start": "30f"`), clipA(func(it *timeline.Item) {
		it.Opacity, it.Rotation = 0.8, 5
		it.FadeIn, it.FadeOut, it.Denoise = 10, 10, 0.4
		it.Shadow = &timeline.Shadow{Color: "#000000", Opacity: 0.5, DX: 2, DY: 2, Blur: 4}
		it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(100, 1.2)}, "volume": {kf(0, 1), kf(50, 0.5)}}
	}))
	blur("ok_reuses_the_blur_track", withIDs(`"start": "30f", "duration": "30f"`), func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0).with(blurTrack)
	})
	blur("ok_given_track", withIDs(fmt.Sprintf(`"track_id": %q, "start": "1s", "duration": "1s"`, capTr)), clipA())
	blur("ok_radius_and_feather", withIDs(newTrack+`, "radius": 60, "feather": 12`), clipA())
	blur("ok_strength_medium", withIDs(newTrack+`, "strength": "medium"`), clipA())
	blur("ok_strength_strong", withIDs(newTrack+`, "strength": "strong"`), clipA())
	blur("ok_sped_up_clip", withIDs(newTrack+`, "start": "30f", "duration": "30f"`), clipA(func(it *timeline.Item) {
		it.Speed, it.SourceIn = rate2, 60
	}))
	// VV-392: the blur repeats the edge pixels, so a region at the edge is not reported as weak.
	blur(
		"ok_at_the_edge_is_not_near_edge",
		fmt.Sprintf(
			`{"item_id": %q, "x": 0, "y": 0.9, "width": 1, "height": 0.1, "new_id": %q, %s}`,
			itemA,
			itemC,
			newTrack,
		),
		clipA(),
	)
	blur(
		"ok_near_edge_with_a_feather_is_reported",
		fmt.Sprintf(
			`{"item_id": %q, "x": 0, "y": 0.9, "width": 1, "height": 0.1, "feather": 12, "new_id": %q, %s}`,
			itemA,
			itemC,
			newTrack,
		),
		clipA(),
	)
	blur(
		"ok_image_clip",
		fmt.Sprintf(
			`{"item_id": %q, "x": 0.2, "y": 0.2, "width": 0.3, "height": 0.3, "new_id": %q, %s}`,
			itemA,
			itemC,
			newTrack,
		),
		func() *B {
			return base().clip(itemA, vidTr, 0, 90, 0, func(it *timeline.Item) { it.Asset = imgAsset })
		},
	)

	blur("err_validation_region_outside",
		fmt.Sprintf(`{"item_id": %q, "x": 0.5, "y": 0.4, "width": 0.8, "height": 0.1}`, itemA), clipA())
	blur("err_validation_radius_below_minimum", withIDs(newTrack+`, "radius": 4`), clipA())
	blur("err_validation_strength_unknown", withIDs(newTrack+`, "strength": "gentle"`), clipA())
	blur("err_validation_strength_and_radius", withIDs(newTrack+`, "strength": "medium", "radius": 60`), clipA())
	blur("err_validation_track_not_above", withIDs(fmt.Sprintf(`"track_id": %q`, vidTr)), clipA())
	blur("err_not_found_item",
		`{"item_id": "itm_zzzzzzzzzz", "x": 0.1, "y": 0.4, "width": 0.8, "height": 0.1}`, clipA())
	blur("err_not_found_track", withIDs(`"track_id": "trk_zzzzzzzzzz"`), clipA())
	blur("err_type_mismatch_text_item",
		fmt.Sprintf(`{"item_id": %q, "x": 0.1, "y": 0.4, "width": 0.8, "height": 0.1}`, itemC),
		func() *B { return base().text(itemC, vidTr, 0, 60) })
	blur("err_type_mismatch_audio_track", withIDs(fmt.Sprintf(`"track_id": %q`, audTr)), clipA())
	blur("err_invalid_state_speed_curve", withIDs(newTrack), clipA(func(it *timeline.Item) {
		it.SpeedCurve = []timeline.SpeedPoint{{T: 0, Speed: one}, {T: 1, Speed: rate2}}
	}))
	blur("err_invalid_time_range_outside_clip", withIDs(`"start": "20s"`), clipA())
	blur(
		"err_id_exists",
		fmt.Sprintf(
			`{"item_id": %q, "x": 0.1, "y": 0.4, "width": 0.8, "height": 0.1, "new_id": %q, "new_track_id": "trk_b2c3d4e5f6"}`,
			itemA,
			itemA,
		),
		clipA(),
	)
	blur(
		"err_overlap_given_track",
		withIDs(fmt.Sprintf(`"track_id": %q, "start": "1s", "duration": "1s"`, capTr)),
		func() *B {
			return base().clip(itemA, vidTr, 0, 150, 0).text(itemB, capTr, 20, 60)
		},
	)
}

func init() {
	gc(
		"clip.blur_region",
		"err_limit_500_items",
		func() *B { return base().clip(itemA, vidTr, 0, 150, 0).manyItems(capTr, 499) },
		op(
			"clip.blur_region",
			fmt.Sprintf(
				`{"item_id": %q, "x": 0.1, "y": 0.4, "width": 0.8, "height": 0.1, "new_id": %q, "new_track_id": "trk_b2c3d4e5f6"}`,
				itemA,
				itemC,
			),
		),
	)
}
