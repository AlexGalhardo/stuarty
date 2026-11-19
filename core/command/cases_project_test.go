package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func base() *B { return newCaseDoc() }

func noAssets() *B {
	return base().with(func(d *timeline.Document) { d.Assets = map[string]timeline.Asset{} })
}

func hashN(n int) string { return fmt.Sprintf("sha256:%064x", n+1) }

func withAssets(n int) *B {
	b := noAssets()
	for i := range n {
		id := fmt.Sprintf("ast_x%09d", i)
		b.d.Assets[id] = timeline.Asset{
			ID:     id,
			Hash:   hashN(i),
			Kind:   timeline.AssetImage,
			Name:   "i",
			Width:  10,
			Height: 10,
		}
	}
	return b
}

func assetArg(extra string) string {
	return `{"asset": {"id": "ast_k000000001", "hash": "` + hashN(1000) + `", "name": "clip.mov"` + extra + `}}`
}

func init() {
	// project.rename
	r := func(name, args string, b func() *B) { gc("project.rename", name, b, op("project.rename", args)) }
	r("ok_basic", `{"name": "Launch teaser v2"}`, base)
	r("ok_min_length_1", `{"name": "A"}`, base)
	r("ok_max_length_120", `{"name": "`+strings.Repeat("n", 120)+`"}`, base)
	r("ok_same_name_noop", `{"name": "Launch teaser"}`, base)
	r("err_validation_empty_name", `{"name": ""}`, base)
	r("err_validation_121_chars", `{"name": "`+strings.Repeat("n", 121)+`"}`, base)

	// settings.set
	s := func(name, args string, b func() *B) { gc("settings.set", name, b, op("settings.set", args)) }
	positioned := func() *B {
		return base().text(itemC, vidTr, 0, 60, func(it *timeline.Item) {
			x, y := 810.0, 480.0
			it.X, it.Y = &x, &y
		})
	}
	s("ok_width_only", `{"width": 720}`, positioned)
	s("ok_height_only", `{"height": 1280}`, positioned)
	s("ok_both_sizes_rescale_items", `{"width": 720, "height": 1280}`, positioned)
	s("ok_default_position_items_stay_centred", `{"width": 720, "height": 1280}`, func() *B {
		return base().text(itemC, vidTr, 0, 60)
	})
	s("ok_keyframed_x_rescaled", `{"width": 720}`, func() *B {
		return base().text(itemC, vidTr, 0, 60, func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{"x": {kf(0, 810), kf(30, 270)}}
		})
	})
	s("ok_background", `{"background": "#112233"}`, base)
	s("ok_background_with_alpha_uppercased", `{"background": "#aabbccdd"}`, base)
	s("ok_background_transparent", `{"background": "transparent"}`, base)
	s("err_validation_background_unknown_keyword", `{"background": "clear"}`, base)
	s("ok_loudness_boundaries_minus30_minus5", `{"loudness_target": -30}`, base)
	s("ok_size_boundaries_16_and_4096", `{"width": 16, "height": 4096}`, base)
	s("ok_same_values_noop", `{"width": 1080, "height": 1920}`, positioned)
	s("err_validation_no_argument", `{}`, base)
	s("err_validation_odd_width", `{"width": 1081}`, base)
	s("err_validation_width_15", `{"width": 15}`, base)
	s("err_validation_width_4098", `{"width": 4098}`, base)
	s("err_validation_loudness_minus31", `{"loudness_target": -31}`, base)
	s("err_validation_unknown_color_reference", `{"background": "$brand"}`, base)

	// asset.add
	a := func(name, args string, b func() *B) { gc("asset.add", name, b, op("asset.add", args)) }
	video := `, "kind": "video", "frame_rate": {"num": 30, "den": 1}, "frame_count": 900, "width": 1080, "height": 1920`
	a("ok_video_with_audio", assetArg(video+`, "sample_count": 1440000, "has_audio": true`), noAssets)
	a("ok_video_without_audio", assetArg(video), noAssets)
	a("ok_audio", assetArg(`, "kind": "audio", "sample_count": 96000`), noAssets)
	a("ok_image", assetArg(`, "kind": "image", "width": 512, "height": 512`), noAssets)
	a(
		"ok_same_hash_returns_existing_id",
		`{"asset": {"id": "ast_k000000001", "hash": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", "name": "again.mov"`+
			video+`, "sample_count": 1440000, "has_audio": true}}`,
		base,
	)
	a(
		"ok_199_assets_then_add",
		assetArg(`, "kind": "image", "width": 10, "height": 10`),
		func() *B { return withAssets(199) },
	)
	a(
		"err_validation_video_missing_frame_count",
		assetArg(`, "kind": "video", "frame_rate": {"num": 30, "den": 1}, "width": 1080, "height": 1920`),
		noAssets,
	)
	a("err_validation_sample_count_missing_with_audio", assetArg(video+`, "has_audio": true`), noAssets)
	a(
		"err_validation_bad_hash",
		`{"asset": {"id": "ast_k000000001", "hash": "md5:abc", "name": "x", "kind": "image", "width": 1, "height": 1}}`,
		noAssets,
	)
	a(
		"err_id_exists_other_hash",
		`{"asset": {"id": "`+vidAsset+`", "hash": "`+hashN(
			1000,
		)+`", "name": "x", "kind": "image", "width": 1, "height": 1}}`,
		base,
	)
	a(
		"err_limit_200_assets",
		assetArg(`, "kind": "image", "width": 10, "height": 10`),
		func() *B { return withAssets(200) },
	)
}
