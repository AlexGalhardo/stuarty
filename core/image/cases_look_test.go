package image

import "fmt"

// Golden cases of the look commands (SDD-021 section 3.1, epic 29).

func withAdjust() *Document {
	d := baseDoc()
	d.Layers[1].Adjust = &Adjust{Brightness: 10, Contrast: 20, Levels: &Levels{Black: 10, White: 240, Gamma: 120}}

	return d
}

// withLUT has the photo graded by a table at 80 per cent, with a brightness next to it.
func withLUT() *Document {
	d := baseDoc()
	d.Layers[1].Adjust = &Adjust{Brightness: 10, LUT: &LUTRef{Asset: hashCube, Intensity: 80}}
	d.Assets[hashCube] = AssetRef{Hash: hashCube, Name: "warm.cube", Kind: AssetLUT}

	return d
}

// withFullAssets holds the 64 files a document may list: the photo and 63 images, each shown by a layer of its own
// (a file no layer uses would leave the table with the batch, and would not hold the ceiling).
func withFullAssets() *Document {
	return fillAssets(baseDoc())
}

// fillAssets adds image layers, each with a file of its own, until the document lists the 64 files it may.
func fillAssets(d *Document) *Document {
	for i := 0; len(d.Assets) < MaxAssets; i++ {
		h := fmt.Sprintf("sha256:%064x", i+1)
		d.Assets[h] = AssetRef{Hash: h, Name: "x.png", Width: 1, Height: 1}
		d.Layers = append(d.Layers, Layer{
			ID: fmt.Sprintf("lyr_%010d", i), Kind: KindImage, Name: "x", Visible: true, Opacity: 100, W: 1, H: 1,
			Asset: h,
		})
	}

	ReorderAll(d)

	return d
}

// withFullTables has the 32 lookup tables a document may hold, each graded on a copy of the photo of its own.
func withFullTables() *Document {
	d := baseDoc()

	for i := range MaxLUTs {
		h := fmt.Sprintf("sha256:%064x", i+1)
		d.Assets[h] = AssetRef{Hash: h, Name: "x.cube", Kind: AssetLUT}
		d.Layers = append(d.Layers, Layer{
			ID: fmt.Sprintf("lyr_%010d", i), Kind: KindImage, Name: "x", Visible: true, Opacity: 100, W: 1, H: 1,
			Asset: hashFace, Adjust: &Adjust{LUT: &LUTRef{Asset: h, Intensity: MaxLUTIntensity}},
		})
	}

	ReorderAll(d)

	return d
}

func withFilters() *Document {
	d := baseDoc()
	f1, f2, f3 := NewFilter(FilterBlur), NewFilter(FilterShadow), NewFilter(FilterVignette)
	d.Layers[1].Filters = []Filter{f1, f2, f3}

	return d
}

func withEightFilters() *Document {
	d := baseDoc()

	for _, k := range []string{
		FilterGrayscale, FilterBlur, FilterSharpen, FilterVignette, FilterGrain, FilterShadow, FilterGlow, FilterStroke,
	} {
		d.Layers[1].Filters = append(d.Layers[1].Filters, NewFilter(k))
	}

	return d
}

func withMask() *Document {
	d := baseDoc()
	d.Layers[1].Mask = &Mask{Kind: MaskEllipse, X: 100, Y: 100, W: 800, H: 800, Feather: 12}

	return d
}

// withLayerMask has the photo masked by the alpha of the bar, and the bar a plain layer.
func withLayerMask() *Document {
	d := baseDoc()
	d.Layers[1].Mask = &Mask{Kind: MaskLayer, Source: lid('d')}

	return d
}

func withBlend() *Document {
	d := baseDoc()
	d.Layers[3].Blend = "multiply"

	return d
}

