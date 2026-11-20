// Package templates_test builds the templates of the recipes from commands, so no document of this package is
// written by hand, and checks that the committed files are exactly what the commands build. UPDATE_TEMPLATES=1
// rewrites them.
package templates_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const placeholder = timeline.PlaceholderAsset

// id is a fixed identifier: 10 hexadecimal digits are valid Crockford characters.
func id(prefix string, n int) string { return fmt.Sprintf("%s_%010x", prefix, n) }

type op struct {
	name string
	args map[string]any
}

func o(name string, args map[string]any) op { return op{name: name, args: args} }

func operations(t *testing.T, ops []op) []command.Operation {
	t.Helper()

	batch := make([]command.Operation, 0, len(ops))

	for _, p := range ops {
		raw, err := json.Marshal(p.args)
		if err != nil {
			t.Fatal(err)
		}

		batch = append(batch, command.Operation{Op: p.name, Args: raw})
	}

	return batch
}

// build applies the operations to an empty document, in one batch.
func build(t *testing.T, name string, w, h int, ops []op) *timeline.Document {
	t.Helper()

	doc := &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion, ID: id("prj", 1), Name: name,
		Settings: timeline.DefaultSettings(),
		Assets:   map[string]timeline.Asset{}, Tracks: map[string]timeline.Track{}, Items: map[string]timeline.Item{},
	}
	doc.Settings.Width, doc.Settings.Height = w, h

	res, err := command.Default().ApplyBatch(doc, operations(t, ops), command.Env{
		NewIDs: func(prefix string) string {
			t.Helper()
			t.Fatalf("every identifier of a template is given, %s was not", prefix)

			return ""
		},
	})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}

	return res.Doc
}

// template names a template, the file it is committed as and how it is built.
type template struct {
	file string
	doc  func(t *testing.T) *timeline.Document
}

func timelineTemplates() []template {
	return []template{
		{"timeline/commercial-15s.template.json", commercial15},
		{"timeline/commercial-30s.template.json", commercial30},
		{"timeline/long-video-opening.template.json", longOpening},
	}
}

// The text styles of a commercial: big, bold, and readable over footage. Positions are the centre of the item in
// the frame, inside the safe band of a 1080 by 1920 video (220 pixels from the top, 500 from the bottom).
func headline(color string) map[string]any {
	return map[string]any{
		"font_family": "Montserrat", "font_weight": 800, "size": 96, "color": color, "align": "center",
		"max_width": 880, "line_height": 1.1, "uppercase": true,
		"shadow_color": "#000000AA", "shadow_dy": 4, "shadow_blur": 14,
	}
}

func pill() map[string]any {
	return map[string]any{
		"font_family": "Inter", "font_weight": 800, "size": 64, "color": "$brand", "background_color": "$ink",
		"background_padding": 30, "background_radius": 24, "align": "center", "max_width": 820,
	}
}

func card(track string, n int, start, dur, color string) op {
	return o("shape.add", map[string]any{
		"track_id": track, "start": start, "duration": dur, "shape": "rect", "width": 1080, "height": 1920,
		"fill": color, "stroke_width": 0, "id": id("itm", n),
	})
}

func slotOp(item int, name, description string, extra map[string]any) op {
	args := map[string]any{"item_id": id("itm", item), "name": name, "description": description}
	for k, v := range extra {
		args[k] = v
	}

	return o("slot.mark", args)
}

func optional() map[string]any { return map[string]any{"required": false} }

func commercialInfo(description string) op {
	return o("template.set_info", map[string]any{
		"description": description,
		"colors":      map[string]any{"brand": "#FF3366", "ink": "#FFFFFF", "accent": "#1B1B2F"},
	})
}

