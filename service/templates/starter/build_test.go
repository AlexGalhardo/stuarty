// Package starter_test builds the three starter templates from commands, so no
// document of this folder is written by hand, and checks that the committed
// files are exactly what the commands build. UPDATE_STARTER=1 rewrites them.
package starter_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
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

// build applies the operations to an empty document, in one batch.
func build(t *testing.T, name string, w, h int, ops []op) *timeline.Document {
	t.Helper()

	doc := &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion, ID: id("prj", 1), Name: name,
		Settings: timeline.DefaultSettings(),
		Assets:   map[string]timeline.Asset{}, Tracks: map[string]timeline.Track{}, Items: map[string]timeline.Item{},
	}
	doc.Settings.Width, doc.Settings.Height = w, h

	batch := make([]command.Operation, 0, len(ops))

	for _, p := range ops {
		raw, err := json.Marshal(p.args)
		if err != nil {
			t.Fatal(err)
		}

		batch = append(batch, command.Operation{Op: p.name, Args: raw})
	}

	res, err := command.Default().ApplyBatch(doc, batch, command.Env{
		NewIDs: func(prefix string) string {
			t.Helper()
			t.Fatalf("every identifier of a starter is given, %s was not", prefix)

			return ""
		},
	})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}

	return res.Doc
}

// starter names a template, the file it is committed as and how it is built.
type starter struct {
	file string
	doc  func(t *testing.T) *timeline.Document
}

func starters() []starter {
	return []starter{
		{"vertical-cut-captions.template.json", verticalCutCaptions},
		{"product-teaser.template.json", productTeaser},
		{"quote-card.template.json", quoteCard},
	}
}

// verticalCutCaptions is a 9:16 cut with one media slot and burned-in captions.
func verticalCutCaptions(t *testing.T) *timeline.Document {
	t.Helper()

	video, caps := id("trk", 1), id("trk", 2)

	return build(t, "Vertical cut with captions", 1080, 1920, []op{
		o("template.set_info", map[string]any{
			"description": "A vertical video with word-highlighted captions. Fill the clip and the captions.",
			"colors":      map[string]any{"highlight": "#FFE600", "ink": "#FFFFFF"},
		}),
		o("settings.set", map[string]any{"background": "#000000"}),
		o("track.add", map[string]any{"kind": "visual", "name": "Video", "id": video}),
		o("track.add", map[string]any{"kind": "visual", "name": "Captions", "id": caps}),
		o("clip.add", map[string]any{
			"track_id": video, "asset_id": placeholder, "start": "0s", "duration": "15s", "id": id("itm", 1),
		}),
		o("slot.mark", map[string]any{
			"item_id": id("itm", 1), "name": "clip", "description": "The vertical footage to cut.",
			"duration_policy": "fit",
		}),
		o("track.set_caption_style", map[string]any{"track_id": caps, "style": map[string]any{
			"font_family": "Montserrat", "font_weight": 800, "size": 76, "color": "$ink",
			"stroke_color": "#000000", "stroke_width": 6, "uppercase": true, "align": "center", "max_width": 920,
			"position": "bottom", "highlight": "word", "highlight_color": "$highlight",
		}}),
		o("caption.add", map[string]any{
			"track_id": caps, "start": "0s", "text": "Your words appear here as the clip speaks", "duration": "4s",
			"id": id("itm", 2),
		}),
		o("slot.mark", map[string]any{
			"item_id": id("itm", 2), "name": "subtitles", "description": "The words shown over the clip.",
		}),
	})
}

// productTeaser is a 16:9 commercial: two shots, a headline, a call to action and music.
func productTeaser(t *testing.T) *timeline.Document {
	t.Helper()

	video, music, text := id("trk", 1), id("trk", 2), id("trk", 3)
	title := map[string]any{
		"font_family": "Montserrat", "font_weight": 800, "size": 120, "color": "$ink", "align": "center",
		"shadow_color": "#00000099", "shadow_dy": 6, "shadow_blur": 18, "max_width": 1500,
	}
	cta := map[string]any{
		"font_family": "Inter", "font_weight": 700, "size": 64, "color": "$ink", "background_color": "$brand",
		"background_padding": 28, "background_radius": 20, "align": "center",
	}

	return build(t, "Product teaser", 1920, 1080, []op{
		o("template.set_info", map[string]any{
			"description": "A 10 second product commercial: two shots, a headline, a call to action and a music bed.",
			"colors":      map[string]any{"brand": "#FF3366", "ink": "#FFFFFF"},
		}),
		o("settings.set", map[string]any{"background": "#000000"}),
		o("track.add", map[string]any{"kind": "visual", "name": "Footage", "id": video}),
		o("track.add", map[string]any{"kind": "audio", "name": "Music", "id": music, "position": "top"}),
		o("track.add", map[string]any{"kind": "visual", "name": "Text", "id": text}),
		o("clip.add", map[string]any{
			"track_id": video, "asset_id": placeholder, "start": "0s", "duration": "5s", "id": id("itm", 1),
		}),
		o("clip.add", map[string]any{
			"track_id": video, "asset_id": placeholder, "start": "5s", "duration": "5s", "id": id("itm", 2),
		}),
		o("audio.add", map[string]any{
			"track_id": music, "asset_id": placeholder, "start": "0s", "duration": "10s", "id": id("itm", 3),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "0.5s", "duration": "3.5s", "text": "Meet the new product", "style": title,
			"id": id("itm", 4),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "6s", "duration": "4s", "text": "Available now", "style": cta,
			"id": id("itm", 5),
		}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 4), "preset": "fade_up", "edge": "in"}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 4), "preset": "fade_down", "edge": "out"}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 5), "preset": "pop", "edge": "in"}),
		o("slot.mark", map[string]any{"item_id": id("itm", 1), "name": "shot_1", "description": "The opening shot."}),
		o("slot.mark", map[string]any{"item_id": id("itm", 2), "name": "shot_2", "description": "The closing shot."}),
		o("slot.mark", map[string]any{
			"item_id": id("itm", 3), "name": "music", "description": "The music bed.", "required": false,
		}),
		o("slot.mark", map[string]any{"item_id": id("itm", 4), "name": "headline", "description": "The headline."}),
		o("slot.mark", map[string]any{
			"item_id": id("itm", 5), "name": "call_to_action", "description": "The closing line.",
		}),
	})
}

