package motion

import (
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func rendered(w, h, num, den, frames int, alpha bool) timeline.Asset {
	return timeline.Asset{
		Kind: timeline.AssetVideo, Width: w, Height: h, FrameRate: timeline.Rational{Num: num, Den: den},
		FrameCount: frames, HasAlpha: alpha,
	}
}

func TestMotionCheckReportsEachMismatchWithTheExpectedValue(t *testing.T) {
	t.Parallel()

	b, _ := Build(doc(), 0, 90, "trk_aaaaaaaaaa")

	if got := Check(b, rendered(1080, 1920, 30, 1, 90, true)); len(got) != 0 {
		t.Fatalf("a file that fits has mismatches: %+v", got)
	}

	got := Check(b, rendered(1920, 1080, 24, 1, 120, false))
	want := map[string][2]string{
		FieldSize: {"1080x1920", "1920x1080"}, FieldFrameRate: {"30/1", "24/1"},
		// 120 frames at 24 fps last 150 frames at the 30 fps of the project.
		FieldDuration: {"90f", "150f"}, FieldAlpha: {"alpha", "no alpha"},
	}

	if len(got) != len(want) {
		t.Fatalf("mismatches %+v", got)
	}

	for _, m := range got {
		if w, ok := want[m.Field]; !ok || m.Expected != w[0] || m.Found != w[1] || m.Hint == "" {
			t.Errorf("%s: expected %q found %q hint %q, want %v", m.Field, m.Expected, m.Found, m.Hint, w)
		}
	}
}

func TestMotionCheckImagesAndOtherKinds(t *testing.T) {
	t.Parallel()

	b, _ := Build(doc(), 0, 90, "trk_bbbbbbbbbb")

	// A still holds for any time: only the size is compared (the lowest track needs no alpha).
	still := timeline.Asset{Kind: timeline.AssetImage, Width: 1080, Height: 1920}
	if got := Check(b, still); len(got) != 0 {
		t.Errorf("a still of the right size: %+v", got)
	}

	still.Width = 540
	if got := Check(b, still); len(got) != 1 || got[0].Field != FieldSize {
		t.Errorf("a still of the wrong size: %+v", got)
	}

	for _, k := range []timeline.AssetKind{timeline.AssetAudio, timeline.AssetLUT} {
		if got := Check(b, timeline.Asset{Kind: k}); len(got) != 1 || got[0].Field != FieldKind {
			t.Errorf("%s: %+v", k, got)
		}
	}
}

func TestMotionCheckComparesRatesByValue(t *testing.T) {
	t.Parallel()

	b, _ := Build(doc(), 0, 60, "")

	// 60/2 is 30 fps: the same rate written another way is not a mismatch.
	if got := Check(b, rendered(1080, 1920, 60, 2, 60, false)); len(got) != 0 {
		t.Errorf("60/2 against 30/1: %+v", got)
	}
}

func TestTargetsAreTheClipsOfTheEarlierRenders(t *testing.T) {
	t.Parallel()

	d := doc()
	clip := func(id, asset string) {
		it := timeline.NewItem(id, timeline.ItemClip, "trk_bbbbbbbbbb", 0, 30)
		it.Asset = asset
		d.Items[id] = it
	}
	clip("itm_clp0000002", "ast_old0000001")
	clip("itm_clp0000001", "ast_old0000001")
	clip("itm_clp0000003", "ast_old0000002")
	clip("itm_clp0000004", "ast_new0000001")
	clip("itm_clp0000005", "ast_other00001")

	got := Targets(
		d,
		map[string]bool{"ast_old0000001": true, "ast_old0000002": true, "ast_new0000001": true},
		"ast_new0000001",
	)
	if want := []string{"itm_clp0000001", "itm_clp0000002", "itm_clp0000003"}; !slices.Equal(got, want) {
		t.Errorf("targets %v, want %v", got, want)
	}
}