// commercial15 is a 9:16 commercial of 15 seconds: hook, problem, product, offer, call to action.
func commercial15(t *testing.T) *timeline.Document {
	t.Helper()

	footage, cards, text, music := id("trk", 1), id("trk", 2), id("trk", 3), id("trk", 4)

	return build(t, "Commercial 15 seconds", 1080, 1920, []op{
		commercialInfo("A 15 second vertical commercial: a hook, the problem, the product, the offer and a call to " +
			"action. Fill the three shots, the four lines and, if you have one, the music."),
		o("settings.set", map[string]any{"background": "$accent"}),
		o("track.add", map[string]any{"kind": "visual", "name": "Footage", "id": footage}),
		o("track.add", map[string]any{"kind": "visual", "name": "Cards", "id": cards}),
		o("track.add", map[string]any{"kind": "visual", "name": "Text", "id": text}),
		o("track.add", map[string]any{"kind": "audio", "name": "Music", "id": music, "position": "top"}),
		o("clip.add", map[string]any{
			"track_id": footage, "asset_id": placeholder, "start": "0s", "duration": "3s", "id": id("itm", 1),
		}),
		o("clip.add", map[string]any{
			"track_id": footage, "asset_id": placeholder, "start": "3s", "duration": "3.5s", "id": id("itm", 2),
		}),
		o("clip.add", map[string]any{
			"track_id": footage, "asset_id": placeholder, "start": "6.5s", "duration": "4s", "id": id("itm", 3),
		}),
		card(cards, 4, "10.5s", "4.5s", "$brand"),
		o("audio.add", map[string]any{
			"track_id": music, "asset_id": placeholder, "start": "0s", "duration": "15s", "id": id("itm", 5),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "0.3s", "duration": "2.7s", "text": "Tired of the usual way?",
			"style": headline("$ink"), "y": 1100, "id": id("itm", 6),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "3.2s", "duration": "3.3s", "text": "Here is what it costs you",
			"style": headline("$ink"), "y": 1100, "id": id("itm", 7),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "6.7s", "duration": "3.8s", "text": "Meet the product",
			"style": headline("$ink"), "y": 1100, "id": id("itm", 8),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "10.7s", "duration": "2.3s", "text": "The offer, in one line",
			"style": headline("$ink"), "y": 800, "id": id("itm", 9),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "13s", "duration": "2s", "text": "Get it today",
			"style": pill(), "y": 1100, "id": id("itm", 10),
		}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 6), "preset": "pop", "edge": "in"}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 9), "preset": "fade_up", "edge": "in"}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 10), "preset": "pop", "edge": "in"}),
		slotOp(1, "hook_shot", "The opening shot: what stops the thumb.", nil),
		slotOp(2, "problem_shot", "The shot of the problem the person has.", nil),
		slotOp(3, "product_shot", "The product, in use.", nil),
		slotOp(5, "music", "The music bed.", optional()),
		slotOp(6, "hook", "The hook, five words or fewer.", nil),
		slotOp(7, "problem", "The problem, in one line.", nil),
		slotOp(8, "product_line", "What the product is or does, in one line.", nil),
		slotOp(9, "offer", "The offer: the price, the discount or the deadline.", nil),
		slotOp(10, "call_to_action", "What to do next, in three words.", nil),
	})
}

