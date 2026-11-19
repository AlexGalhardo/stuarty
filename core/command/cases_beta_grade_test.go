package command

// Golden cases of the advanced colour of clip.set_color: HSL bands, wheels and curves (TASK-0254).

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	plain := func() *B { return base().clip(itemA, vidTr, 0, 90, 0) }
	graded := func() *B {
		return base().clip(itemA, vidTr, 0, 90, 0, func(it *timeline.Item) {
			it.Hsl = []timeline.HslBand{{Range: "red", Hue: 0.1}, {Range: "blue", Saturation: -0.5}}
			it.Wheels = &timeline.Wheels{Shadows: &timeline.Wheel{Hue: 240, Amount: 0.3, Luma: 0}}
			it.Curves = &timeline.Curves{Master: []timeline.CurvePoint{{X: 0, Y: 0}, {X: 0.5, Y: 0.4}, {X: 1, Y: 1}}}
		})
	}
	set := func(name, args string, b func() *B) {
		gc("clip.set_color", name, b, op("clip.set_color", fmt.Sprintf(`{"item_id": %q, %s}`, itemA, args)))
	}

	set(
		"ok_hsl_bands_in_hue_order",
		`"hsl": [{"range": "blue", "hue": -0.2}, {"range": "red", "saturation": 0.4}]`,
		plain,
	)
	set("ok_hsl_replaces_the_band_of_the_range", `"hsl": [{"range": "red", "lightness": 0.25}]`, graded)
	set("ok_hsl_neutral_band_removes_it", `"hsl": [{"range": "red"}]`, graded)
	set("ok_wheels_set_midtones", `"wheels": {"midtones": {"hue": 30.1234, "amount": 0.5, "luma": -0.2}}`, plain)
	set("ok_wheels_neutral_wheel_removes_it", `"wheels": {"shadows": {"hue": 10}}`, graded)
	set(
		"ok_curves_set_red_and_blue",
		`"curves": {"red": [{"x": 0, "y": 0.1}, {"x": 1, "y": 0.9}], "blue": [{"x": 0.2, "y": 0.2}, {"x": 0.8, "y": 0.7}]}`,
		plain,
	)
	set("ok_curves_empty_list_removes_the_channel", `"curves": {"master": []}`, graded)
	set("ok_curves_identity_removes_the_channel", `"curves": {"master": [{"x": 0, "y": 0}, {"x": 1, "y": 1}]}`, graded)
	set(
		"ok_grade_with_a_colour_adjustment",
		`"contrast": 1.1, "wheels": {"highlights": {"hue": 60, "amount": 0.2}}`,
		plain,
	)
	set("err_validation_hsl_range_twice", `"hsl": [{"range": "red", "hue": 0.1}, {"range": "red", "hue": 0.2}]`, plain)
	set("err_validation_hsl_unknown_range", `"hsl": [{"range": "pink", "hue": 0.1}]`, plain)
	set("err_validation_hsl_hue_above_1", `"hsl": [{"range": "red", "hue": 1.5}]`, plain)
	set("err_validation_wheel_hue_above_360", `"wheels": {"shadows": {"hue": 400, "amount": 0.1}}`, plain)
	set("err_validation_curve_one_point", `"curves": {"master": [{"x": 0.5, "y": 0.5}]}`, plain)
	set(
		"err_validation_curve_x_not_increasing",
		`"curves": {"green": [{"x": 0.5, "y": 0.1}, {"x": 0.5, "y": 0.9}]}`,
		plain,
	)
	set("err_validation_curve_nine_points", `"curves": {"master": [`+curveOf(9)+`]}`, plain)
	set("err_validation_curve_y_above_1", `"curves": {"red": [{"x": 0, "y": 0}, {"x": 1, "y": 1.2}]}`, plain)
}

func curveOf(n int) string {
	parts := make([]string, n)
	for i := range n {
		parts[i] = fmt.Sprintf(`{"x": %v, "y": %v}`, float64(i)/10, float64(i)/10)
	}
	return strings.Join(parts, ", ")
}
