package image

import (
	"strings"
)

const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// fid is the n-th filler layer id.
func fid(n int) string {
	return "lyr_" + string([]byte{
		alphabet[(n>>45)&31], alphabet[(n>>40)&31], alphabet[(n>>35)&31], alphabet[(n>>30)&31], alphabet[(n>>25)&31],
		alphabet[(n>>20)&31], alphabet[(n>>15)&31], alphabet[(n>>10)&31], alphabet[(n>>5)&31], alphabet[n&31],
	})
}

// fullDoc is a document that already has the most layers.
func fullDoc() *Document {
	d := baseDoc()
	n := 1

	for Count(d) < MaxLayers {
		d.Layers = append(d.Layers, Layer{
			ID: fid(n), Kind: KindFill, Name: "Filler", Visible: true, Opacity: 100, W: 10, H: 10, Fill: "#000000",
		})
		n++
	}

	return d
}

// deepDoc has seven groups nested in one another, the last holding a shape, and a group of two levels beside them.
func deepDoc() *Document {
	d := baseDoc()
	ids := []byte{'j', 'k', 'm', 'n', 'p', 'q', 'r'}

	chain := Layer{
		ID: lid('r'), Kind: KindGroup, Name: "G", Visible: true, Opacity: 100,
		Children: []Layer{{
			ID: lid('t'), Kind: KindShape, Name: "Dot", Visible: true, Opacity: 100, W: 10, H: 10,
			Shape: ShapeRect, Fill: "#FFFFFF",
		}},
	}

	for i := len(ids) - 2; i >= 0; i-- {
		chain = Layer{
			ID: lid(ids[i]), Kind: KindGroup, Name: "G", Visible: true, Opacity: 100, Children: []Layer{chain},
		}
	}

	pair := Layer{
		ID: lid('s'), Kind: KindGroup, Name: "Pair", Visible: true, Opacity: 100,
		Children: []Layer{{
			ID: lid('u'), Kind: KindGroup, Name: "Inner", Visible: true, Opacity: 100,
			Children: []Layer{{
				ID: lid('w'), Kind: KindShape, Name: "Dot", Visible: true, Opacity: 100, W: 10, H: 10,
				Shape: ShapeRect, Fill: "#FFFFFF",
			}},
		}},
	}

	d.Layers = append(d.Layers, chain, pair)

	return d
}

func lockedGroupDoc() *Document {
	d := baseDoc()
	d.Layers[4].Locked = true

	return d
}

func groupWithLockedChild() *Document {
	d := baseDoc()
	d.Layers[4].Children[0].Locked = true

	return d
}

// addErrors are the cases every command that adds a layer shares: a parent that is missing, is not a group or
// is locked, an id that is taken, a document that is full.
func addErrors(cmd, body string) []gcase {
	with := func(extra string) string { return "{" + body + "," + extra + "}" }

	return []gcase{
		gc(cmd, "parent_missing", with(`"parent": "`+lid('z')+`"`), lid('k')),
		gc(cmd, "parent_is_not_a_group", with(`"parent": "`+lid('c')+`"`), lid('k')),
		gc(cmd, "parent_is_locked", with(`"parent": "`+lid('h')+`"`), lid('k')),
		gc(cmd, "id_taken", with(`"id": "`+lid('c')+`"`)),
		gc(cmd, "id_malformed", with(`"id": "not-an-id"`)),
		gc(cmd, "document_full", "{"+body+"}", lid('k')).on(fullDoc),
		gc(cmd, "in_a_group_at_the_bottom", with(`"parent": "`+lid('e')+`", "index": 0`), lid('k')),
		gc(cmd, "at_the_deepest_level", with(`"parent": "`+lid('r')+`"`), lid('k')).on(deepDoc),
	}
}