// commercial30 is the same five beats in 30 seconds, with two product shots and a proof card.
func commercial30(t *testing.T) *timeline.Document {
	t.Helper()

	footage, cards, text, music := id("trk", 1), id("trk", 2), id("trk", 3), id("trk", 4)

	return build(t, "Commercial 30 seconds", 1080, 1920, []op{
		commercialInfo("A 30 second vertical commercial: a hook, the problem, two shots of the product, a proof " +
			"card, the offer and a call to action."),
		o("settings.set", map[string]any{"background": "$accent"}),
		o("track.add", map[string]any{"kind": "visual", "name": "Footage", "id": footage}),
		o("track.add", map[string]any{"kind": "visual", "name": "Cards", "id": cards}),
		o("track.add", map[string]any{"kind": "visual", "name": "Text", "id": text}),
		o("track.add", map[string]any{"kind": "audio", "name": "Music", "id": music, "position": "top"}),
		o("clip.add", map[string]any{
			"track_id": footage, "asset_id": placeholder, "start": "0s", "duration": "4s", "id": id("itm", 1),
		}),
		o("clip.add", map[string]any{
			"track_id": footage, "asset_id": placeholder, "start": "4s", "duration": "6s", "id": id("itm", 2),
		}),
		o("clip.add", map[string]any{
			"track_id": footage, "asset_id": placeholder, "start": "10s", "duration": "6s", "id": id("itm", 3),
		}),
		o("clip.add", map[string]any{
			"track_id": footage, "asset_id": placeholder, "start": "16s", "duration": "5s", "id": id("itm", 4),
		}),
		card(cards, 5, "21s", "4s", "$accent"),
		card(cards, 6, "25s", "5s", "$brand"),
		o("audio.add", map[string]any{
			"track_id": music, "asset_id": placeholder, "start": "0s", "duration": "30s", "id": id("itm", 7),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "0.3s", "duration": "3.7s", "text": "Tired of the usual way?",
			"style": headline("$ink"), "y": 1100, "id": id("itm", 8),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "4.3s", "duration": "5.7s", "text": "Here is what it costs you",
			"style": headline("$ink"), "y": 1100, "id": id("itm", 9),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "10.3s", "duration": "10.7s", "text": "Meet the product",
			"style": headline("$ink"), "y": 1100, "id": id("itm", 10),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "21.3s", "duration": "3.7s", "text": "The proof, in one line",
			"style": headline("$ink"), "y": 800, "id": id("itm", 11),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "25.3s", "duration": "2.9s", "text": "The offer, in one line",
			"style": headline("$ink"), "y": 800, "id": id("itm", 12),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "28.3s", "duration": "1.7s", "text": "Get it today",
			"style": pill(), "y": 1100, "id": id("itm", 13),
		}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 8), "preset": "pop", "edge": "in"}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 11), "preset": "fade_up", "edge": "in"}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 12), "preset": "fade_up", "edge": "in"}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 13), "preset": "pop", "edge": "in"}),
		slotOp(1, "hook_shot", "The opening shot: what stops the thumb.", nil),
		slotOp(2, "problem_shot", "The shot of the problem the person has.", nil),
		slotOp(3, "product_shot_1", "The product, in use.", nil),
		slotOp(4, "product_shot_2", "A second view of the product: a detail, or the result.", nil),
		slotOp(7, "music", "The music bed.", optional()),
		slotOp(8, "hook", "The hook, five words or fewer.", nil),
		slotOp(9, "problem", "The problem, in one line.", nil),
		slotOp(10, "product_line", "What the product is or does, in one line.", nil),
		slotOp(11, "proof", "One proof from the brief: a number, a review, a demonstration.", nil),
		slotOp(12, "offer", "The offer: the price, the discount or the deadline.", nil),
		slotOp(13, "call_to_action", "What to do next, in three words.", nil),
	})
}

