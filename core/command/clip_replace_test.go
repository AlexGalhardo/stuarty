package command

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// replaceMedia applies clip.replace_media to the clip itemA with the asset.
func replaceMedia(t *testing.T, doc *timeline.Document, asset string) (Result, error) {
	t.Helper()

	args := json.RawMessage(fmt.Sprintf(`{"item_id":%q,"asset_id":%q}`, itemA, asset))

	return Default().Apply(doc, Operation{Op: "clip.replace_media", Args: args}, Env{})
}

// SDD-002 C.78: the asset is the one thing that changes. Every other field of the clip, and every other item, is
// as it was, and the old asset stays in the document.
func TestReplaceMediaChangesOnlyTheAsset(t *testing.T) {
	t.Parallel()

	half := 0.5
	x := 120.0
	doc := base().with(withMotionAssets).clip(itemA, vidTr, 40, 100, 30, func(it *timeline.Item) {
		it.Speed, it.X, it.Scale, it.Opacity = timeline.Rational{Num: 2, Den: 1}, &x, 1.5, 0.8
		it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(30, 2, "ease_out")}}
		it.Effects = []timeline.Effect{{Kind: "vignette", Amount: &half, Softness: &half}}
		it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 10}
	}).text(itemC, vidTr, 200, 30).d

	res, err := replaceMedia(t, doc, vidLong)
	if err != nil {
		t.Fatal(err)
	}

	got, want := res.Doc.Items[itemA], doc.Items[itemA]
	if got.Asset != vidLong {
		t.Fatalf("the asset is %s, want %s", got.Asset, vidLong)
	}

	got.Asset = want.Asset
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the clip changed beyond its asset:\n got %+v\nwant %+v", got, want)
	}

	if !reflect.DeepEqual(res.Doc.Items[itemC], doc.Items[itemC]) || len(res.Doc.Assets) != len(doc.Assets) {
		t.Error("another item or an asset changed")
	}

	if len(res.Diff) != 1 || res.Effective["asset_id"] != vidLong || res.Effective["previous_asset_id"] != vidAsset {
		t.Errorf("diff %+v effective %v", res.Diff, res.Effective)
	}
}

// The inverse of a replacement is the old asset: undo restores the clip byte for byte.
func TestReplaceMediaIsUndone(t *testing.T) {
	t.Parallel()

	doc := base().with(withMotionAssets).clip(itemA, vidTr, 0, 90, 0).d

	res, err := replaceMedia(t, doc, vidLong)
	if err != nil {
		t.Fatal(err)
	}

	back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
	if len(skipped) != 0 || !reflect.DeepEqual(back.Doc.Items[itemA], doc.Items[itemA]) {
		t.Errorf("the inverse does not restore the clip (skipped %v): %+v", skipped, back.Doc.Items[itemA])
	}
}

// A video the clip does not fit is refused, naming what the new asset allows: nothing is trimmed to fit.
func TestReplaceMediaRefusesAVideoTheRangeDoesNotFit(t *testing.T) {
	t.Parallel()

	doc := base().with(withMotionAssets).clip(itemA, vidTr, 0, 150, 0).d

	_, err := replaceMedia(t, doc, vidShort)

	e := AsError(err)
	if e == nil || e.Code != CodeOutOfSource {
		t.Fatalf("a 60 frame video for a 150 frame clip: %v", err)
	}

	avail, _ := e.Details["available"].(map[string]any)
	if avail["asset_duration"] != 60 || avail["max_duration"] != 60 {
		t.Errorf("details %+v", e.Details)
	}

	// Trimmed to what it allows, the same clip takes the shorter render.
	short := base().with(withMotionAssets).clip(itemA, vidTr, 0, 60, 0).d
	if _, err := replaceMedia(t, short, vidShort); err != nil {
		t.Errorf("60 frames from 0 fit a 60 frame video: %v", err)
	}
}
