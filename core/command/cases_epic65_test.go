package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The golden cases of the commands that finish a screen take (SDD-002 C.93 to C.95): a 1920 by 1080 project of
// 30 fps with one clip of a 1920 by 1080 recording that lasts 60 s.

const takeAsset = "ast_t4k3a5s7e9"

// takeDoc is the project of a screen take: the clip itemA on vidTr plays the whole asset.
func takeDoc(mods ...func(*timeline.Item)) *B {
	return base().with(func(d *timeline.Document) {
		d.Settings.Width, d.Settings.Height = 1920, 1080
		d.Assets[takeAsset] = timeline.Asset{
			ID: takeAsset, Hash: "sha256:" + strings.Repeat("c3", 32), Kind: timeline.AssetVideo, Name: "capture.mp4",
			FrameRate: timeline.Rational{Num: 30, Den: 1}, FrameCount: 3600, Width: 1920, Height: 1080,
		}
	}).clip(itemA, vidTr, 0, 1800, 0, append([]func(*timeline.Item){func(it *timeline.Item) { it.Asset = takeAsset }}, mods...)...)
}

func takeWith(mods ...func(*timeline.Item)) func() *B { return func() *B { return takeDoc(mods...) } }

// down is a button press line of the resolved events.
func down(ms, x, y int) string {
	return fmt.Sprintf(`{"t_ms": %d, "kind": "down", "button": "left", "x": %d, "y": %d}`, ms, x, y)
}

// move is a position line.
func move(ms, x, y int) string {
	return fmt.Sprintf(`{"t_ms": %d, "kind": "move", "x": %d, "y": %d}`, ms, x, y)
}

// pointer is the resolved member of the three commands.
func pointer(events ...string) string {
	return `"resolved": {"width": 1920, "height": 1080, "events": [` + strings.Join(events, ", ") + `]}`
}

// fiveClicks are five presses ten seconds apart, each in another place.
func fiveClicks() string {
	return pointer(down(4000, 300, 200), down(14000, 1500, 300), down(24000, 960, 540), down(34000, 400, 900),
		down(44000, 1700, 950))
}

func cmdArgs(item string, rest ...string) string {
	return "{" + strings.Join(append([]string{fmt.Sprintf(`"item_id": %q`, item)}, rest...), ", ") + "}"
}

func init() {
	z := func(name, item string, b func() *B, rest ...string) {
		gc("recording.auto_zoom", name, b, op("recording.auto_zoom", cmdArgs(item, rest...)))
	}

	z("ok_five_separated_clicks", itemA, takeWith(), fiveClicks())
	z("ok_clicks_in_one_place_make_one_zoom", itemA, takeWith(),
		pointer(down(4000, 900, 500), down(4800, 960, 520), down(6200, 1000, 500)))
	z("ok_close_clicks_chain_into_a_pan", itemA, takeWith(), pointer(down(4000, 200, 200), down(7500, 1700, 900)))
	z("ok_options_given", itemA, takeWith(), `"level": 2.2`, `"lead_ms": 0`, `"ramp_ms": 500`, `"hold_ms": 800`,
		`"merge_ms": 0`, fiveClicks())
	z("ok_starts_inside_the_source", itemA, takeWith(func(it *timeline.Item) { it.SourceIn, it.Duration = 600, 900 }),
		pointer(down(24000, 300, 200), down(44000, 1500, 300)))
	z("ok_replaces_zoom_keyframes", itemA, takeWith(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{
			"scale":   {kf(0, 1), kf(60, 1.5)},
			"x":       {kf(0, 960), kf(60, 800)},
			"opacity": {kf(0, 1), kf(60, 0.5)},
		}
	}), fiveClicks())
	z("ok_vertical_frame", itemA, func() *B {
		return takeDoc().with(func(d *timeline.Document) { d.Settings.Width, d.Settings.Height = 1080, 1920 })
	}, fiveClicks())
	z("ok_more_zooms_than_max_drops_the_rest", itemA, takeWith(), `"max_zooms": 2`, fiveClicks())
	z("err_validation_resolved_missing", itemA, takeWith())
	z("err_validation_option_out_of_range", itemA, takeWith(), `"level": 3.5`, `"hold_ms": 100`, fiveClicks())
	z(
		"err_validation_resolved_invalid",
		itemA,
		takeWith(),
		`"resolved": {"width": 1920, "height": 1080, "events": [{"t_ms": 5, "kind": "down", "button": "left", "x": 4000, "y": 0}]}`,
	)
	z("err_not_found", "itm_zzzzzzzzzz", takeWith(), fiveClicks())
	z("err_type_mismatch_not_a_clip", itemA, func() *B { return base().audio(itemA, 0, 90, 0) }, fiveClicks())
	z("err_invalid_state_no_clicks", itemA, takeWith(), pointer(move(1000, 10, 10), move(2000, 20, 20)))
	z("err_invalid_state_reversed_clip", itemA, takeWith(func(it *timeline.Item) { it.Reverse = true }), fiveClicks())
	z(
		"err_invalid_state_cropped_clip",
		itemA,
		takeWith(func(it *timeline.Item) { it.Crop = &timeline.Crop{Left: 0.1} }),
		fiveClicks(),
	)
}

