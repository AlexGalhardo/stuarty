package lint

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// An optional slot left on its placeholder is not an error (the export skips it);
// a required one still is.
func TestOptionalSlotOnPlaceholderIsQuiet(t *testing.T) {
	t.Parallel()

	for _, required := range []bool{false, true} {
		d := newDoc()
		d.Template = &timeline.TemplateInfo{Description: "t", Instance: true}

		it := clip("itm_music", 0, 90)
		it.Asset = timeline.PlaceholderAsset
		s := timeline.NewSlot("music", "media")
		s.Required = required
		it.Slot = &s
		put(d, it)

		got := has(Lint(d, Env{}, Fast), CodeSlotUnfilled)
		if (len(got) == 1) != required || len(got) > 1 {
			t.Errorf("required=%v: L_SLOT_UNFILLED findings %+v", required, got)
		}
	}
}
