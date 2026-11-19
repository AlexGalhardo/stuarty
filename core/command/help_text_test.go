package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The help of track.set_caption_style once said highlight accepts only "none" while
// the schema also takes "word" (VV-100).
func TestCaptionStyleHelpMatchesHighlightEnum(t *testing.T) {
	help, err := Default().Help("track.set_caption_style")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help, "accepts only") {
		t.Errorf("help is out of date: %s", help)
	}
}

// Every id argument of a command carries the pattern of the ids ValidID accepts (VV-103).
func TestIDArgumentsHavePattern(t *testing.T) {
	prefixes := map[string]string{
		"item_id": "itm", "from_item_id": "itm", "to_item_id": "itm", "clip_item_id": "itm", "new_id": "itm",
		"new_ids": "itm", "item_ids": "itm", "speech_item_ids": "itm", "freeze_id": "itm",
		"track_id": "trk", "above": "trk", "below": "trk", "asset_id": "ast", "lut_asset": "ast",
	}
	reg := Default()
	for _, name := range reg.Names() {
		raw, err := reg.Schema(name)
		if err != nil {
			t.Fatal(err)
		}
		var sc struct {
			Properties map[string]struct {
				Pattern string `json:"pattern"`
				Items   struct {
					Pattern string `json:"pattern"`
				} `json:"items"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &sc); err != nil {
			t.Fatal(err)
		}
		for prop, p := range sc.Properties {
			prefix, ok := prefixes[prop]
			if !ok {
				continue
			}
			got := p.Pattern + p.Items.Pattern
			want := timeline.IDPattern(prefix)
			placeholder := prefix == "ast" && strings.Contains(got, "placeholder")
			if got != want && !placeholder {
				t.Errorf("%s.%s pattern %q, want %q", name, prop, got, want)
			}
		}
	}
}