// longOpening is the opening of a 16:9 long video: a cold open, a title card and a lower third.
func longOpening(t *testing.T) *timeline.Document {
	t.Helper()

	footage, cards, text, music, sub := id("trk", 1), id("trk", 2), id("trk", 3), id("trk", 4), id("trk", 5)
	title := map[string]any{
		"font_family": "Montserrat", "font_weight": 800, "size": 110, "color": "$ink", "align": "center",
		"max_width": 1500, "line_height": 1.1,
	}
	subtitle := map[string]any{
		"font_family": "Inter", "font_weight": 500, "size": 52, "color": "$accent", "align": "center", "max_width": 1400,
	}
	lower := map[string]any{
		"font_family": "Inter", "font_weight": 700, "size": 48, "color": "$ink", "background_color": "$brand",
		"background_padding": 22, "background_radius": 12, "align": "left", "max_width": 900,
	}

	return build(t, "Long video opening", 1920, 1080, []op{
		o("template.set_info", map[string]any{
			"description": "The opening of a long talking video: a cold open of the best sentence, a title card and a " +
				"lower third with the name of the speaker. Put the talking head after the title card.",
			"colors": map[string]any{"brand": "#2D5BFF", "ink": "#FFFFFF", "accent": "#FFD166"},
		}),
		o("settings.set", map[string]any{"background": "#101820"}),
		o("track.add", map[string]any{"kind": "visual", "name": "Footage", "id": footage}),
		o("track.add", map[string]any{"kind": "visual", "name": "Cards", "id": cards}),
		o("track.add", map[string]any{"kind": "visual", "name": "Text", "id": text}),
		o("track.add", map[string]any{"kind": "visual", "name": "Subtitle", "id": sub}),
		o("track.add", map[string]any{"kind": "audio", "name": "Music", "id": music, "position": "top"}),
		o("clip.add", map[string]any{
			"track_id": footage, "asset_id": placeholder, "start": "0s", "duration": "6s", "id": id("itm", 1),
		}),
		o("shape.add", map[string]any{
			"track_id": cards, "start": "6s", "duration": "4s", "shape": "rect", "width": 1920, "height": 1080,
			"fill": "$brand", "stroke_width": 0, "id": id("itm", 2),
		}),
		o("audio.add", map[string]any{
			"track_id": music, "asset_id": placeholder, "start": "0s", "duration": "10s", "id": id("itm", 3),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "6.3s", "duration": "3.4s", "text": "What this video is about",
			"style": title, "y": 480, "id": id("itm", 4),
		}),
		o("text.add", map[string]any{
			"track_id": sub, "start": "7s", "duration": "2.7s", "text": "A line that says who it is for",
			"style": subtitle, "y": 700, "id": id("itm", 5),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "10.5s", "duration": "4s", "text": "Speaker name, role",
			"style": lower, "x": 520, "y": 900, "id": id("itm", 6),
		}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 4), "preset": "fade_up", "edge": "in"}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 6), "preset": "fade_up", "edge": "in"}),
		slotOp(1, "cold_open", "The best sentence of the talk, six seconds, before any title.", nil),
		slotOp(3, "music", "The music under the opening.", optional()),
		slotOp(4, "title", "The title of the video.", nil),
		slotOp(5, "subtitle", "Who it is for, or the promise.", optional()),
		slotOp(6, "speaker", "The name and the role of the speaker.", nil),
	})
}

