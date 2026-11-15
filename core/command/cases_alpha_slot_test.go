package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// slotted marks an item as a slot of the given name and kind.
func slotted(name, kind string, mods ...func(*timeline.Slot)) func(*timeline.Item) {
	return func(it *timeline.Item) {
		s := timeline.NewSlot(name, kind)
		for _, m := range mods {
			m(&s)
		}
		it.Slot = &s
	}
}

func placeholder(it *timeline.Item) { it.Asset = timeline.PlaceholderAsset }

func init() {
	sm := func(name, args string, b func() *B) { gc("slot.mark", name, b, op("slot.mark", args)) }
	args := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemC, rest) }
	text := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().template().text(itemC, vidTr, 0, 60, mods...) }
	}
	filled := func(s *timeline.Slot) { s.Filled = true }

	sm("ok_text_slot", args(`"name": "title", "description": "Headline"`), text())
	sm("ok_caption_slot", args(`"name": "subtitles"`), func() *B { return base().template().caption(itemC, 0, 3) })
	sm("ok_clip_slot_kind_media", args(`"name": "hero"`),
		func() *B { return base().template().clip(itemC, vidTr, 0, 60, 0) })
	sm(
		"ok_audio_slot_kind_media",
		args(`"name": "music"`),
		func() *B { return base().template().audio(itemC, 0, 60, 0) },
	)
	sm("ok_media_slot_stretch_timeline", args(`"name": "hero", "duration_policy": "stretch_timeline"`),
		func() *B { return base().template().clip(itemC, vidTr, 0, 60, 0) })
	sm("ok_required_false", args(`"name": "title", "required": false`), text())
	sm("ok_remark_same_name_updates_fields_keeps_filled", args(`"name": "title", "description": "New"`),
		text(slotted("title", "text", filled, func(s *timeline.Slot) { s.Description, s.Required = "Old", false })))
	sm("ok_name_40_chars", args(`"name": "`+strings.Repeat("a", 40)+`"`), text())
	sm("err_validation_uppercase_name", args(`"name": "Title"`), text())
	sm("err_validation_name_41_chars", args(`"name": "`+strings.Repeat("a", 41)+`"`), text())
	sm(
		"err_validation_description_201_chars",
		args(`"name": "title", "description": "`+strings.Repeat("d", 201)+`"`),
		text(),
	)
	sm("err_validation_duration_policy_on_text_slot", args(`"name": "title", "duration_policy": "fit"`), text())
	sm("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "name": "title"}`, text())
	sm("err_type_mismatch_shape_item", args(`"name": "title"`),
		func() *B { return base().template().shape(itemC, vidTr, 0, 60) })
	sm("err_slot_name_used_by_other_item", args(`"name": "title"`), func() *B {
		return text()().text(itemB, vidTr, 100, 30, slotted("title", "text"))
	})
	sm("err_invalid_state_item_already_has_other_slot", args(`"name": "title"`),
		text(slotted("headline", "text")))
}

func init() {
	su := func(name, args string, b func() *B) { gc("slot.unmark", name, b, op("slot.unmark", args)) }
	args := fmt.Sprintf(`{"item_id": %q}`, itemC)

	su("ok_unmark_text_slot", args, func() *B {
		return base().template().text(itemC, vidTr, 0, 60, slotted("title", "text"))
	})
	su("ok_unmark_filled_media_slot", args, func() *B {
		return base().template().
			clip(itemC, vidTr, 0, 60, 0, slotted("hero", "media", func(s *timeline.Slot) { s.Filled = true }))
	})
	su("ok_unmark_unfilled_text_slot", args, func() *B {
		return base().template().text(itemC, vidTr, 0, 60, slotted("title", "text"))
	})
	su(
		"err_not_found",
		`{"item_id": "itm_zzzzzzzzzz"}`,
		func() *B { return base().template().text(itemC, vidTr, 0, 60) },
	)
	su("err_slot_item_is_not_a_slot", args, func() *B { return base().template().text(itemC, vidTr, 0, 60) })
	su("err_invalid_state_unfilled_media_slot_with_placeholder", args, func() *B {
		return base().template().clip(itemC, vidTr, 0, 90, 0, placeholder, slotted("hero", "media"))
	})
}

