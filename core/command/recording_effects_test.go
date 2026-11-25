package command

import (
	"bytes"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Acceptance criteria 1, 3 and 5 of epic 65 on the real registry.

func TestAutoZoomTwiceGivesTheSameKeyframes(t *testing.T) {
	args := cmdArgs(itemA, fiveClicks())

	first, err := Default().Apply(takeDoc().d, op2("recording.auto_zoom", args), Env{})
	if err != nil {
		t.Fatal(err)
	}

	second, err := Default().Apply(first.Doc, op2("recording.auto_zoom", args), Env{})
	if err != nil {
		t.Fatal(err)
	}

	for _, prop := range []string{"scale", "x", "y"} {
		a, b := first.Doc.Items[itemA].Keyframes[prop], second.Doc.Items[itemA].Keyframes[prop]
		if len(a) == 0 || !bytes.Equal(canonKeyframes(a), canonKeyframes(b)) {
			t.Errorf("%s: %d and %d keyframes, or they differ", prop, len(a), len(b))
		}
	}

	again, err := Default().Apply(takeDoc().d, op2("recording.auto_zoom", args), Env{})
	if err != nil || !bytes.Equal(timeline.Canonical(first.Doc), timeline.Canonical(again.Doc)) {
		t.Errorf("two runs on the same document differ: %v", err)
	}
}

func canonKeyframes(k []timeline.Keyframe) []byte {
	d := &timeline.Document{Items: map[string]timeline.Item{"a": {Keyframes: timeline.Keyframes{"x": k}}}}

	return timeline.Canonical(d)
}

func TestAutoZoomFiveClicksMakeFiveZoomsInsideTheLimits(t *testing.T) {
	res, err := Default().Apply(takeDoc().d, op2("recording.auto_zoom", cmdArgs(itemA, fiveClicks())), Env{})
	if err != nil {
		t.Fatal(err)
	}

	if res.Effective["zooms"] != 5 || res.Effective["dropped"] != 0 {
		t.Fatalf("effective %v", res.Effective)
	}

	ks := res.Doc.Items[itemA].Keyframes

	for _, k := range ks["scale"] {
		if k.Value < 1 || k.Value > 1.6 {
			t.Errorf("scale %v at %d is outside 1 to the level", k.Value, k.T)
		}
	}

	// A zoom is held at least 0.5 s: the two keyframes of the full level are at least 15 frames apart.
	var full []int

	for _, k := range ks["scale"] {
		if k.Value == 1.6 {
			full = append(full, k.T)
		}
	}

	if len(full) != 10 {
		t.Fatalf("%d keyframes at the full level, want 2 per zoom", len(full))
	}

	for i := 0; i < len(full); i += 2 {
		if full[i+1]-full[i] < 15 {
			t.Errorf("zoom %d is held %d frames", i/2, full[i+1]-full[i])
		}
	}

	// The picture covers the frame at every keyframe: x and y never move it past its edge.
	for i, k := range ks["x"] {
		s := ks["scale"][i].Value
		if room := 1920 * (s - 1) / 2; k.Value < 960-room-0.01 || k.Value > 960+room+0.01 {
			t.Errorf("x %v at scale %v leaves the frame uncovered", k.Value, s)
		}
	}
}

func TestAutoZoomIsUndone(t *testing.T) {
	doc := takeDoc().d
	before := timeline.Canonical(doc)

	res, err := Default().Apply(doc, op2("recording.auto_zoom", cmdArgs(itemA, fiveClicks())), Env{})
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(before, timeline.Canonical(res.Doc)) {
		t.Fatal("the command changed nothing")
	}

	back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
	if len(skipped) != 0 || !bytes.Equal(before, timeline.Canonical(back.Doc)) {
		t.Errorf("the inverse did not restore the document: %v", skipped)
	}
}

func TestAutoZoomWithoutClicksChangesNothing(t *testing.T) {
	doc := takeDoc().d
	before := timeline.Canonical(doc)

	for name, args := range map[string]string{
		"no resolved":  cmdArgs(itemA),
		"only moves":   cmdArgs(itemA, pointer(move(1000, 5, 5))),
		"no events":    cmdArgs(itemA, pointer()),
		"other clicks": cmdArgs(itemA, pointer(down(90000, 5, 5))),
	} {
		_, err := Default().Apply(doc, op2("recording.auto_zoom", args), Env{})
		if err == nil {
			t.Errorf("%s: no error", name)

			continue
		}

		if e := AsError(err); e == nil || (e.Code != CodeInvalidState && e.Code != CodeValidation) {
			t.Errorf("%s: %v", name, err)
		}
	}

	if !bytes.Equal(before, timeline.Canonical(doc)) {
		t.Error("a failed command changed the document")
	}
}

// The cursor is drawn through the zoom the clip has: a click in a place the zoom centres puts the cursor at the
// centre of the frame while the view is held on it, and the zoom did not move the picture off its geometry.
func TestCursorLandsWhereTheZoomLooks(t *testing.T) {
	clicks := pointer(move(0, 700, 400), move(3500, 700, 400), down(4000, 700, 400), move(4100, 700, 400),
		move(9000, 700, 400))

	zoomed, err := Default().Apply(takeDoc().d, op2("recording.auto_zoom", cmdArgs(itemA, clicks)), Env{})
	if err != nil {
		t.Fatal(err)
	}

	res, err := Default().Apply(zoomed.Doc, op2("recording.set_cursor",
		cmdArgs(itemA, `"smoothing_ms": 0`, `"keyframe_ms": 100`, clicks)), Env{NewIDs: (&seqIDs{}).next})
	if err != nil {
		t.Fatal(err)
	}

	// Frame 126 is 4.2 s: the zoom is held there (the full level from 3.6 s to 5.2 s).
	held := 126

	for _, it := range res.Doc.Items {
		if it.Type != timeline.ItemShape || it.Name != "Cursor "+itemA {
			continue
		}

		if held < it.Start || held >= it.Start+it.Duration {
			continue
		}

		x := timeline.ValueAt(it.Keyframes["x"], held-it.Start, 0)
		y := timeline.ValueAt(it.Keyframes["y"], held-it.Start, 0)
		// The focus is the click, at 700.5 of 1920 and 400.5 of 1080: with the picture held against nothing, the
		// view is centred on it, so the cursor is at the middle of the frame.
		if x < 955 || x > 965 || y < 535 || y > 545 {
			t.Errorf("the cursor is at %v, %v while the zoom is held on it", x, y)
		}

		return
	}

	t.Fatal("no cursor item covers the frame")
}

func TestCursorHiddenRemovesTheItems(t *testing.T) {
	doc := withCursor(takeDoc()).d

	res, err := Default().Apply(doc, op2("recording.set_cursor", cmdArgs(itemA, `"visible": false`)), Env{})
	if err != nil {
		t.Fatal(err)
	}

	for id, it := range res.Doc.Items {
		if it.Type == timeline.ItemShape {
			t.Errorf("%s is still there", id)
		}
	}
}
