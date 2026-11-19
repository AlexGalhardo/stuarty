package image

func lockedText() *Document {
	d := baseDoc()
	d.Layers[2].Locked = true

	return d
}

func emptyImageSlot() *Document {
	d := baseDoc()
	d.Assets = nil
	d.Layers[1].Asset = ""
	d.Layers[1].Slot = "photo"

	return d
}

func emptyGroup() *Document {
	d := baseDoc()
	d.Layers[4].Children = nil

	return d
}

func croppedImage() *Document {
	d := baseDoc()
	d.Layers[1].Crop = &Crop{X: 200, Y: 0, W: 900, H: 900}

	return d
}

func init() {
	addCases(
		gc(
			"layer.transform",
			"box_and_rotation",
			`{"layer_id": "`+lid('d')+`", "x": 80, "y": 440, "w": 800, "rotation": -5}`,
		),
		gc("layer.transform", "text_width_measures_again", `{"layer_id": "`+lid('c')+`", "w": 300}`),
		gc("layer.transform", "text_height_given", `{"layer_id": "`+lid('c')+`", "w": 300, "h": 500}`),
		gc("layer.transform", "group_offset", `{"layer_id": "`+lid('e')+`", "x": 0, "y": 0}`),
		gc("layer.transform", "nothing_given", `{"layer_id": "`+lid('d')+`"}`),
		gc("layer.transform", "width_zero", `{"layer_id": "`+lid('d')+`", "w": 0}`),
		gc("layer.transform", "rotation_out_of_range", `{"layer_id": "`+lid('d')+`", "rotation": 400}`),
		gc("layer.transform", "missing", `{"layer_id": "`+lid('z')+`", "x": 1}`),
		gc("layer.transform", "group_cannot_be_resized", `{"layer_id": "`+lid('e')+`", "w": 100}`),
		gc("layer.transform", "locked", `{"layer_id": "`+lid('h')+`", "x": 1}`),
		gc("layer.transform", "inside_a_locked_group", `{"layer_id": "`+lid('f')+`", "x": 1}`).on(lockedGroupDoc),
		gc("layer.set", "opacity_and_name", `{"layer_id": "`+lid('d')+`", "opacity": 80, "name": "Headline bar"}`),
		gc("layer.set", "hide", `{"layer_id": "`+lid('e')+`", "visible": false}`),
		gc("layer.set", "unlock", `{"layer_id": "`+lid('h')+`", "locked": false}`),
		gc("layer.set", "slot_on_an_image", `{"layer_id": "`+lid('b')+`", "slot": "photo"}`),
		gc("layer.set", "slot_removed", `{"layer_id": "`+lid('c')+`", "slot": ""}`),
		gc("layer.set", "nothing_given", `{"layer_id": "`+lid('d')+`"}`),
		gc("layer.set", "opacity_over_100", `{"layer_id": "`+lid('d')+`", "opacity": 101}`),
		gc("layer.set", "missing", `{"layer_id": "`+lid('z')+`", "name": "x"}`),
		gc("layer.set", "slot_on_a_shape", `{"layer_id": "`+lid('d')+`", "slot": "bar"}`),
		gc("layer.set", "slot_taken", `{"layer_id": "`+lid('g')+`", "slot": "title"}`),
		gc("layer.set", "slot_malformed", `{"layer_id": "`+lid('g')+`", "slot": "Not A Slot"}`),
		gc("layer.crop", "square_of_the_source", `{"layer_id": "`+lid('b')+`", "x": 200, "y": 0, "w": 900, "h": 900}`),
		gc("layer.crop", "reset", `{"layer_id": "`+lid('b')+`", "reset": true}`).on(croppedImage),
		gc("layer.crop", "partial_rectangle", `{"layer_id": "`+lid('b')+`", "x": 0, "y": 0}`),
		gc("layer.crop", "reset_and_rectangle", `{"layer_id": "`+lid('b')+`", "reset": true, "x": 0}`),
		gc("layer.crop", "missing", `{"layer_id": "`+lid('z')+`", "reset": true}`),
		gc("layer.crop", "not_an_image", `{"layer_id": "`+lid('c')+`", "reset": true}`),
		gc("layer.crop", "outside_the_source", `{"layer_id": "`+lid('b')+`", "x": 1000, "y": 0, "w": 700, "h": 900}`),
		gc("layer.crop", "no_image_yet", `{"layer_id": "`+lid('b')+`", "reset": true}`).on(emptyImageSlot),
		gc("layer.crop", "locked", `{"layer_id": "`+lid('b')+`", "reset": true}`).on(func() *Document {
			d := baseDoc()
			d.Layers[1].Locked = true

			return d
		}),
		gc(
			"layer.set_text",
			"text_and_style",
			`{"layer_id": "`+lid('c')+`", "text": "Hello world", "style": {"color": "#00ff00", "size": 90}}`,
		),
		gc("layer.set_text", "key_removed", `{"layer_id": "`+lid('c')+`", "style": {"color": null}}`),
		gc("layer.set_text", "style_reset", `{"layer_id": "`+lid('c')+`", "reset_style": true, "text": "Plain"}`),
		gc("layer.set_text", "text_cleared", `{"layer_id": "`+lid('c')+`", "text": ""}`),
		gc("layer.set_text", "nothing_given", `{"layer_id": "`+lid('c')+`"}`),
		gc("layer.set_text", "bad_style_key", `{"layer_id": "`+lid('c')+`", "style": {"glow": 1}}`),
		gc("layer.set_text", "size_too_small", `{"layer_id": "`+lid('c')+`", "style": {"size": 2}}`),
		gc("layer.set_text", "max_width_is_not_a_key", `{"layer_id": "`+lid('c')+`", "style": {"max_width": 100}}`),
		gc("layer.set_text", "missing", `{"layer_id": "`+lid('z')+`", "text": "x"}`),
		gc("layer.set_text", "not_text", `{"layer_id": "`+lid('d')+`", "text": "x"}`),
		gc("layer.set_text", "locked", `{"layer_id": "`+lid('c')+`", "text": "x"}`).on(lockedText),
		gc("layer.set_text", "unknown_font", `{"layer_id": "`+lid('c')+`", "style": {"font_family": "Nope Sans"}}`),
		gc("layer.set_text", "weight_the_font_lacks", `{"layer_id": "`+lid('c')+`", "style": {"font_weight": 300}}`),
		gc(
			"layer.set_shape",
			"rounded",
			`{"layer_id": "`+lid(
				'd',
			)+`", "shape": "rounded_rect", "radius": 24, "stroke_color": "#ffffff", "stroke_width": 3}`,
		),
		gc(
			"layer.set_shape",
			"back_to_a_rect_drops_the_radius",
			`{"layer_id": "`+lid('d')+`", "shape": "rect"}`,
		).on(func() *Document {
			d := baseDoc()
			d.Layers[3].Shape, d.Layers[3].Radius = ShapeRoundedRect, 20

			return d
		}),
		gc(
			"layer.set_shape",
			"stroke_removed",
			`{"layer_id": "`+lid('d')+`", "stroke_color": ""}`,
		).on(func() *Document {
			d := baseDoc()
			d.Layers[3].StrokeColor, d.Layers[3].StrokeWidth = "#FFFFFF", 4

			return d
		}),
		gc("layer.set_shape", "fill_colour", `{"layer_id": "`+lid('a')+`", "fill": "#ff0000"}`),
		gc("layer.set_shape", "nothing_given", `{"layer_id": "`+lid('d')+`"}`),
		gc("layer.set_shape", "radius_on_a_rect", `{"layer_id": "`+lid('d')+`", "radius": 5}`),
		gc("layer.set_shape", "bad_colour", `{"layer_id": "`+lid('d')+`", "fill": "red"}`),
		gc("layer.set_shape", "missing", `{"layer_id": "`+lid('z')+`", "fill": "#000000"}`),
		gc("layer.set_shape", "not_a_shape", `{"layer_id": "`+lid('c')+`", "fill": "#000000"}`),
		gc("layer.set_shape", "a_fill_has_no_shape", `{"layer_id": "`+lid('a')+`", "shape": "ellipse"}`),
		gc("layer.set_shape", "locked", `{"layer_id": "`+lid('h')+`", "fill": "#000000"}`),
		gc(
			"layer.align",
			"centre_on_the_canvas",
			`{"layer_ids": ["`+lid('c')+`"], "to": "canvas", "horizontal": "center", "vertical": "middle"}`,
		),
		gc(
			"layer.align",
			"right_and_bottom",
			`{"layer_ids": ["`+lid('d')+`"], "to": "canvas", "horizontal": "right", "vertical": "bottom"}`,
		),
		gc(
			"layer.align",
			"to_another_layer",
			`{"layer_ids": ["`+lid('d')+`"], "to": "`+lid('c')+`", "horizontal": "left"}`,
		),
		gc("layer.align", "a_group", `{"layer_ids": ["`+lid('e')+`"], "to": "canvas", "horizontal": "center"}`),
		gc(
			"layer.align",
			"a_child_to_the_canvas",
			`{"layer_ids": ["`+lid('f')+`"], "to": "canvas", "vertical": "top"}`,
		),
		gc("layer.align", "no_axis", `{"layer_ids": ["`+lid('c')+`"], "to": "canvas"}`),
		gc("layer.align", "to_itself", `{"layer_ids": ["`+lid('c')+`"], "to": "`+lid('c')+`", "horizontal": "left"}`),
		gc("layer.align", "bad_axis_word", `{"layer_ids": ["`+lid('c')+`"], "to": "canvas", "horizontal": "middle"}`),
		gc("layer.align", "layer_missing", `{"layer_ids": ["`+lid('z')+`"], "to": "canvas", "horizontal": "left"}`),
		gc(
			"layer.align",
			"target_missing",
			`{"layer_ids": ["`+lid('c')+`"], "to": "`+lid('z')+`", "horizontal": "left"}`,
		),
		gc("layer.align", "locked", `{"layer_ids": ["`+lid('h')+`"], "to": "canvas", "horizontal": "left"}`),
		gc(
			"layer.align",
			"empty_group",
			`{"layer_ids": ["`+lid('e')+`"], "to": "canvas", "horizontal": "left"}`,
		).on(emptyGroup),
		gc(
			"layer.align",
			"to_an_empty_group",
			`{"layer_ids": ["`+lid('c')+`"], "to": "`+lid('e')+`", "horizontal": "left"}`,
		).on(emptyGroup),
		gc("layer.fit", "cover_the_canvas", `{"layer_id": "`+lid('b')+`", "mode": "cover"}`).on(func() *Document {
			d := baseDoc()
			d.Layers[1].X, d.Layers[1].Y, d.Layers[1].W, d.Layers[1].H = 100, 100, 200, 200

			return d
		}),
		gc("layer.fit", "contain_a_cropped_image", `{"layer_id": "`+lid('b')+`", "mode": "contain"}`).on(croppedImage),
		gc("layer.fit", "shape_into_a_layer", `{"layer_id": "`+lid('d')+`", "mode": "contain", "to": "`+lid('c')+`"}`),
		gc("layer.fit", "a_child_to_the_canvas", `{"layer_id": "`+lid('f')+`", "mode": "cover"}`),
		gc("layer.fit", "to_itself", `{"layer_id": "`+lid('d')+`", "mode": "cover", "to": "`+lid('d')+`"}`),
		gc("layer.fit", "missing", `{"layer_id": "`+lid('z')+`", "mode": "cover"}`),
		gc("layer.fit", "target_missing", `{"layer_id": "`+lid('d')+`", "mode": "cover", "to": "`+lid('z')+`"}`),
		gc("layer.fit", "text_is_sized_by_transform", `{"layer_id": "`+lid('c')+`", "mode": "cover"}`),
		gc("layer.fit", "locked", `{"layer_id": "`+lid('h')+`", "mode": "cover"}`),
		gc("layer.fit", "no_image_yet", `{"layer_id": "`+lid('b')+`", "mode": "cover"}`).on(emptyImageSlot),
	)
}
