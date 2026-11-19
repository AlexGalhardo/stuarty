package command

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func presetDoc() *timeline.Document {
	return base().clip(itemA, vidTr, 0, 90, 0).d
}

func presetOp(preset, extra string) string {
	return fmt.Sprintf(`{"item_id": %q, "preset": %q%s}`, itemA, preset, extra)
}

// RF-082: a preset expands into ordinary keyframes, which the keyframe commands then edit.
func TestAnimationPresetKeyframesCanBeEditedAfterwards(t *testing.T) {
	t.Parallel()

	doc := applyBeta(t, presetDoc(), "animation.apply_preset", presetOp("pop", ""))
	keys := doc.Items[itemA].Keyframes["scale"]

	if len(keys) != 2 || keys[0].Easing != timeline.EasingBezier || len(keys[0].Bezier) != 4 {
		t.Fatalf("the pop is not a bezier segment of two keyframes: %+v", keys)
	}

	// Change the value of the end keyframe, the curve of the first, then remove the first.
	doc = applyBeta(
		t,
		doc,
		"keyframe.set",
		fmt.Sprintf(`{"item_id": %q, "property": "scale", "at": "15f", "value": 1.3}`, itemA),
	)
	doc = applyBeta(t, doc, "keyframe.set_easing",
		fmt.Sprintf(`{"item_id": %q, "property": "scale", "at": "0f", "bezier": [0.2, 0.8, 0.3, 1]}`, itemA))

	keys = doc.Items[itemA].Keyframes["scale"]
	if keys[1].Value != 1.3 || !slices.Equal(keys[0].Bezier, []float64{0.2, 0.8, 0.3, 1}) {
		t.Errorf("the edits did not land: %+v", keys)
	}

	doc = applyBeta(t, doc, "keyframe.remove", fmt.Sprintf(`{"item_id": %q, "property": "scale", "at": "0f"}`, itemA))
	if got := doc.Items[itemA].Keyframes["scale"]; len(got) != 1 || got[0].Value != 1.3 {
		t.Errorf("after the removal: %+v", got)
	}

	if vs := timeline.Validate(doc); len(vs) != 0 {
		t.Errorf("the document is not valid: %v", vs)
	}
}

// keyframe.clear removes what a preset wrote, as it does for any keyframes: the item keeps the value of its first frame.
func TestAnimationPresetThenClearLeavesNoKeyframes(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ preset, extra string }{
		{"fade", ""},
		{"slide", `, "direction": "left"`},
		{"slide", `, "direction": "down", "edge": "out"`},
		{"scale", ""},
		{"push_in", `, "amount": 1.5`},
	} {
		applied := applyBeta(t, presetDoc(), "animation.apply_preset", presetOp(c.preset, c.extra))
		if len(applied.Items[itemA].Keyframes) == 0 {
			t.Fatalf("%s wrote no keyframes", c.preset)
		}

		cleared := applyBeta(t, applied, "keyframe.clear", fmt.Sprintf(`{"item_id": %q}`, itemA))
		if got := cleared.Items[itemA].Keyframes; len(got) != 0 {
			t.Errorf("%s then keyframe.clear left %v", c.preset, got)
		}

		if vs := timeline.Validate(cleared); len(vs) != 0 {
			t.Errorf("%s then keyframe.clear: %v", c.preset, vs)
		}
	}
}

// Both edges of one property: applying out after in keeps the first end.
func TestAnimationPresetInThenOutKeepsBothEnds(t *testing.T) {
	t.Parallel()

	doc := applyBeta(t, presetDoc(), "animation.apply_preset", presetOp("fade", ""))
	doc = applyBeta(t, doc, "animation.apply_preset", presetOp("fade", `, "edge": "out"`))
	keys := doc.Items[itemA].Keyframes["opacity"]

	want := [][2]float64{{0, 0}, {15, 1}, {75, 1}, {90, 0}}
	if len(keys) != len(want) {
		t.Fatalf("%d keyframes, want %d: %+v", len(keys), len(want), keys)
	}

	for i, k := range keys {
		if float64(k.T) != want[i][0] || k.Value != want[i][1] {
			t.Errorf("keyframe %d is (%d, %v), want %v", i, k.T, k.Value, want[i])
		}
	}
}