func init() {
	sf := func(name, args string, b func() *B) { gc("slot.fill", name, b, op("slot.fill", args)) }
	media := func(rest string) string { return `{"name": "hero", ` + rest + `}` }
	vid := `"asset_id": "` + vidAsset + `"`
	// A template with a media slot "hero": a clip of the placeholder asset, 90 frames from frame 0.
	hero := func(policy string, mods ...func(*timeline.Item)) func() *B {
		return func() *B {
			all := append([]func(*timeline.Item){placeholder, slotted("hero", "media", func(s *timeline.Slot) {
				s.Description, s.DurationPolicy = "Main footage", policy
			})}, mods...)
			return base().template().clip(itemA, vidTr, 0, 90, 0, all...)
		}
	}
	withMore := func(h func() *B, more func(*B)) func() *B {
		return func() *B {
			b := h()
			more(b)
			return b
		}
	}
	textSlot := func() *B {
		return base().template().text(itemC, vidTr, 0, 60, slotted("title", "text", func(s *timeline.Slot) {
			s.Required = true
		}))
	}
	captionSlot := func(mods ...func(*B)) func() *B {
		return func() *B {
			b := base().template().caption(itemC, 0, 1, func(it *timeline.Item) {
				slotted("subs", "caption")(it)
				it.Words, it.Duration = []timeline.Word{{Text: "Title", Start: 0, Duration: 30}}, 30
			})
			for _, m := range mods {
				m(b)
			}
			return b
		}
	}
	words := func(n, dur int) string { return `{"name": "subs", "words": ` + wordArgs(n, dur) + `}` }

	sf("ok_media_fit_asset_long_enough", media(vid), hero("fit"))
	sf("ok_media_fit_asset_exactly_long_enough", media(vid+`, "source_in": "810f"`), hero("fit"))
	sf("ok_media_fit_with_source_in", media(vid+`, "source_in": "2s"`), hero("fit"))
	sf("ok_media_fit_image_asset_no_source_check", media(`"asset_id": "`+imgAsset+`"`), hero("fit"))
	sf("ok_media_fit_speed_3_2", media(vid+`, "source_in": "100f"`), hero("fit", func(it *timeline.Item) {
		it.Speed, it.Duration = timeline.Rational{Num: 3, Den: 2}, 60
	}))
	more := func(b *B) {
		b.text(itemC, capTr, 90, 60)
		b.audio(itemB, 120, 30, 0)
		b.text("itm_p7q8r9s0t1", capTr, 30, 20)
	}
	sf("ok_media_stretch_longer_shifts_later_items_on_all_tracks", media(vid+`, "source_in": "1s"`),
		withMore(hero("stretch_timeline"), more))
	sf("ok_media_stretch_shorter_shifts_earlier_and_cuts_keyframes", media(vid+`, "source_in": "850f"`),
		withMore(hero("stretch_timeline", func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{"opacity": {kf(0, 1), kf(80, 0.5)}}
		}), func(b *B) { b.text(itemC, capTr, 90, 60) }))
	sf("ok_media_stretch_image_keeps_duration", media(`"asset_id": "`+imgAsset+`"`),
		withMore(hero("stretch_timeline"), func(b *B) { b.text(itemC, capTr, 90, 60) }))
	sf("ok_media_audio_slot_with_video_asset_with_audio", `{"name": "voice", `+vid+`}`, func() *B {
		return base().template().audio(itemB, 0, 60, 0, placeholder, slotted("voice", "media"))
	})
	sf("ok_text_slot", `{"name": "title", "text": "Hello"}`, textSlot)
	sf("ok_caption_slot_duration_follows_last_word", words(2, 12), captionSlot())
	sf("ok_other_slots_untouched", `{"name": "title", "text": "Hello"}`, func() *B {
		return textSlot().with(func(d *timeline.Document) {
			it := timeline.NewItem(itemB, timeline.ItemText, vidTr, 100, 30)
			it.Text = "Other"
			slotted("other", "text")(&it)
			d.Items[itemB] = it
		})
	})
	sf("err_validation_none_of_three", `{"name": "hero"}`, hero("fit"))
	sf("err_validation_two_of_three", media(vid+`, "text": "x"`), hero("fit"))
	sf("err_validation_source_in_without_asset_id", `{"name": "title", "text": "x", "source_in": "1s"}`, textSlot)
	sf("err_invalid_time_source_in_no_unit", media(vid+`, "source_in": "30"`), hero("fit"))
	sf("err_invalid_time_stretch_past_108000", media(vid), func() *B {
		return base().template().clip(itemA, vidTr, 107900, 90, 0, placeholder,
			slotted("hero", "media", func(s *timeline.Slot) { s.DurationPolicy = "stretch_timeline" }))
	})
	sf("err_slot_unknown_name", `{"name": "nope", "text": "x"}`, hero("fit"))
	sf("err_slot_wrong_kind_text_for_media", `{"name": "hero", "text": "x"}`, hero("fit"))
	sf("err_slot_wrong_kind_asset_for_text", `{"name": "title", `+vid+`}`, textSlot)
	sf("err_slot_already_filled", media(vid), hero("fit", func(it *timeline.Item) { it.Slot.Filled = true }))
	sf("err_not_found_asset", media(`"asset_id": "ast_zzzzzzzzzz"`), hero("fit"))
	sf("err_not_found_placeholder_asset", media(`"asset_id": "ast_placeholder"`), hero("fit"))
	sf("err_type_mismatch_image_for_audio_slot", `{"name": "voice", "asset_id": "`+imgAsset+`"}`, func() *B {
		return base().template().audio(itemB, 0, 60, 0, placeholder, slotted("voice", "media"))
	})
	sf("err_type_mismatch_audio_asset_for_clip_slot", media(`"asset_id": "`+audAsset+`"`), hero("fit"))
	sf("err_out_of_source_fit_asset_too_short", media(vid+`, "source_in": "850f"`), hero("fit"))
	sf("err_out_of_source_stretch_source_in_at_asset_end", media(vid+`, "source_in": "900f"`), hero("stretch_timeline"))
	sf("err_overlap_stretch_shorter_shift_collides", media(vid+`, "source_in": "850f"`),
		withMore(hero("stretch_timeline"), func(b *B) {
			b.text(itemC, capTr, 90, 30)
			b.text("itm_p7q8r9s0t1", capTr, 30, 40)
		}))
	sf("err_overlap_caption_words_longer_than_gap", words(5, 12), captionSlot(func(b *B) { b.caption(itemB, 40, 1) }))
	sf("err_limit_61_caption_words", words(61, 1), captionSlot())
	sf("err_limit_5000_caption_words", words(30, 2), captionSlot(func(b *B) {
		for i := range 83 {
			b.caption(fmt.Sprintf("itm_k%09d", i), 1000+i*700, 60)
		}
	}))
	sf("err_invalid_state_fades_no_longer_fit", media(vid+`, "source_in": "850f"`),
		hero("stretch_timeline", func(it *timeline.Item) { it.FadeIn, it.FadeOut = 40, 40 }))
}

