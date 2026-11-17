package command

// Golden cases of clip.set_shadow and clip.set_outline (VV-122).

import (
	"fmt"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	plain := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 90, 0, mods...) }
	}
	image := func(it *timeline.Item) { it.Asset, it.SourceIn = imgAsset, 0 }
	text := func() *B { return base().text(itemA, vidTr, 0, 90) }
	fa := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest) }

	shadowed := func(it *timeline.Item) {
		it.Shadow = &timeline.Shadow{Color: "#000000", Opacity: 0.5, DX: 8, DY: 8, Blur: 12}
	}
	sh := func(name, args string, b func() *B) { gc("clip.set_shadow", name, b, op("clip.set_shadow", args)) }
	sh("ok_defaults", fa(`"shadow": {}`), plain())
	sh("ok_full", fa(`"shadow": {"color": "#ff8800", "opacity": 0.8, "dx": -10, "dy": 14.5, "blur": 30}`), plain())
	sh("ok_replaces_shadow", fa(`"shadow": {"opacity": 1}`), plain(shadowed))
	sh("ok_null_removes_shadow", fa(`"shadow": null`), plain(shadowed))
	sh("ok_null_without_shadow_noop", fa(`"shadow": null`), plain())
	sh("ok_same_shadow_noop", fa(`"shadow": {}`), plain(shadowed))
	sh("ok_image_clip", fa(`"shadow": {"blur": 0}`), plain(image))
	sh("err_validation_no_shadow", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	sh("err_validation_opacity_above_1", fa(`"shadow": {"opacity": 1.5}`), plain())
	sh("err_validation_blur_above_100", fa(`"shadow": {"blur": 101}`), plain())
	sh("err_validation_bad_colour", fa(`"shadow": {"color": "red"}`), plain())
	sh("err_validation_unknown_template_colour", fa(`"shadow": {"color": "$brand"}`), plain())
	sh("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "shadow": {}}`, plain())
	sh("err_type_mismatch_text_item", fa(`"shadow": {}`), text)

	outlined := func(it *timeline.Item) { it.Outline = &timeline.Outline{Color: "#000000", Width: 4} }
	ol := func(name, args string, b func() *B) { gc("clip.set_outline", name, b, op("clip.set_outline", args)) }
	ol("ok_defaults", fa(`"outline": {}`), plain())
	ol("ok_full", fa(`"outline": {"color": "#ffffff", "width": 6.5}`), plain())
	ol("ok_replaces_outline", fa(`"outline": {"width": 10}`), plain(outlined))
	ol("ok_null_removes_outline", fa(`"outline": null`), plain(outlined))
	ol("ok_null_without_outline_noop", fa(`"outline": null`), plain())
	ol("ok_same_outline_noop", fa(`"outline": {}`), plain(outlined))
	ol("ok_image_clip", fa(`"outline": {"width": 2}`), plain(image))
	ol("err_validation_no_outline", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	ol("err_validation_width_below_half", fa(`"outline": {"width": 0.1}`), plain())
	ol("err_validation_width_above_100", fa(`"outline": {"width": 100.5}`), plain())
	ol("err_validation_bad_colour", fa(`"outline": {"color": 5}`), plain())
	ol("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "outline": {}}`, plain())
	ol("err_type_mismatch_text_item", fa(`"outline": {}`), text)
}