// Applying the same preset twice gives the same document (SDD-002 C.76 idempotency).
func TestAnimationPresetTwiceIsTheSameAsOnce(t *testing.T) {
	t.Parallel()

	for _, p := range []string{"fade", "slide", "scale", "pop", "push_in", "pull_out"} {
		once := applyBeta(t, presetDoc(), "animation.apply_preset", presetOp(p, ""))
		twice := applyBeta(t, once, "animation.apply_preset", presetOp(p, ""))

		if !bytes.Equal(timeline.Canonical(once), timeline.Canonical(twice)) {
			t.Errorf("%s applied twice differs from once", p)
		}
	}
}

// The sampled value at the ends of an "in" preset is the far value and then the value of the item.
func TestAnimationPresetSamplesFromTheFarValueToTheItsOwn(t *testing.T) {
	t.Parallel()

	doc := applyBeta(t, presetDoc(), "animation.apply_preset", presetOp("scale", `, "amount": 0.25, "duration": "1s"`))
	it := doc.Items[itemA]
	keys := it.Keyframes["scale"]

	for _, c := range []struct {
		frame int
		want  float64
	}{{0, 0.25}, {30, 1}, {89, 1}} {
		if got := timeline.ValueAt(keys, c.frame, it.Scale); got != c.want {
			t.Errorf("scale at frame %d = %v, want %v", c.frame, got, c.want)
		}
	}

	if mid := timeline.ValueAt(keys, 15, it.Scale); mid <= 0.25 || mid >= 1 {
		t.Errorf("scale in the middle of the ramp = %v, want between the ends", mid)
	}
}

// Every preset of the list can be applied to a clip and to a text item, on both edges.
func TestAnimationPresetListIsAppliable(t *testing.T) {
	t.Parallel()

	for _, p := range animationPresets {
		for _, edge := range []string{"in", "out"} {
			for _, doc := range []*timeline.Document{
				presetDoc(),
				base().text(itemA, vidTr, 0, 90).d,
			} {
				extra := fmt.Sprintf(`, "edge": %q, "duration": "30f"`, edge)
				got := applyBeta(t, doc, "animation.apply_preset", presetOp(p.ID, extra))

				if vs := timeline.Validate(got); len(vs) != 0 {
					t.Errorf("%s %s: %v", p.ID, edge, vs)
				}
			}
		}
	}
}

// text.apply_preset gives the last keyframe of an in edge the easing in force there (SDD-002 C.30); when
// that easing is a Bezier curve, the keyframe gets its four numbers too, so the document stays valid.
func TestTextPresetInheritsABezierCurveWithItsNumbers(t *testing.T) {
	t.Parallel()

	curve := []float64{0.2, 0.8, 0.3, 1}
	first := `{"item_id": %q, "property": "opacity", "at": "0f", "value": 0.5, "easing": "bezier", ` +
		`"bezier": [0.2, 0.8, 0.3, 1]}`
	doc := applyBeta(t, presetDoc(), "keyframe.set", fmt.Sprintf(first, itemA))
	doc = applyBeta(
		t,
		doc,
		"keyframe.set",
		fmt.Sprintf(`{"item_id": %q, "property": "opacity", "at": "60f", "value": 1}`, itemA),
	)
	doc = applyBeta(
		t,
		doc,
		"text.apply_preset",
		fmt.Sprintf(`{"item_id": %q, "preset": "fade_up", "edge": "in"}`, itemA),
	)

	if vs := timeline.Validate(doc); len(vs) != 0 {
		t.Fatalf("the document is not valid: %v", vs)
	}

	keys := doc.Items[itemA].Keyframes["opacity"]
	if len(keys) != 3 || keys[1].Easing != timeline.EasingBezier || !slices.Equal(keys[1].Bezier, curve) {
		t.Errorf("the tail keyframe does not continue the curve: %+v", keys)
	}
}