func writeOrRead(t *testing.T, file string, want []byte) []byte {
	t.Helper()

	if os.Getenv("UPDATE_TEMPLATES") == "1" {
		if err := os.WriteFile(file, want, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	got, err := os.ReadFile(filepath.Clean(file))
	if err != nil {
		t.Fatal(err)
	}

	return got
}

func TestTimelineFilesAreWhatTheCommandsBuild(t *testing.T) {
	for _, tpl := range timelineTemplates() {
		t.Run(tpl.file, func(t *testing.T) {
			doc := tpl.doc(t)
			want := timeline.Canonical(doc)
			got := writeOrRead(t, tpl.file, want)

			if !bytes.Equal(got, want) {
				t.Errorf("%s is not what the commands build; run with UPDATE_TEMPLATES=1", tpl.file)
			}

			if vs := timeline.Validate(doc); len(vs) > 0 {
				t.Errorf("%s does not validate: %v", tpl.file, vs[0])
			}

			if doc.Template == nil || doc.Template.Instance || len(doc.Assets) != 0 {
				t.Errorf("%s is not a template draft without media", tpl.file)
			}
		})
	}
}

// Every template has slots, none is filled, and every media item is a slot that holds the placeholder.
func TestTimelineSlotsAreOpen(t *testing.T) {
	for _, tpl := range timelineTemplates() {
		doc := tpl.doc(t)
		slots := 0

		for _, it := range doc.Items {
			if it.Slot == nil {
				if it.Type == timeline.ItemClip || it.Type == timeline.ItemAudio {
					t.Errorf("%s: media item %s is not a slot", tpl.file, it.ID)
				}

				continue
			}

			slots++

			if it.Slot.Filled {
				t.Errorf("%s: slot %s is filled", tpl.file, it.Slot.Name)
			}

			if it.Slot.Kind == "media" && it.Asset != placeholder {
				t.Errorf("%s: slot %s holds %q", tpl.file, it.Slot.Name, it.Asset)
			}
		}

		if slots == 0 {
			t.Errorf("%s has no slot", tpl.file)
		}
	}
}

// ---- the image templates ------------------------------------------------------------------------------------

const fixtureHash = "sha256:0000000000000000000000000000000000000000000000000000000000000001"

type imageTemplate struct {
	file string
	doc  func(t *testing.T) *coreimage.Document
}

func imageTemplates() []imageTemplate {
	return []imageTemplate{
		{"image/commercial-cover-vertical.image.json", coverVertical},
		{"image/commercial-cover-landscape.image.json", coverLandscape},
		{"image/long-video-thumbnail.image.json", longThumbnail},
	}
}

func lid(n int) string { return id("lyr", n) }

// buildImage applies the operations to a new template document. The picture slot is made with a fixture asset
// and then emptied: a slot with no file is what makes it a slot to fill (SDD-021 section 7).
func buildImage(t *testing.T, docID string, ops []op) *coreimage.Document {
	t.Helper()

	env := coreimage.Env{
		NewIDs: func(prefix string) string {
			t.Helper()
			t.Fatalf("every identifier of a template is given, %s was not", prefix)

			return ""
		},
		Asset: func(hash string) (coreimage.AssetInfo, bool) {
			return coreimage.AssetInfo{Width: 1000, Height: 1000}, hash == fixtureHash
		},
	}

	res, err := coreimage.ApplyBatch(coreimage.New(docID), operations(t, ops), env)
	if err != nil {
		t.Fatal(err)
	}

	doc := res.Doc
	doc.Assets = nil

	coreimage.Walk(doc, func(l, _ *coreimage.Layer, _ int) {
		if l.Kind == coreimage.KindImage {
			l.Asset = ""
		}
	})

	return doc
}

func coverStyle(size int, color string) map[string]any {
	return map[string]any{
		"font_family": "Montserrat", "font_weight": 800, "size": size, "color": color, "align": "center",
		"uppercase": true, "line_height": 1.05,
	}
}

// coverVertical is the cover of a 9:16 commercial: the picture, a dark band, a title and a line under it.
func coverVertical(t *testing.T) *coreimage.Document {
	t.Helper()

	return buildImage(t, "img_r3c1p30001", []op{
		o("image.create", map[string]any{
			"preset": "vertical_cover", "name": "Commercial cover, vertical", "background": "#1B1B2F", "template": true,
		}),
		o("layer.add_image", map[string]any{
			"asset": fixtureHash, "name": "Picture", "fit": "cover", "x": 0, "y": 0, "w": 1080, "h": 1920,
			"slot": "picture", "id": lid(1),
		}),
		o("layer.add_shape", map[string]any{
			"shape": "rect", "w": 1080, "h": 620, "x": 0, "y": 1180, "fill": "#1B1B2F", "name": "Band", "id": lid(2),
		}),
		o("layer.add_shape", map[string]any{
			"shape": "rect", "w": 160, "h": 12, "x": 460, "y": 1230, "fill": "#FF3366", "name": "Accent", "id": lid(3),
		}),
		o("layer.add_text", map[string]any{
			"text": "Your headline here", "name": "Title", "x": 90, "y": 1320, "w": 900,
			"style": coverStyle(110, "#FFFFFF"), "slot": "title", "id": lid(4),
		}),
		o("layer.add_text", map[string]any{
			"text": "Available now", "name": "Subtitle", "x": 90, "y": 1620, "w": 900,
			"style": map[string]any{
				"font_family": "Inter", "font_weight": 700, "size": 56, "color": "#FF3366", "align": "center",
			},
			"slot": "subtitle", "id": lid(5),
		}),
	})
}

// coverLandscape is the cover of a 16:9 commercial on a feed.
func coverLandscape(t *testing.T) *coreimage.Document {
	t.Helper()

	return buildImage(t, "img_r3c1p30002", []op{
		o("image.create", map[string]any{
			"preset": "video_thumbnail", "name": "Commercial cover, landscape", "background": "#1B1B2F", "template": true,
		}),
		o("layer.add_image", map[string]any{
			"asset": fixtureHash, "name": "Picture", "fit": "cover", "x": 0, "y": 0, "w": 1280, "h": 720,
			"slot": "picture", "id": lid(1),
		}),
		o("layer.add_shape", map[string]any{
			"shape": "rect", "w": 700, "h": 720, "x": 0, "y": 0, "fill": "#1B1B2F", "name": "Panel", "id": lid(2),
		}),
		o("layer.add_shape", map[string]any{
			"shape": "rect", "w": 120, "h": 10, "x": 60, "y": 150, "fill": "#FF3366", "name": "Accent", "id": lid(3),
		}),
		o("layer.add_text", map[string]any{
			"text": "Your headline here", "name": "Title", "x": 60, "y": 200, "w": 580,
			"style": map[string]any{
				"font_family": "Montserrat", "font_weight": 800, "size": 88, "color": "#FFFFFF", "align": "left",
				"uppercase": true, "line_height": 1.05,
			},
			"slot": "title", "id": lid(4),
		}),
		o("layer.add_text", map[string]any{
			"text": "Available now", "name": "Subtitle", "x": 60, "y": 560, "w": 580,
			"style": map[string]any{
				"font_family": "Inter", "font_weight": 700, "size": 44, "color": "#FF3366", "align": "left",
			},
			"slot": "subtitle", "id": lid(5),
		}),
	})
}

// longThumbnail is the thumbnail of a long talking video: the face on the right, the title on the left.
func longThumbnail(t *testing.T) *coreimage.Document {
	t.Helper()

	return buildImage(t, "img_r3c1p30003", []op{
		o("image.create", map[string]any{
			"preset": "video_thumbnail", "name": "Long video thumbnail", "background": "#101820", "template": true,
		}),
		o("layer.add_image", map[string]any{
			"asset": fixtureHash, "name": "Picture", "fit": "cover", "x": 640, "y": 0, "w": 640, "h": 720,
			"slot": "picture", "id": lid(1),
		}),
		o("layer.add_shape", map[string]any{
			"shape": "rect", "w": 14, "h": 720, "x": 626, "y": 0, "fill": "#FFD166", "name": "Accent", "id": lid(2),
		}),
		o("layer.add_text", map[string]any{
			"text": "Three words max", "name": "Title", "x": 50, "y": 150, "w": 550,
			"style": map[string]any{
				"font_family": "Montserrat", "font_weight": 800, "size": 84, "color": "#FFFFFF", "align": "left",
				"uppercase": true, "line_height": 1.05,
			},
			"slot": "title", "id": lid(3),
		}),
		o("layer.add_text", map[string]any{
			"text": "The topic of the talk", "name": "Subtitle", "x": 50, "y": 540, "w": 550,
			"style": map[string]any{
				"font_family": "Inter", "font_weight": 600, "size": 40, "color": "#FFD166", "align": "left",
			},
			"slot": "subtitle", "id": lid(4),
		}),
	})
}

func TestImageFilesAreWhatTheCommandsBuild(t *testing.T) {
	for _, tpl := range imageTemplates() {
		t.Run(tpl.file, func(t *testing.T) {
			doc := tpl.doc(t)
			want := coreimage.Canonical(doc)
			got := writeOrRead(t, tpl.file, want)

			if !bytes.Equal(got, want) {
				t.Errorf("%s is not what the commands build; run with UPDATE_TEMPLATES=1", tpl.file)
			}

			if vs := coreimage.Validate(doc); len(vs) > 0 {
				t.Errorf("%s does not validate: %v", tpl.file, vs[0])
			}

			if !doc.Template || len(doc.Assets) != 0 {
				t.Errorf("%s is not a template with no media", tpl.file)
			}

			picture := false

			for _, l := range coreimage.Slots(doc) {
				picture = picture || l.Slot == "picture" && l.Kind == coreimage.KindImage && l.Asset == ""
			}

			if !picture {
				t.Errorf("%s has no open picture slot", tpl.file)
			}
		})
	}
}