func init() {
	ti := func(name, args string, b func() *B) { gc("template.set_info", name, b, op("template.set_info", args)) }
	withTpl := func(colors map[string]timeline.Color, mods ...func(*B)) func() *B {
		return func() *B {
			b := base().template()
			b.d.Template = &timeline.TemplateInfo{Description: "", Colors: colors}
			for _, m := range mods {
				m(b)
			}
			return b
		}
	}
	none := base

	ti("ok_instance_flag_kept", `{"colors": {"brand": "#ff3366"}}`, func() *B {
		b := base().template()
		b.d.Template.Instance = true

		return b
	})
	ti("ok_description_only", `{"description": "Product teaser"}`, withTpl(map[string]timeline.Color{"old": "#112233"}))
	ti("ok_add_colors_creates_template_object", `{"colors": {"brand": "#ff3366"}}`, none)
	ti(
		"ok_replace_colour_value",
		`{"colors": {"old": "#445566"}}`,
		withTpl(map[string]timeline.Color{"old": "#112233"}),
	)
	ti("ok_colour_uppercased", `{"colors": {"brand": "#aabbccdd"}}`, withTpl(nil))
	ti("ok_remove_unused_colour", `{"colors": {"old": null}}`, withTpl(map[string]timeline.Color{"old": "#112233"}))
	ti("ok_remove_absent_name_noop", `{"colors": {"ghost": null}}`, none)
	ti("ok_description_500_chars", `{"description": "`+strings.Repeat("d", 500)+`"}`, withTpl(nil))
	ti("ok_same_values_noop", `{"description": "Same", "colors": {"old": "#112233"}}`, func() *B {
		b := withTpl(map[string]timeline.Color{"old": "#112233"})()
		b.d.Template.Description = "Same"
		return b
	})
	ti("err_validation_nothing_to_set", `{}`, withTpl(nil))
	ti("err_validation_bad_colour_name", `{"colors": {"Brand": "#ff3366"}}`, withTpl(nil))
	ti("err_validation_bad_colour_value", `{"colors": {"brand": "red"}}`, withTpl(nil))
	ti("err_validation_description_501_chars", `{"description": "`+strings.Repeat("d", 501)+`"}`, withTpl(nil))
	brand := map[string]timeline.Color{"brand": "#FF3366"}
	ti("err_in_use_text_style", `{"colors": {"brand": null}}`, withTpl(brand, func(b *B) {
		b.text(itemC, vidTr, 0, 60, func(it *timeline.Item) { it.Style = styleOf(map[string]any{"color": "$brand"}) })
	}))
	ti("err_in_use_track_caption_style", `{"colors": {"brand": null}}`, withTpl(brand, func(b *B) {
		t := b.d.Tracks[capTr]
		t.CaptionStyle = styleOf(map[string]any{"highlight_color": "$brand"})
		b.d.Tracks[capTr] = t
	}))
	ti("err_in_use_settings_background", `{"colors": {"brand": null}}`, withTpl(brand, func(b *B) {
		b.d.Settings.Background = "$brand"
	}))
}
