package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The golden case of epic 65 (SDD-002 C.93 to C.95, SDD-025 section 17): a screen take, zoomed on its clicks, with
// the smoothed cursor and the click highlights, drawn by the export. The document is the one the three commands
// make from the pointer events; the export draws it with the layers it already has (a clip with x, y and scale
// keyframes and shapes), so no renderer change is under test, only that the effects reach the pixels the same way
// as everything else. It is the case screen-take-effects of testdata/golden (golden_alpha_test.go), so the preview
// against export specs of the editor read it too. Regenerate with go test ./engine/export -run Alpha -update; every
// regeneration is a reviewed change.

const takeClipID = "itm_c000000001"

// takePointer is the pointer file of a 320 by 180 screen: two clicks, 1 s apart, and the path between them.
const takePointer = `{"width": 320, "height": 180, "events": [
	{"t_ms": 0, "kind": "move", "x": 20, "y": 20},
	{"t_ms": 400, "kind": "move", "x": 120, "y": 40},
	{"t_ms": 800, "kind": "move", "x": 240, "y": 60},
	{"t_ms": 800, "kind": "down", "button": "left", "x": 240, "y": 60},
	{"t_ms": 880, "kind": "up", "button": "left", "x": 240, "y": 60},
	{"t_ms": 1300, "kind": "move", "x": 150, "y": 100},
	{"t_ms": 1800, "kind": "move", "x": 80, "y": 120},
	{"t_ms": 1800, "kind": "down", "button": "left", "x": 80, "y": 120},
	{"t_ms": 1880, "kind": "up", "button": "left", "x": 80, "y": 120},
	{"t_ms": 2500, "kind": "move", "x": 200, "y": 150}]}`

// takeClicks is the same file with the presses only, as the service resolves it for the zoom and the highlight.
const takeClicks = `{"width": 320, "height": 180, "events": [
	{"t_ms": 800, "kind": "down", "button": "left", "x": 240, "y": 60},
	{"t_ms": 1800, "kind": "down", "button": "left", "x": 80, "y": 120}]}`

// screenTake is the clip of a screen on a 320 by 180 project; withEffects runs the three commands.
func screenTake(withEffects, withOverlays bool) alphaCase {
	b := newCase("screen take", rate(30, 1))
	b.doc.Settings.Width, b.doc.Settings.Height = 320, 180
	asset := b.video(landscape)
	b.clip(b.track(), asset, 0, 90, nil)

	var oldID string
	for id := range b.doc.Items {
		oldID = id
	}

	it := b.doc.Items[oldID]
	delete(b.doc.Items, oldID)
	it.ID = takeClipID
	b.doc.Items[takeClipID] = it

	doc := b.doc

	if withEffects {
		n := 0
		env := command.Env{NewIDs: func(prefix string) string {
			n++

			return fmt.Sprintf("%s_e65%07d", prefix, n)
		}}

		steps := []struct{ op, args string }{
			{
				"recording.auto_zoom",
				fmt.Sprintf(`{"item_id": %q, "level": 1.8, "lead_ms": 200, "resolved": %s}`, takeClipID, takeClicks),
			},
		}

		if withOverlays {
			steps = append(steps,
				struct{ op, args string }{
					"recording.set_cursor",
					fmt.Sprintf(
						`{"item_id": %q, "size": 14, "smoothing_ms": 120, "resolved": %s}`,
						takeClipID,
						takePointer,
					),
				},
				struct{ op, args string }{
					"recording.set_click_highlight",
					fmt.Sprintf(`{"item_id": %q, "style": "wave", "size": 40, "resolved": %s}`, takeClipID, takeClicks),
				})
		}

		for _, s := range steps {
			res, err := command.Default().Apply(doc, command.Operation{Op: s.op, Args: json.RawMessage(s.args)}, env)
			if err != nil {
				panic(s.op + ": " + err.Error())
			}

			doc = res.Doc
		}
	}

	// rest, easing in, on the first click, held on the second zoom, easing out
	return alphaCase{doc: doc, fixtures: b.fixtures, frames: []int{0, 12, 26, 40, 60, 80}}
}

// prepared is the document of a case with its generated media ingested, as loadDir leaves it.
func prepared(t *testing.T, c alphaCase) *timeline.Document {
	t.Helper()

	for id, spec := range c.fixtures {
		useAsset(c.doc, id, ingestClip(t, spec))
	}

	return c.doc
}

// changed counts the pixels two frames of the same size draw differently.
func changed(a, b []byte) int {
	n := 0

	for i := 0; i+3 < len(a); i += 4 {
		if !bytes.Equal(a[i:i+4], b[i:i+4]) {
			n++
		}
	}

	return n
}

// The three effects each reach the pixels: the zoom changes the picture, the cursor and the highlight change a small
// part of it, and the rest state at the ends of the take is the plain clip.
func TestEpic65EffectsAreDrawn(t *testing.T) {
	setup(t)

	plain, zoomed, full := screenTake(false, false), screenTake(true, false), screenTake(true, true)
	frames := full.frames

	render := func(name string, c alphaCase) [][]byte { return renderDoc(t, prepared(t, c), name, 0, frames) }
	pPlain, pZoom, pFull := render("plain", plain), render("zoomed", zoomed), render("full", full)
	total := full.doc.Settings.Width * full.doc.Settings.Height

	for i, f := range frames {
		zoomDiff, overlayDiff := changed(pPlain[i], pZoom[i]), changed(pZoom[i], pFull[i])

		if f == 0 && zoomDiff != 0 {
			t.Errorf("frame %d: the zoom changed %d pixels of the rest state", f, zoomDiff)
		} else if f != 0 && zoomDiff < total/4 {
			t.Errorf("frame %d: the zoom changed only %d of %d pixels", f, zoomDiff, total)
		}

		// The cursor is out of the frame while the zoom looks at the other click (frame 12): the others show it.
		if (overlayDiff == 0 && f != 12) || overlayDiff > total/8 {
			t.Errorf("frame %d: the cursor and the highlight changed %d of %d pixels", f, overlayDiff, total)
		}
	}

	// The highlight of the first click is on its frame: the same document with the cursor only differs from it.
	cursorOnly, err := command.Default().Apply(prepared(t, screenTake(true, false)), command.Operation{
		Op: "recording.set_cursor",
		Args: json.RawMessage(fmt.Sprintf(`{"item_id": %q, "size": 14, "smoothing_ms": 120, "resolved": %s}`,
			takeClipID, takePointer)),
	}, command.Env{NewIDs: func(prefix string) string { return prefix + "_e65aaaaaaa" }})
	if err != nil {
		t.Fatal(err)
	}

	with := renderDoc(t, prepared(t, screenTake(true, true)), "with-highlight", 0, []int{26})
	without := renderDoc(t, cursorOnly.Doc, "cursor-only", 0, []int{26})

	if changed(with[0], without[0]) == 0 {
		t.Error("frame 26: the highlight of the first click is not drawn")
	}
}
