package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// SDD-002 C.90: the blur comes from the same frame plan as everything else. Over the range, and only over it, the
// plan has a second layer on top of the clip, blurred by the radius and cut by the mask of the region; the range is
// exact to the frame (20 frames from frame 30 are frames 30 to 49) and the copy reads the same source frames.
func TestBlurRegionIsInThePlanForItsRangeOnly(t *testing.T) {
	t.Parallel()

	const (
		video = "trk_f6g7h8j9k0"
		asset = "ast_3c5v8n2w6d"
		clip  = "itm_m1n2p3q4r5"
		blur  = "itm_s6t7v8w9x0"
	)

	doc := &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion,
		ID:            "prj_7k2m9x4q1b",
		Name:          "Blur plan",
		Settings:      timeline.DefaultSettings(),
		Assets: map[string]timeline.Asset{asset: {
			ID: asset, Hash: "sha256:" + fmt.Sprintf("%064x", 9), Kind: timeline.AssetVideo, Name: "a.mov",
			FrameRate: timeline.Rational{Num: 30, Den: 1}, FrameCount: 900, Width: 1080, Height: 1920,
		}},
		Tracks: map[string]timeline.Track{video: {ID: video, Kind: timeline.TrackVisual, Name: "Video", Order: "a1"}},
		Items:  map[string]timeline.Item{},
	}
	it := timeline.NewItem(clip, timeline.ItemClip, video, 0, 150)
	it.Asset = asset
	doc.Items[clip] = it

	args, err := json.Marshal(map[string]any{
		"item_id": clip, "x": 0.1, "y": 0.4, "width": 0.8, "height": 0.1, "start": "30f", "duration": "20f",
		"radius": 40, "new_id": blur, "new_track_id": "trk_b2c3d4e5f6",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, aerr := command.Default().Apply(doc, command.Operation{Op: "clip.blur_region", Args: args}, command.Env{})
	if aerr != nil {
		t.Fatal(aerr)
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

	cp := res.Doc.Items[blur]
	if cp.Start != 30 || cp.Duration != 20 || cp.SourceIn != 30 {
		t.Errorf(
			"the blur item is at %d for %d from source frame %d, want 30, 20 and 30",
			cp.Start,
			cp.Duration,
			cp.SourceIn,
		)
	}
}
