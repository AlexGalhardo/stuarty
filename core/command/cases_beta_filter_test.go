package command

import (
	"fmt"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	fa := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest) }
	plain := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 150, 0, mods...) }
	}
	audioItem := func() *B { return base().audio(itemB, 0, 60, 0) }
	image := func(it *timeline.Item) { it.Asset, it.SourceIn = imgAsset, 0 }

	fi := func(name, args string, b func() *B) { gc("clip.set_filter", name, b, op("clip.set_filter", args)) }
	fi("ok_filter_only", fa(`"filter": "sepia"`), plain())
	fi("ok_amount_only", fa(`"amount": 0.4`), plain(func(it *timeline.Item) { it.Filter = "invert" }))
	fi("ok_filter_and_amount", fa(`"filter": "grayscale", "amount": 0.6`), plain())
	fi("ok_each_filter", fa(`"filter": "cool"`), plain(func(it *timeline.Item) { it.Filter = "warm" }))
	fi(
		"ok_boundaries_amount_0_1",
		fa(`"filter": "warm", "amount": 0`),
		plain(func(it *timeline.Item) { it.FilterAmount = 1 }),
	)
	fi("ok_default_values_remove_fields", fa(`"filter": "none", "amount": 1`),
		plain(func(it *timeline.Item) { it.Filter, it.FilterAmount = "sepia", 0.3 }))
	fi("ok_none_keeps_amount", fa(`"filter": "none"`),
		plain(func(it *timeline.Item) { it.Filter, it.FilterAmount = "sepia", 0.3 }))
	fi("ok_rounded_to_3_decimals", fa(`"filter": "sepia", "amount": 0.12345`), plain())
	fi("ok_image_clip", fa(`"filter": "invert"`), plain(image))
	fi("ok_same_values_noop", fa(`"filter": "sepia", "amount": 0.5`),
		plain(func(it *timeline.Item) { it.Filter, it.FilterAmount = "sepia", 0.5 }))
	fi("err_validation_no_argument", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	fi("err_validation_unknown_filter", fa(`"filter": "sharpen"`), plain())
	fi("err_validation_amount_above_1", fa(`"amount": 1.5`), plain())
	fi("err_validation_amount_negative", fa(`"amount": -0.1`), plain())
	fi("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "filter": "sepia"}`, plain())
	fi("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "filter": "sepia"}`, itemB), audioItem)

	bl := func(name, args string, b func() *B) { gc("clip.set_blur", name, b, op("clip.set_blur", args)) }
	bl("ok_radius", fa(`"radius": 12.5`), plain())
	bl("ok_boundaries_0_100", fa(`"radius": 100`), plain(func(it *timeline.Item) { it.Blur = 5 }))
	bl("ok_zero_removes_field", fa(`"radius": 0`), plain(func(it *timeline.Item) { it.Blur = 5 }))
	bl("ok_rounded_to_3_decimals", fa(`"radius": 3.14159`), plain())
	bl("ok_image_clip", fa(`"radius": 8`), plain(image))
	bl("ok_same_value_noop", fa(`"radius": 5`), plain(func(it *timeline.Item) { it.Blur = 5 }))
	bl("err_validation_no_argument", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	bl("err_validation_radius_above_100", fa(`"radius": 120`), plain())
	bl("err_validation_radius_negative", fa(`"radius": -1`), plain())
	bl("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "radius": 4}`, plain())
	bl("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "radius": 4}`, itemB), audioItem)
}
