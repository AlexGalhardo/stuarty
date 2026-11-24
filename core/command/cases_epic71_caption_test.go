package command

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Golden cases of caption.apply_style and caption.list_styles (SDD-002 C.88, C.89).

func init() {
	apply := func(name, args string, b func() *B) { gc("caption.apply_style", name, b, op("caption.apply_style", args)) }
	args := func(track, style, rest string) string {
		s := fmt.Sprintf(`{"track_id": %q, "style": %q`, track, style)
		if rest != "" {
			s += ", " + rest
		}
		return s + "}"
	}
	withCaption := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().caption(itemC, 0, 4, mods...) }
	}

	for _, id := range CaptionStyleIDs() {
		apply("ok_style_"+id, args(capTr, id, ""), withCaption())
	}
	apply("ok_replaces_the_track_style", args(capTr, "bold_pop", ""), func() *B {
		return base().with(func(d *timeline.Document) {
			t := d.Tracks[capTr]
			t.CaptionStyle = &timeline.CaptionStyle{Position: ptr("top"), TextStyle: timeline.TextStyle{Size: ptr(30.0), Align: ptr("left")}}
			d.Tracks[capTr] = t
		})
	})
	apply("ok_scales_with_a_wide_project", args(capTr, "thick_outline", ""), func() *B {
		return base().with(func(d *timeline.Document) { d.Settings.Width, d.Settings.Height = 1280, 720 })
	})
	override := func(it *timeline.Item) { it.Style = &timeline.CaptionStyle{Position: ptr("top")} }
	apply("ok_reports_item_overrides", args(capTr, "soft_box", ""), withCaption(override))
	apply("ok_reset_overrides", args(capTr, "soft_box", `"reset_overrides": true`), withCaption(override))
	apply("ok_same_style_noop", args(capTr, "clean_white", ""), func() *B {
		return base().with(func(d *timeline.Document) {
			spec, _ := CaptionStyleByID("clean_white")
			t := d.Tracks[capTr]
			t.CaptionStyle = ResolveCaptionStyle(spec, d.Settings.Width, d.Settings.Height)
			d.Tracks[capTr] = t
		})
	})
	apply("err_font_unknown_weight_of_a_project_font", args(capTr, "clean_white", ""), func() *B {
		return base().with(func(d *timeline.Document) {
			d.Fonts = map[string]timeline.ProjectFont{"Inter": {Files: []timeline.FontFile{{Weight: 400, Hash: hashN(7)}}}}
		})
	})
	apply("err_validation_unknown_style", args(capTr, "fancy", ""), withCaption())
	apply("err_not_found_track", args("trk_zzzzzzzzzz", "bold_pop", ""), withCaption())
	apply("err_type_mismatch_audio_track", args(audTr, "bold_pop", ""), withCaption())

	gc("caption.list_styles", "ok_lists_the_styles", withCaption(), op("caption.list_styles", `{}`))
	gc("caption.list_styles", "err_validation_takes_no_arguments", withCaption(),
		op("caption.list_styles", `{"style": "bold_pop"}`))
}