// randomClicks is a resolved member with a few presses of a 1080 by 1920 source in the first seconds of a clip.
func randomClicks(p *docPick) string {
	events := make([]string, 0, 6)

	for i := range 2 + p.n(4) {
		events = append(events, fmt.Sprintf(`{"t_ms": %d, "kind": "down", "button": "left", "x": %d, "y": %d}`,
			i*700+p.n(600), p.n(1080), p.n(1920)))
	}

	return `"resolved": {"width": 1080, "height": 1920, "events": [` + strings.Join(events, ", ") + `]}`
}

// walk is n positions 500 ms apart, the pointer crossing the screen from the top left to the bottom right.
func walk(n int) []string {
	out := make([]string, n)
	for i := range n {
		out[i] = move(i*500, 10+i*1900/n, 10+i*1060/n)
	}

	return out
}

// withCursor adds the cursor items of a take made earlier: a track named Cursor and one item on it.
func withCursor(b *B) *B {
	return b.with(func(d *timeline.Document) {
		d.Tracks["trk_cr00000001"] = timeline.Track{
			ID:    "trk_cr00000001",
			Kind:  timeline.TrackVisual,
			Name:  CursorTrackName,
			Order: "a05",
		}
		it := timeline.NewItem("itm_cr0000000a", timeline.ItemShape, "trk_cr00000001", 0, 60)
		it.Name, it.Shape, it.Width, it.Height = "Cursor "+itemA, "ellipse", 20, 20
		d.Items[it.ID] = it
	})
}

func init() {
	cu := func(name, item string, b func() *B, rest ...string) {
		gc("recording.set_cursor", name, b, op("recording.set_cursor", cmdArgs(item, rest...)))
	}
	cuIDs := func(name string, ids []string, b func() *B, rest ...string) {
		gcIDs("recording.set_cursor", name, b, op("recording.set_cursor", cmdArgs(itemA, rest...)), ids...)
	}
	newTrack, one := "trk_n3w7r4ck21", "itm_cr00000001"
	shortClip := takeWith(func(it *timeline.Item) { it.Duration = 150 })

	short := pointer(append(walk(8), down(3600, 900, 500))...)

	cuIDs("ok_follows_the_pointer", []string{newTrack, one}, shortClip, short)
	cuIDs("ok_ring_style_and_size", []string{newTrack, one}, shortClip, `"style": "ring"`, `"size": 40`,
		`"color": "#33CCFF"`, `"smoothing_ms": 0`, `"keyframe_ms": 200`, short)
	cuIDs("ok_follows_the_zoom", []string{newTrack, one}, takeWith(func(it *timeline.Item) {
		it.Duration = 150
		it.Keyframes = timeline.Keyframes{
			"scale": {kf(0, 1), kf(30, 2, "ease_in_out"), kf(120, 2)},
			"x":     {kf(0, 960), kf(30, 200, "ease_in_out"), kf(120, 200)},
			"y":     {kf(0, 540), kf(30, 100, "ease_in_out"), kf(120, 100)},
		}
	}), short)
	cuIDs("ok_long_clip_is_cut_in_segments", []string{
		newTrack, "itm_cr00000001", "itm_cr00000002", "itm_cr00000003",
		"itm_cr00000004", "itm_cr00000005", "itm_cr00000006", "itm_cr00000007",
	}, takeWith(), `"keyframe_ms": 100`,
		pointer(walk(110)...))
	cuIDs("ok_replaces_the_previous_cursor", []string{one}, func() *B { return withCursor(shortClip()) }, short)
	cu("ok_hidden_removes_the_cursor", itemA, func() *B { return withCursor(takeDoc()) }, `"visible": false`)
	cu("err_validation_resolved_missing", itemA, takeWith())
	cu("err_validation_option_out_of_range", itemA, takeWith(), `"smoothing_ms": 5000`, short)
	cu("err_not_found", "itm_zzzzzzzzzz", takeWith(), short)
	cu("err_type_mismatch_not_a_clip", itemA, func() *B { return base().audio(itemA, 0, 90, 0) }, short)
	cu("err_invalid_state_no_positions", itemA, takeWith(), pointer())
	cuIDs("err_limit_items", []string{newTrack, one}, func() *B {
		return takeDoc().with(func(d *timeline.Document) {
			d.Tracks["trk_fe00000001"] = timeline.Track{ID: "trk_fe00000001", Kind: timeline.TrackVisual, Order: "a9"}
		}).manyItems("trk_fe00000001", timeline.MaxItems-1)
	}, short)
}