func init() {
	photo, bar, group, locked, none := lid('b'), lid('d'), lid('e'), lid('h'), lid('z')

	arg := func(id, rest string) string { return fmt.Sprintf(`{"layer_id": %q, %s}`, id, rest) }

	only := func(id string) string { return fmt.Sprintf(`{"layer_id": %q}`, id) }

	addCases(
		gc(
			"layer.adjust",
			"colour",
			arg(photo, `"brightness": 10, "contrast": 20, "saturation": -30, "temperature": 5, "tint": -5, "hue": 15`),
		),
		gc(
			"layer.adjust",
			"exposure_levels_curve",
			arg(
				photo,
				`"exposure": 50, "levels": {"black": 10, "white": 240, "gamma": 120}, "curve": [{"x": 64, "y": 48}, {"x": 192, "y": 208}]`,
			),
		),
		gc(
			"layer.adjust",
			"merges_with_the_stored",
			arg(photo, `"saturation": 40, "levels": {"gamma": 150}`),
		).on(withAdjust),
		gc(
			"layer.adjust",
			"neutral_values_are_removed",
			arg(photo, `"brightness": 0, "contrast": 0, "levels": {"black": 0, "white": 255, "gamma": 100}`),
		).on(withAdjust),
		gc("layer.adjust", "empty_curve_removes_it", arg(photo, `"curve": []`)),
		gc("layer.adjust", "reset", arg(photo, `"reset": true`)).on(withAdjust),
		gc("layer.adjust", "on_a_text_layer", arg(lid('c'), `"brightness": -20`)),
		gc("layer.adjust", "nothing_given", arg(photo, `"hue": null`)),
		gc("layer.adjust", "value_out_of_range", arg(photo, `"brightness": 101`)),
		gc("layer.adjust", "white_not_above_black", arg(photo, `"levels": {"black": 200, "white": 100}`)),
		gc("layer.adjust", "curve_x_not_increasing", arg(photo, `"curve": [{"x": 100, "y": 10}, {"x": 100, "y": 20}]`)),
		gc("layer.adjust", "reset_with_values", arg(photo, `"reset": true, "hue": 3`)),
		gc("layer.adjust", "missing", arg(none, `"hue": 3`)),
		gc("layer.adjust", "group", arg(group, `"hue": 3`)),
		gc("layer.adjust", "locked", arg(locked, `"hue": 3`)),
		gc("layer.adjust", "lut_full_intensity", arg(photo, fmt.Sprintf(`"lut": {"asset": %q}`, hashCube))),
		gc("layer.adjust", "lut_half", arg(photo, fmt.Sprintf(`"lut": {"asset": %q, "intensity": 50}`, hashCube))),
		gc(
			"layer.adjust",
			"lut_next_to_the_colour_keys",
			arg(photo, fmt.Sprintf(`"saturation": 20, "lut": {"asset": %q}`, hashCube)),
		),
		gc("layer.adjust", "lut_intensity_only", arg(photo, `"lut": {"intensity": 30}`)).on(withLUT),
		gc(
			"layer.adjust",
			"lut_another_table_keeps_the_intensity",
			arg(photo, fmt.Sprintf(`"lut": {"asset": %q}`, hashCube2)),
		).on(withLUT),
		gc("layer.adjust", "lut_zero_removes_it", arg(photo, `"lut": {"intensity": 0}`)).on(withLUT),
		gc("layer.adjust", "reset_removes_the_lut_and_its_asset", arg(photo, `"reset": true`)).on(withLUT),
		gc("layer.adjust", "lut_an_image_is_not_a_table", arg(photo, fmt.Sprintf(`"lut": {"asset": %q}`, hashFace))),
		gc("layer.adjust", "lut_unknown_file", arg(photo, fmt.Sprintf(`"lut": {"asset": %q}`, hashNone))),
		gc("layer.adjust", "lut_not_a_hash", arg(photo, `"lut": {"asset": "sha256:abc"}`)),
		gc(
			"layer.adjust",
			"lut_intensity_out_of_range",
			arg(photo, fmt.Sprintf(`"lut": {"asset": %q, "intensity": 101}`, hashCube)),
		),
		gc("layer.adjust", "lut_intensity_without_a_table", arg(photo, `"lut": {"intensity": 40}`)),
		gc("layer.adjust", "lut_with_nothing", arg(photo, `"lut": {}`)),
		gc("layer.adjust", "lut_with_reset", arg(photo, fmt.Sprintf(`"reset": true, "lut": {"asset": %q}`, hashCube))),
		gc("layer.adjust", "lut_on_a_group", arg(group, fmt.Sprintf(`"lut": {"asset": %q}`, hashCube))),
		gc("layer.adjust", "lut_on_a_locked_layer", arg(locked, fmt.Sprintf(`"lut": {"asset": %q}`, hashCube))),
		gc(
			"layer.adjust",
			"lut_past_the_limit_of_files",
			arg(photo, fmt.Sprintf(`"lut": {"asset": %q}`, hashCube)),
		).on(withFullAssets),
		gc(
			"layer.adjust",
			"lut_past_the_limit_of_tables",
			arg(photo, fmt.Sprintf(`"lut": {"asset": %q}`, hashCube)),
		).on(withFullTables),
		gc("layer.filter_add", "blur_defaults", arg(photo, `"kind": "blur"`)),
		gc(
			"layer.filter_add",
			"shadow_with_colour_at_the_front",
			arg(photo, `"kind": "shadow", "color": "#112233", "dx": -6, "index": 0`),
		).on(withFilters),
		gc("layer.filter_add", "grain", arg(photo, `"kind": "grain", "amount": 60, "size": 3, "seed": 7`)),
		gc("layer.filter_add", "colour_filter", arg(photo, `"kind": "sepia", "amount": 70`)),
		gc("layer.filter_add", "sharpen_glow_stroke", arg(photo, `"kind": "glow", "radius": 20, "color": "#FFAA00"`)),
		gc("layer.filter_add", "parameter_of_another_kind", arg(photo, `"kind": "sepia", "radius": 4`)),
		gc("layer.filter_add", "radius_out_of_range_for_the_kind", arg(photo, `"kind": "sharpen", "radius": 50`)),
		gc("layer.filter_add", "colour_on_a_kind_without", arg(photo, `"kind": "blur", "color": "#FFFFFF"`)),
		gc("layer.filter_add", "index_past_the_end", arg(photo, `"kind": "grain", "index": 4`)).on(withFilters),
		gc("layer.filter_add", "kind_already_there", arg(photo, `"kind": "blur"`)).on(withFilters),
		gc("layer.filter_add", "ninth_filter", arg(photo, `"kind": "sepia"`)).on(withEightFilters),
		gc("layer.filter_add", "missing", arg(none, `"kind": "blur"`)),
		gc("layer.filter_add", "group", arg(group, `"kind": "blur"`)),
		gc("layer.filter_add", "locked", arg(locked, `"kind": "blur"`)),
		gc("layer.filter_set", "radius", arg(photo, `"kind": "blur", "radius": 30`)).on(withFilters),
		gc(
			"layer.filter_set",
			"shadow_colour_and_offset",
			arg(photo, `"kind": "shadow", "color": "#FF0000", "dx": 0, "dy": 10`),
		).on(withFilters),
		gc("layer.filter_set", "nothing_given", arg(photo, `"kind": "blur"`)).on(withFilters),
		gc("layer.filter_set", "no_such_filter", arg(photo, `"kind": "grain", "amount": 10`)).on(withFilters),
		gc("layer.filter_set", "parameter_of_another_kind", arg(photo, `"kind": "blur", "amount": 10`)).on(withFilters),
		gc("layer.filter_set", "group", arg(group, `"kind": "blur", "radius": 3`)),
		gc("layer.filter_set", "locked", arg(locked, `"kind": "blur", "radius": 3`)),
		gc("layer.filter_remove", "middle", arg(photo, `"kind": "shadow"`)).on(withFilters),
		gc("layer.filter_remove", "last_one_leaves_no_list", arg(photo, `"kind": "blur"`)).on(func() *Document {
			d := baseDoc()
			d.Layers[1].Filters = []Filter{NewFilter(FilterBlur)}

			return d
		}),
		gc("layer.filter_remove", "no_such_filter", arg(photo, `"kind": "grain"`)).on(withFilters),
		gc("layer.filter_remove", "unknown_kind", arg(photo, `"kind": "sparkle"`)).on(withFilters),
		gc("layer.filter_remove", "group", arg(group, `"kind": "blur"`)),
		gc("layer.filter_remove", "locked", arg(locked, `"kind": "blur"`)),
		gc("layer.filter_move", "to_the_front", arg(photo, `"kind": "vignette", "index": 0`)).on(withFilters),
		gc("layer.filter_move", "to_the_end", arg(photo, `"kind": "blur", "index": 2`)).on(withFilters),
		gc("layer.filter_move", "index_past_the_end", arg(photo, `"kind": "blur", "index": 3`)).on(withFilters),
		gc("layer.filter_move", "no_such_filter", arg(photo, `"kind": "grain", "index": 0`)).on(withFilters),
		gc("layer.filter_move", "group", arg(group, `"kind": "blur", "index": 0`)),
		gc("layer.filter_move", "locked", arg(locked, `"kind": "blur", "index": 0`)),
		gc("layer.mask_set", "rect_default_is_the_whole_box", arg(photo, `"kind": "rect"`)),
		gc(
			"layer.mask_set",
			"rounded",
			arg(photo, `"kind": "rounded_rect", "x": 50, "y": 50, "w": 900, "h": 900, "radius": 40, "feather": 4`),
		),
		gc("layer.mask_set", "rounded_default_radius", arg(photo, `"kind": "rounded_rect"`)),
		gc(
			"layer.mask_set",
			"ellipse_inverted",
			arg(photo, `"kind": "ellipse", "w": 500, "h": 500, "invert": true`),
		).on(withMask),
		gc("layer.mask_set", "from_a_layer", arg(photo, fmt.Sprintf(`"kind": "layer", "source": %q`, bar))),
		gc(
			"layer.mask_set",
			"from_a_layer_inverted",
			arg(photo, fmt.Sprintf(`"kind": "layer", "source": %q, "invert": true`, bar)),
		),
		gc("layer.mask_set", "radius_on_a_rect", arg(photo, `"kind": "rect", "radius": 8`)),
		gc("layer.mask_set", "rectangle_leaves_the_box", arg(photo, `"kind": "rect", "x": 600, "w": 600`)),
		gc(
			"layer.mask_set",
			"layer_mask_with_a_shape_member",
			arg(photo, fmt.Sprintf(`"kind": "layer", "source": %q, "feather": 3`, bar)),
		),
		gc("layer.mask_set", "layer_mask_without_source", arg(photo, `"kind": "layer"`)),
		gc("layer.mask_set", "source_on_a_shape", arg(photo, fmt.Sprintf(`"kind": "rect", "source": %q`, bar))),
		gc("layer.mask_set", "source_is_itself", arg(photo, fmt.Sprintf(`"kind": "layer", "source": %q`, photo))),
		gc("layer.mask_set", "source_missing", arg(photo, fmt.Sprintf(`"kind": "layer", "source": %q`, none))),
		gc("layer.mask_set", "source_is_a_group", arg(photo, fmt.Sprintf(`"kind": "layer", "source": %q`, group))),
		gc(
			"layer.mask_set",
			"source_has_a_layer_mask",
			arg(bar, fmt.Sprintf(`"kind": "layer", "source": %q`, photo)),
		).on(withLayerMask),
		gc(
			"layer.mask_set",
			"layer_is_a_source_already",
			arg(bar, fmt.Sprintf(`"kind": "layer", "source": %q`, lid('c'))),
		).on(withLayerMask),
		gc("layer.mask_set", "missing", arg(none, `"kind": "rect"`)),
		gc("layer.mask_set", "group", arg(group, `"kind": "rect"`)),
		gc("layer.mask_set", "locked", arg(locked, `"kind": "rect"`)),
		gc("layer.mask_clear", "shape", only(photo)).on(withMask),
		gc("layer.mask_clear", "missing", only(none)),
		gc("layer.mask_clear", "group", only(group)),
		gc("layer.mask_clear", "locked", only(locked)),
		gc("layer.blend", "soft_light", arg(bar, `"mode": "soft_light"`)),
		gc("layer.blend", "back_to_normal_removes_it", arg(bar, `"mode": "normal"`)).on(withBlend),
		gc("layer.blend", "not_a_mode", arg(bar, `"mode": "add"`)),
		gc("layer.blend", "missing", arg(none, `"mode": "screen"`)),
		gc("layer.blend", "group", arg(group, `"mode": "screen"`)),
		gc("layer.blend", "locked", arg(locked, `"mode": "screen"`)),
	)
}
