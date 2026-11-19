package command

// Golden cases of the lookup table of clip.set_color and of the commands that meet a lut asset (TASK-0254).

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const (
	lutAsset  = "ast_2b3c4d5f6g"
	lutAsset2 = "ast_7h8j9k2m3n"
)

func init() {
	withLuts := func(b *B) *B {
		return b.with(func(d *timeline.Document) {
			for i, id := range []string{lutAsset, lutAsset2} {
				d.Assets[id] = timeline.Asset{
					ID: id, Kind: timeline.AssetLUT, Name: fmt.Sprintf("grade%d.cube", i+1),
					Hash: "sha256:" + strings.Repeat(fmt.Sprintf("c%d", i+1), 32),
				}
			}
		})
	}
	plain := func() *B { return withLuts(base().clip(itemA, vidTr, 0, 90, 0)) }
	graded := func(intensity float64) func() *B {
		return func() *B {
			return withLuts(base().clip(itemA, vidTr, 0, 90, 0, func(it *timeline.Item) {
				it.Lut = &timeline.Lut{Asset: lutAsset, Intensity: intensity}
			}))
		}
	}
	set := func(name, args string, b func() *B) {
		gc("clip.set_color", name, b, op("clip.set_color", fmt.Sprintf(`{"item_id": %q, %s}`, itemA, args)))
	}

	set("ok_lut_sets_the_table_at_full_intensity", fmt.Sprintf(`"lut_asset": %q`, lutAsset), plain)
	set("ok_lut_with_an_intensity", fmt.Sprintf(`"lut_asset": %q, "lut_intensity": 0.456`, lutAsset), plain)
	set("ok_lut_intensity_changes_the_current_table", `"lut_intensity": 0.25`, graded(1))
	set("ok_lut_other_table_resets_the_intensity", fmt.Sprintf(`"lut_asset": %q`, lutAsset2), graded(0.4))
	set("ok_lut_intensity_0_removes_the_table", `"lut_intensity": 0`, graded(0.4))
	set("ok_lut_with_a_colour_adjustment", fmt.Sprintf(`"saturation": 1.2, "lut_asset": %q`, lutAsset), plain)
	set(
		"ok_lut_same_values_noop",
		fmt.Sprintf(`"lut_asset": %q, "lut_intensity": 0.4`, lutAsset),
		graded(0.4),
	)
	set("err_validation_lut_intensity_without_a_table", `"lut_intensity": 0.5`, plain)
	set("err_validation_lut_intensity_above_1", fmt.Sprintf(`"lut_asset": %q, "lut_intensity": 1.5`, lutAsset), plain)
	set("err_validation_lut_asset_bad_id", `"lut_asset": "grade"`, plain)
	set("err_not_found_lut_asset", `"lut_asset": "ast_zzzzzzzzzz"`, plain)
	set("err_type_mismatch_lut_asset_is_a_video", fmt.Sprintf(`"lut_asset": %q`, vidAsset), plain)

	gc("asset.add", "ok_lut", plain, op("asset.add", `{"asset": {"id": "ast_k000000001", "hash": "sha256:`+
		strings.Repeat("0", 61)+`3ea", "name": "look.cube", "kind": "lut"}}`))
	gc(
		"asset.remove",
		"err_in_use_by_a_lut",
		graded(1),
		op("asset.remove", fmt.Sprintf(`{"asset_id": %q}`, lutAsset)),
	)
	gc("clip.add", "err_type_mismatch_lut_asset", plain, op("clip.add",
		fmt.Sprintf(`{"track_id": %q, "asset_id": %q, "start": "0s", "id": %q}`, vidTr, lutAsset, itemC)))
	gc("audio.add", "err_type_mismatch_lut_asset", plain, op("audio.add",
		fmt.Sprintf(`{"track_id": %q, "asset_id": %q, "start": "0s", "id": %q}`, audTr, lutAsset, itemC)))
}
