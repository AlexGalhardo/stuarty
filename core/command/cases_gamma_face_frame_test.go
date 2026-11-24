package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// faceTrackArgs is the resolved face track of a 1920x1080 source: n samples 200 ms apart, the face moving
// from the left third to the right third.
func faceTrackArgs(n int) string {
	samples := make([]string, n)
	for i := range n {
		cx := 2500 + 5000*i/max(n-1, 1)
		samples[i] = fmt.Sprintf(
			`{"t": %d, "faces": [{"x": %d, "y": 2500, "w": 1500, "h": 3300, "s": 900}]}`,
			i*200,
			cx-750,
		)
	}

	return fmt.Sprintf(`"resolved": {"track": {"version": 1, "model": "yunet-2023mar", "provider": "cpu", `+
		`"source": {"width": 1920, "height": 1080}, "fps": 5, "start_ms": 0, "end_ms": %d, "samples": [%s]}}`,
		n*200, strings.Join(samples, ", "))
}

func init() {
	ff := func(name, item string, b func() *B, rest ...string) {
		parts := append([]string{fmt.Sprintf(`"item_id": %q`, item)}, rest...)

		gc("clip.face_frame", name, b, op("clip.face_frame", "{"+strings.Join(parts, ", ")+"}"))
	}

	track := faceTrackArgs(16)
	clip := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 90, 0, mods...) }
	}
	wide := func(d *timeline.Document) { d.Settings.Width, d.Settings.Height = 1920, 1080 }

	ff("ok_follows_the_face_in_a_vertical_frame", itemA, clip(), track)
	ff("ok_starts_inside_the_source", itemA, clip(func(it *timeline.Item) { it.SourceIn = 30 }), track)
	ff("ok_options_given", itemA, clip(), `"smoothing_ms": 100`, `"dead_zone": 0`, `"max_pan": 3000`, track)
	ff("ok_replaces_crop_keyframes", itemA, clip(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"crop_left": {kf(0, 0.1), kf(30, 0.2)}}
		it.Crop = &timeline.Crop{Left: 0.1}
	}), track)
	ff("ok_same_aspect_pans_nothing", itemA, func() *B { return clip()().with(wide) }, track)
	ff("err_validation_resolved_missing", itemA, clip())
	ff("err_validation_option_out_of_range", itemA, clip(), `"max_pan": 5`, track)
	ff("err_validation_track_invalid", itemA, clip(), `"resolved": {"track": {"version": 9}}`)
	ff("err_not_found", "itm_zzzzzzzzzz", clip(), track)
	ff("err_type_mismatch_not_a_clip", itemA, func() *B { return base().audio(itemA, 0, 90, 0) }, track)
	ff("err_invalid_state_reversed_clip", itemA, clip(func(it *timeline.Item) { it.Reverse = true }), track)
}
