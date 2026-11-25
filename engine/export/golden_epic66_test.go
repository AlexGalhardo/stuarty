package export

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// The golden cases of epic 66 (SDD-002 C.96 and C.97, SDD-025 section 20): a screen take and a webcam take laid
// out by recording.apply_layout on a background of recording.set_background, in a frame of each of four formats.
// The export draws the result with the layers it already has (clips with a position, a scale, a crop and a mask,
// a rectangle and a shadow), so no renderer change is under test: only that a layout reaches the pixels the same way
// as everything else, inside the frame. They are the cases recording-layout-* of testdata/golden
// (golden_alpha_test.go); regenerate with go test ./engine/export -run Alpha -update, and every regeneration is a
// reviewed change.

const (
	layoutScreenID = "itm_c660000001"
	layoutCameraID = "itm_c660000002"
)

// layoutCase is a screen take (a landscape fixture) and a camera take (a portrait fixture, so the cover crop shows)
// in a project of w by h, laid out and put on a background.
func layoutCase(w, h int, layout, background string, extra ...string) alphaCase {
	b := newCase("recording layout "+layout, rate(30, 1))
	b.doc.Settings.Width, b.doc.Settings.Height = w, h

	screen, camera := b.video(landscape), b.video(portrait)
	b.clip(b.track(), screen, 0, 60, nil)
	b.clip(b.track(), camera, 0, 60, nil)

	byAsset := map[string]string{screen: layoutScreenID, camera: layoutCameraID}

	for id := range b.doc.Items {
		it := b.doc.Items[id]
		delete(b.doc.Items, id)

		it.ID = byAsset[it.Asset]
		b.doc.Items[it.ID] = it
	}

	n := 0
	env := command.Env{NewIDs: func(prefix string) string {
		n++

		return fmt.Sprintf("%s_e66%07d", prefix, n)
	}}

	args := fmt.Sprintf(`{"layout": %q, "screen_item_id": %q, "camera_item_id": %q, "padding": 0.06%s}`,
		layout, layoutScreenID, layoutCameraID, join(extra))
	steps := []struct{ op, args string }{
		{"recording.apply_layout", args},
		{
			"recording.set_background",
			fmt.Sprintf(`{"item_id": %q, "kind": "color", "color": %q}`, layoutScreenID, background),
		},
	}

	doc := b.doc

	for _, s := range steps {
		res, err := command.Default().Apply(doc, command.Operation{Op: s.op, Args: json.RawMessage(s.args)}, env)
		if err != nil {
			panic(s.op + ": " + err.Error())
		}

		doc = res.Doc
	}

	return alphaCase{doc: doc, fixtures: b.fixtures, frames: []int{0, 40}}
}

func join(extra []string) string {
	if len(extra) == 0 {
		return ""
	}

	return ", " + strings.Join(extra, ", ")
}

// layoutCases are the four cases, one for each format.
var layoutCases = map[string]func() alphaCase{
	"recording-layout-pip-16-9": func() alphaCase {
		return layoutCase(320, 180, "pip", "#1E3A8A", `"corner": "bottom_left"`)
	},
	"recording-layout-split-1-1": func() alphaCase { return layoutCase(240, 240, "split", "#7C2D12") },
	"recording-layout-camera-focus-4-5": func() alphaCase {
		return layoutCase(192, 240, "camera_focus", "#064E3B", `"corner": "top_right"`)
	},
	"recording-layout-split-9-16": func() alphaCase { return layoutCase(180, 320, "split", "#312E81") },
}

func init() { maps.Copy(alphaCases, layoutCases) }