func init() {
	fresh := func() *Document { return New(docID) }

	addCases(
		gc(
			"image.create",
			"preset",
			`{"preset": "video_thumbnail", "name": "Episode 12", "background": "#101820"}`,
		).on(fresh),
		gc(
			"image.create",
			"size_and_template",
			`{"width": 800, "height": 600, "background": "transparent", "template": true}`,
		).on(fresh),
		gc("image.create", "defaults", `{"width": 1080, "height": 1080}`).on(fresh),
		gc("image.create", "preset_and_size", `{"preset": "square_post", "width": 10, "height": 10}`).on(fresh),
		gc("image.create", "nothing_given", `{"name": "Empty"}`).on(fresh),
		gc("image.create", "bad_background", `{"width": 10, "height": 10, "background": "red"}`).on(fresh),
		gc("image.create", "too_large", `{"width": 5000, "height": 10}`).on(fresh),
		gc("image.create", "already_created", `{"preset": "square_post"}`),
		gc("canvas.resize", "keep", `{"width": 1080, "height": 1080}`),
		gc("canvas.resize", "scale", `{"width": 640, "height": 360, "mode": "scale"}`),
		gc("canvas.resize", "zero", `{"width": 0, "height": 100}`),
		gc("canvas.resize", "bad_mode", `{"width": 100, "height": 100, "mode": "stretch"}`),
		gc(
			"canvas.resize",
			"layers_leave_the_limits",
			`{"width": 4096, "height": 4096, "mode": "scale"}`,
		).on(func() *Document {
			d := baseDoc()
			d.Layers[3].X = 10000

			return d
		}),
		gc("layer.add_image", "contain", `{"asset": "`+hashFace+`"}`, lid('k')).on(func() *Document {
			d := baseDoc()
			d.Assets = nil
			d.Layers = d.Layers[:1]

			return d
		}),
		gc("layer.add_image", "cover_a_square", `{"asset": "`+hashLogo+`", "fit": "cover", "name": "Logo"}`, lid('k')),
		gc(
			"layer.add_image",
			"with_a_box_and_a_slot",
			`{"asset": "`+hashLogo+`", "x": 40, "y": 40, "w": 200, "h": 200, "slot": "logo", "index": 1}`,
			lid('k'),
		),
		gc("layer.add_image", "not_a_hash", `{"asset": "face.png"}`),
		gc("layer.add_image", "unknown_file", `{"asset": "sha256:`+strings.Repeat("c", 64)+`"}`, lid('k')),
		gc("layer.add_image", "slot_taken", `{"asset": "`+hashLogo+`", "slot": "title"}`, lid('k')),
		gc("layer.add_image", "too_many_images", `{"asset": "`+hashLogo+`"}`, lid('k')).on(func() *Document {
			d := baseDoc()
			d.Assets = map[string]AssetRef{}

			for i := range MaxAssets {
				h := "sha256:" + strings.Repeat(
					string(alphabet[i%16]),
					60,
				) + strings.Repeat(
					"0",
					3,
				) + string(
					alphabet[i/16],
				)
				d.Assets[h] = AssetRef{Hash: h, Name: "x.png", Width: 10, Height: 10}
				d.Layers = append(d.Layers, Layer{
					ID: fid(i + 1), Kind: KindImage, Name: "x", Visible: true, Opacity: 100, W: 10, H: 10, Asset: h,
				})
			}

			return d
		}),
		gc("layer.add_text", "defaults", `{"text": "Hello world"}`, lid('k')),
		gc(
			"layer.add_text",
			"styled_in_a_slot",
			`{"text": "Episode 12", "w": 600, "x": 40, "y": 40, "slot": "subtitle", "style": {"font_family": "Anton", "size": 60, "color": "#ffe600", "align": "left"}}`,
			lid('k'),
		),
		gc("layer.add_text", "two_lines", `{"text": "One\nTwo", "name": "Lines"}`, lid('k')),
		gc("layer.add_text", "empty_slot", `{"text": "", "slot": "title_2"}`, lid('k')),
		gc("layer.add_text", "unknown_style_key", `{"text": "x", "style": {"glow": 3}}`),
		gc("layer.add_text", "max_width_is_not_a_key", `{"text": "x", "style": {"max_width": 300}}`),
		gc("layer.add_text", "unknown_font", `{"text": "x", "style": {"font_family": "Comic Nope"}}`),
		gc("layer.add_text", "colour_reference", `{"text": "x", "style": {"color": "$accent"}}`),
		gc("layer.add_text", "slot_taken", `{"text": "x", "slot": "title"}`, lid('k')),
		gc("layer.add_text", "slot_malformed", `{"text": "x", "slot": "Title Slot"}`, lid('k')),
		gc(
			"layer.add_shape",
			"rounded",
			`{"shape": "rounded_rect", "w": 600, "h": 200, "radius": 24, "fill": "#e63946", "stroke_color": "#ffffff", "stroke_width": 4}`,
			lid('k'),
		),
		gc(
			"layer.add_shape",
			"ellipse_at_a_position",
			`{"shape": "ellipse", "w": 100, "h": 100, "x": -20, "y": 10}`,
			lid('k'),
		),
		gc("layer.add_shape", "radius_on_a_rect", `{"shape": "rect", "w": 10, "h": 10, "radius": 3}`),
		gc("layer.add_shape", "bad_colour", `{"shape": "rect", "w": 10, "h": 10, "fill": "blue"}`),
		gc("layer.add_shape", "bad_shape", `{"shape": "star", "w": 10, "h": 10}`),
		gc("layer.add_shape", "too_far_away", `{"shape": "rect", "w": 10, "h": 10, "x": 16384, "y": 16384}`, lid('k')),
		gc("layer.add_fill", "on_top", `{"color": "#101820", "name": "Backdrop"}`, lid('k')),
		gc("layer.add_fill", "at_the_bottom", `{"color": "#ff000080", "index": 0}`, lid('k')),
		gc("layer.add_fill", "bad_colour", `{"color": "#12"}`),
		gc(
			"layer.group",
			"two_siblings",
			`{"layer_ids": ["`+lid('c')+`", "`+lid('d')+`"], "name": "Title block"}`,
			lid('k'),
		),
		gc("layer.group", "inside_a_group", `{"layer_ids": ["`+lid('f')+`", "`+lid('g')+`"]}`, lid('k')),
		gc("layer.group", "one_layer_given_id", `{"layer_ids": ["`+lid('b')+`"], "id": "`+lid('m')+`"}`),
		gc("layer.group", "different_parents", `{"layer_ids": ["`+lid('c')+`", "`+lid('f')+`"]}`, lid('k')),
		gc("layer.group", "layer_is_locked", `{"layer_ids": ["`+lid('d')+`", "`+lid('h')+`"]}`, lid('k')),
		gc("layer.group", "layer_missing", `{"layer_ids": ["`+lid('z')+`"]}`, lid('k')),
		gc("layer.group", "listed_twice", `{"layer_ids": ["`+lid('c')+`", "`+lid('c')+`"]}`, lid('k')),
		gc("layer.group", "no_layers", `{"layer_ids": []}`),
		gc("layer.group", "id_taken", `{"layer_ids": ["`+lid('c')+`"], "id": "`+lid('d')+`"}`),
		gc("layer.group", "document_full", `{"layer_ids": ["`+lid('c')+`"]}`, lid('k')).on(fullDoc),
		gc("layer.group", "too_deep", `{"layer_ids": ["`+lid('r')+`"]}`, lid('x')).on(func() *Document {
			// r holds a leaf and sits at the sixth level: a group around it would put the leaf at the ninth
			d := deepDoc()
			q, _ := Find(d, lid('q'))
			q.Children = append(q.Children, Layer{
				ID: lid('y'), Kind: KindShape, Name: "Dot", Visible: true, Opacity: 100, W: 10, H: 10,
				Shape: ShapeRect, Fill: "#FFFFFF",
			})
			ReorderAll(d)

			return d
		}),
		gc("layer.ungroup", "badge", `{"layer_id": "`+lid('e')+`"}`),
		gc("layer.ungroup", "hidden_group", `{"layer_id": "`+lid('e')+`"}`).on(func() *Document {
			d := baseDoc()
			d.Layers[4].Visible = false

			return d
		}),
		gc("layer.ungroup", "not_a_group", `{"layer_id": "`+lid('c')+`"}`),
		gc("layer.ungroup", "missing", `{"layer_id": "`+lid('z')+`"}`),
		gc("layer.ungroup", "locked_group", `{"layer_id": "`+lid('e')+`"}`).on(lockedGroupDoc),
		gc("layer.ungroup", "malformed", `{"layer_id": "x"}`),
		gc("layer.move", "to_the_bottom", `{"layer_id": "`+lid('d')+`", "index": 0}`),
		gc("layer.move", "to_the_top", `{"layer_id": "`+lid('b')+`"}`),
		gc("layer.move", "into_a_group", `{"layer_id": "`+lid('d')+`", "parent": "`+lid('e')+`", "index": 1}`),
		gc("layer.move", "out_to_the_top_level", `{"layer_id": "`+lid('g')+`", "parent": "root"}`),
		gc("layer.move", "inside_its_group", `{"layer_id": "`+lid('g')+`", "index": 0}`),
		gc("layer.move", "nothing_asked", `{"layer_id": "`+lid('d')+`"}`),
		gc("layer.move", "missing", `{"layer_id": "`+lid('z')+`", "index": 0}`),
		gc("layer.move", "parent_is_not_a_group", `{"layer_id": "`+lid('d')+`", "parent": "`+lid('c')+`"}`),
		gc("layer.move", "into_itself", `{"layer_id": "`+lid('e')+`", "parent": "`+lid('e')+`"}`),
		gc("layer.move", "into_a_child", `{"layer_id": "`+lid('s')+`", "parent": "`+lid('u')+`"}`).on(deepDoc),
		gc("layer.move", "locked_layer", `{"layer_id": "`+lid('h')+`", "index": 0}`),
		gc("layer.move", "too_deep", `{"layer_id": "`+lid('s')+`", "parent": "`+lid('r')+`"}`).on(deepDoc),
		gc("layer.duplicate", "shape", `{"layer_id": "`+lid('d')+`", "dx": 40, "dy": 40}`, lid('k')),
		gc(
			"layer.duplicate",
			"slot_is_not_copied",
			`{"layer_id": "`+lid('c')+`", "name": "Title 2", "id": "`+lid('m')+`"}`,
		),
		gc("layer.duplicate", "group_gets_new_ids", `{"layer_id": "`+lid('e')+`"}`, lid('k'), lid('m'), lid('n')),
		gc("layer.duplicate", "missing", `{"layer_id": "`+lid('z')+`"}`),
		gc("layer.duplicate", "id_taken", `{"layer_id": "`+lid('d')+`", "id": "`+lid('c')+`"}`),
		gc("layer.duplicate", "document_full", `{"layer_id": "`+lid('d')+`"}`, lid('k')).on(fullDoc),
		gc("layer.duplicate", "malformed", `{"layer_id": "x"}`),
		gc("layer.remove", "shape", `{"layer_id": "`+lid('d')+`"}`),
		gc("layer.remove", "image_drops_its_asset", `{"layer_id": "`+lid('b')+`"}`),
		gc("layer.remove", "group", `{"layer_id": "`+lid('e')+`"}`),
		gc("layer.remove", "missing", `{"layer_id": "`+lid('z')+`"}`),
		gc("layer.remove", "locked", `{"layer_id": "`+lid('h')+`"}`),
		gc("layer.remove", "group_with_a_locked_child", `{"layer_id": "`+lid('e')+`"}`).on(groupWithLockedChild),
		gc("layer.remove", "malformed", `{"layer_id": "x"}`),
	)

	addCases(addErrors("layer.add_image", `"asset": "`+hashLogo+`"`)...)
	addCases(addErrors("layer.add_text", `"text": "Hi"`)...)
	addCases(addErrors("layer.add_shape", `"shape": "rect", "w": 10, "h": 10`)...)
	addCases(addErrors("layer.add_fill", `"color": "#000000"`)...)
}
