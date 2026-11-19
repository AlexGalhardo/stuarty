package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Section 6 of the real-use plan, through the registry the service calls:
// a valid edit reports the change and undo restores it; an empty, extreme or
// unknown operation is a documented error and leaves the document alone.
func TestQACoreSection6EditSurface(t *testing.T) {
	t.Parallel()

	reg := Default()
	doc := base().d
	before := bytes.Clone(timeline.Canonical(doc))
	res, err := reg.Apply(doc, op2("project.rename", `{"name": "Lançamento"}`), Env{})
	if err != nil {
		t.Fatalf("valid rename: %v", err)
	}

	if !bytes.Equal(timeline.Canonical(doc), before) {
		t.Fatal("Apply changed the input document")
	}

	if res.Doc.Name != "Lançamento" || res.Effective["name"] != "Lançamento" {
		t.Fatalf("effective %+v name %q", res.Effective, res.Doc.Name)
	}

	if len(res.Diff) != 1 || res.Diff[0].Kind != KindProjectUpdated {
		t.Fatalf("diff %+v", res.Diff)
	}

	back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
	if len(skipped) != 0 || !bytes.Equal(timeline.Canonical(back.Doc), before) {
		t.Fatalf("undo skipped %v", skipped)
	}

	refuse := func(name, op, args, code string) {
		t.Helper()

		fresh := base().d
		prior := bytes.Clone(timeline.Canonical(fresh))
		_, err := reg.Apply(fresh, op2(op, args), Env{})
		e := AsError(err)
		if err == nil || e.Code != code || e.Code == CodeInternal {
			t.Errorf("%s: got code=%s message=%q", name, e.Code, e.Message)
		}

		if !bytes.Equal(timeline.Canonical(fresh), prior) {
			t.Errorf("%s changed the document", name)
		}
	}

	refuse("unknown op", "no.such", `{}`, CodeUnknownCommand)
	refuse("empty args", "project.rename", `{}`, CodeValidation)
	refuse("null args", "project.rename", `null`, CodeValidation)
	refuse("args array", "project.rename", `[]`, CodeValidation)
	refuse("unknown field", "project.rename", `{"name": "a", "extra": 1}`, CodeValidation)
	refuse("extreme length", "project.rename", `{"name": "`+strings.Repeat("x", 10000)+`"}`, CodeValidation)
	refuse("negative time", "text.add", `{"start": "-5s", "duration": "2s", "text": "Hi"}`, CodeInvalidTime)
	refuse("huge number", "settings.set", `{"width": 1e308}`, CodeValidation)
	refuse("bare time", "text.add", `{"start": 12, "duration": "2s", "text": "Hi"}`, CodeInvalidTime)

	if _, err := reg.ApplyBatch(base().d, nil, Env{}); AsError(err).Code != CodeValidation {
		t.Errorf("empty batch: %v", err)
	}

	// A 10 s ripple-all delete moves a marker after the cut and drops one inside it.
	marked := base().text(itemA, vidTr, 0, 300).text(itemB, vidTr, 300, 120).with(func(d *timeline.Document) {
		d.Markers = []timeline.Marker{
			{Frame: 150, Label: "inside"},
			{Frame: 300, Label: "edge"},
			{Frame: 450, Label: "after"},
		}
	}).d
	moved, err := reg.Apply(marked, op2("item.delete", `{"item_ids": ["`+itemA+`"], "ripple": "all"}`), Env{})
	if err != nil {
		t.Fatalf("ripple all: %v", err)
	}

	if _, ok := moved.Doc.Items[itemB]; !ok || moved.Doc.Items[itemB].Start != 0 {
		t.Fatalf("later item start %v", moved.Doc.Items[itemB].Start)
	}

	wantMarkers := []timeline.Marker{{Frame: 0, Label: "edge"}, {Frame: 150, Label: "after"}}
	if len(moved.Doc.Markers) != len(wantMarkers) {
		t.Fatalf("markers %+v", moved.Doc.Markers)
	}

	for i, m := range wantMarkers {
		got := moved.Doc.Markers[i]
		if got.Frame != m.Frame || got.Label != m.Label {
			t.Fatalf("marker %d = %+v, want %+v", i, got, m)
		}
	}

	if moved.Effective["markers_removed"] != 1 || moved.Effective["markers_moved"] != 2 {
		t.Fatalf("effective %+v", moved.Effective)
	}

	kept := base().text(itemA, vidTr, 0, 300).text(itemB, vidTr, 300, 120).with(func(d *timeline.Document) {
		d.Markers = []timeline.Marker{{Frame: 450, Label: "after"}}
	}).d
	track, err := reg.Apply(kept, op2("item.delete", `{"item_ids": ["`+itemA+`"], "ripple": "track"}`), Env{})
	if err != nil {
		t.Fatalf("ripple track: %v", err)
	}

	if len(track.Doc.Markers) != 1 || track.Doc.Markers[0].Frame != 450 {
		t.Fatalf("ripple track moved markers: %+v", track.Doc.Markers)
	}

	// A fade out on the last picture is not an empty transition, including while a title runs on.
	last := base().clip(itemA, vidTr, 0, 300, 0, func(it *timeline.Item) {
		it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 60}
	}).d
	if n := countCode(lint.Lint(last, lint.Env{}, lint.Fast), "L_TRANSITION_EMPTY"); n != 0 {
		t.Fatalf("fade out at the end: %d findings", n)
	}

	withTitle := base().clip(itemA, vidTr, 0, 300, 0, func(it *timeline.Item) {
		it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 60}
	}).text(itemB, capTr, 200, 400).d
	if n := countCode(lint.Lint(withTitle, lint.Env{}, lint.Fast), "L_TRANSITION_EMPTY"); n != 0 {
		t.Fatalf("fade out while a title runs on: %d findings", n)
	}

	early := base().clip(itemA, vidTr, 0, 300, 0, func(it *timeline.Item) {
		it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 60}
	}).clip(itemB, vidTr, 600, 60, 0).d
	if n := countCode(lint.Lint(early, lint.Env{}, lint.Fast), "L_TRANSITION_EMPTY"); n != 1 {
		t.Fatalf("fade out before a later clip: %d findings, want 1", n)
	}
}

func countCode(fs []lint.Finding, code string) int {
	n := 0

	for i := range fs {
		if fs[i].Code == code {
			n++
		}
	}

	return n
}
