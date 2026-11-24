package command

import (
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// SDD-002 C.90: the blur comes from the same frame plan as everything else. Over the range, and only over it, the
// plan has a second layer on top of the clip, blurred by the radius and cut by the mask of the region; the range
// is exact to the frame (a range of 20 frames from frame 30 is frames 30 to 49).
func TestBlurRegionIsInThePlanForItsRangeOnly(t *testing.T) {
	t.Parallel()

	doc := base().clip(itemA, vidTr, 0, 150, 0).d
	args := fmt.Sprintf(`{"item_id": %q, "x": 0.1, "y": 0.4, "width": 0.8, "height": 0.1, "start": "30f", `+
		`"duration": "20f", "radius": 40, "new_id": %q, "new_track_id": %q}`, itemA, itemC, blurTr)

	res, err := Default().Apply(doc, Operation{Op: "clip.blur_region", Args: []byte(args)}, Env{})
	if err != nil {
		t.Fatal(err)
	}

	for frame, want := range map[int]int{0: 1, 29: 1, 30: 2, 49: 2, 50: 1, 149: 1} {
		layers := plan.Frame(res.Doc, frame).Layers
		if len(layers) != want {
			t.Fatalf("frame %d has %d layers, want %d", frame, len(layers), want)
		}

		if want == 1 {
			if layers[0].Blur != 0 || layers[0].Mask != nil {
				t.Errorf("frame %d: the clip itself is blurred or masked: %+v", frame, layers[0])
			}

			continue
		}

		top := layers[len(layers)-1]
		if top.Blur != 40 || top.Mask == nil || top.Mask.X != 0.1 || top.Mask.Width != 0.8 {
			t.Errorf(
				"frame %d: the top layer has blur %v and mask %+v, want 40 and the region",
				frame,
				top.Blur,
				top.Mask,
			)
		}

		if layers[0].Blur != 0 {
			t.Errorf("frame %d: the clip under the blur is blurred", frame)
		}
	}

	// The copy reads the same source frames as the clip does at the same moments.
	cp := res.Doc.Items[itemC]
	if cp.Start != 30 || cp.Duration != 20 || cp.SourceIn != 30 {
		t.Errorf(
			"the blur item is at %d for %d from source frame %d, want 30, 20 and 30",
			cp.Start,
			cp.Duration,
			cp.SourceIn,
		)
	}
}

// The strong default: the least radius is 1.5% of the shorter side, and a lighter blur is refused.
func TestBlurRegionRadiusRules(t *testing.T) {
	t.Parallel()

	s := base().d.Settings
	least, def := BlurRadii(s)

	if least != 17 || def != 44 {
		t.Errorf("at 1080 pixels the least and the default radius are %v and %v, want 17 and 44", least, def)
	}

	s.Width, s.Height = 3840, 2160
	if least, def = BlurRadii(s); least != 33 || def != 87 {
		t.Errorf("at 2160 pixels the least and the default radius are %v and %v, want 33 and 87", least, def)
	}

	s.Width, s.Height = 16, 16
	if least, _ = BlurRadii(s); least != 1 {
		t.Errorf("a tiny frame has a least radius of %v, want 1", least)
	}
}