// quoteCard is a square card: a background shape, a quote and its author.
func quoteCard(t *testing.T) *timeline.Document {
	t.Helper()

	back, bar, text, author := id("trk", 1), id("trk", 2), id("trk", 3), id("trk", 4)

	return build(t, "Quote card", 1080, 1080, []op{
		o("template.set_info", map[string]any{
			"description": "A square quote card with the quote and its author.",
			"colors":      map[string]any{"paper": "#F4EFE6", "ink": "#1B1B1B", "accent": "#B5452A"},
		}),
		o("settings.set", map[string]any{"background": "$paper"}),
		o("track.add", map[string]any{"kind": "visual", "name": "Background", "id": back}),
		o("track.add", map[string]any{"kind": "visual", "name": "Accent", "id": bar}),
		o("track.add", map[string]any{"kind": "visual", "name": "Text", "id": text}),
		o("track.add", map[string]any{"kind": "visual", "name": "Author", "id": author}),
		o("shape.add", map[string]any{
			"track_id": back, "start": "0s", "duration": "6s", "shape": "rect", "width": 1080, "height": 1080,
			"fill": "$paper", "stroke_width": 0, "id": id("itm", 1),
		}),
		o("shape.add", map[string]any{
			"track_id": bar, "start": "0s", "duration": "6s", "shape": "rect", "width": 120, "height": 8,
			"fill": "$accent", "stroke_width": 0, "x": 540, "y": 800, "id": id("itm", 2),
		}),
		o("text.add", map[string]any{
			"track_id": text, "start": "0s", "duration": "6s", "text": "A quote worth sharing goes here.",
			"style": map[string]any{
				"font_family": "Playfair Display", "font_weight": 700, "size": 72, "color": "$ink", "align": "center",
				"max_width": 880, "line_height": 1.25,
			},
			"y": 480, "id": id("itm", 3),
		}),
		o("text.add", map[string]any{
			"track_id": author, "start": "0s", "duration": "6s", "text": "Author Name",
			"style": map[string]any{
				"font_family": "Inter", "font_weight": 500, "size": 40, "color": "$accent", "align": "center",
				"uppercase": true,
			},
			"y": 880, "id": id("itm", 4),
		}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 3), "preset": "fade_up", "edge": "in"}),
		o("text.apply_preset", map[string]any{"item_id": id("itm", 4), "preset": "fade_up", "edge": "in"}),
		o("slot.mark", map[string]any{"item_id": id("itm", 3), "name": "quote", "description": "The quote."}),
		o("slot.mark", map[string]any{"item_id": id("itm", 4), "name": "author", "description": "Who said it."}),
	})
}

func TestStarterFilesAreWhatTheCommandsBuild(t *testing.T) {
	for _, s := range starters() {
		t.Run(s.file, func(t *testing.T) {
			doc := s.doc(t)
			want := timeline.Canonical(doc)

			if os.Getenv("UPDATE_STARTER") == "1" {
				if err := os.WriteFile(s.file, want, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			got, err := os.ReadFile(filepath.Clean(s.file))
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(got, want) {
				t.Errorf("%s is not what the commands build; run with UPDATE_STARTER=1", s.file)
			}

			if vs := timeline.Validate(doc); len(vs) > 0 {
				t.Errorf("%s does not validate: %v", s.file, vs[0])
			}

			if doc.Template == nil || doc.Template.Instance || len(doc.Assets) != 0 {
				t.Errorf("%s is not a template draft without media", s.file)
			}
		})
	}
}

// Every template has slots, none is filled, and every media item is a slot that
// holds the placeholder.
func TestStarterSlotsAreOpen(t *testing.T) {
	for _, s := range starters() {
		doc := s.doc(t)
		slots := 0

		for _, it := range doc.Items {
			if it.Slot == nil {
				if it.Type == timeline.ItemClip || it.Type == timeline.ItemAudio {
					t.Errorf("%s: media item %s is not a slot", s.file, it.ID)
				}

				continue
			}

			slots++

			if it.Slot.Filled {
				t.Errorf("%s: slot %s is filled", s.file, it.Slot.Name)
			}

			if it.Slot.Kind == "media" && it.Asset != placeholder {
				t.Errorf("%s: slot %s holds %q", s.file, it.Slot.Name, it.Asset)
			}
		}

		if slots == 0 {
			t.Errorf("%s has no slot", s.file)
		}
	}
}
