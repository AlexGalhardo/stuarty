package image

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

func op(name, args string) command.Operation {
	return command.Operation{Op: name, Args: json.RawMessage(args)}
}

func mustApply(t *testing.T, doc *Document, ops ...command.Operation) BatchResult {
	t.Helper()

	res, err := ApplyBatch(doc, ops, testEnv(lid('k'), lid('m'), lid('n')))
	if err != nil {
		t.Fatalf("%v: %v", ops, err)
	}

	return res
}

// TestUndo is the property of SDD-002 section A.6 for the whole catalogue: the inverse of every golden case
// restores the document it started from, and the inverse of the inverse restores the result.
func TestUndo(t *testing.T) {
	for _, c := range cases {
		if c.cmd == "image.create" {
			continue // the creation of a document is the first revision and is never undone
		}

		t.Run(c.cmd+"/"+c.name, func(t *testing.T) {
			before := baseOrCustom(c)
			ReorderAll(before)

			res, err := ApplyBatch(before, []command.Operation{opOf(c)}, testEnv(c.ids...))
			if err != nil {
				return
			}

			undone, skipped := ApplyPrimitives(res.Doc, res.Inverse, nil)
			if len(skipped) > 0 {
				t.Fatalf("undo skipped %v", skipped)
			}

			if !bytes.Equal(Canonical(undone.Doc), Canonical(before)) {
				t.Fatalf("undo did not restore the document:\n%s\n%s", Canonical(undone.Doc), Canonical(before))
			}

			redone, skipped := ApplyPrimitives(undone.Doc, undone.Inverse, nil)
			if len(skipped) > 0 {
				t.Fatalf("redo skipped %v", skipped)
			}

			if !bytes.Equal(Canonical(redone.Doc), Canonical(res.Doc)) {
				t.Fatal("redo did not restore the result")
			}
		})
	}
}

func TestUndoOfABatchOfSeveralOperations(t *testing.T) {
	before := baseDoc()

	res := mustApply(t, before,
		op("layer.add_shape", `{"shape": "ellipse", "w": 50, "h": 50}`),
		op("layer.group", `{"layer_ids": ["`+lid('c')+`", "`+lid('d')+`"]}`),
		op("layer.remove", `{"layer_id": "`+lid('b')+`"}`),
		op("canvas.resize", `{"width": 640, "height": 360, "mode": "scale"}`),
	)

	undone, skipped := ApplyPrimitives(res.Doc, res.Inverse, nil)
	if len(skipped) > 0 || !bytes.Equal(Canonical(undone.Doc), Canonical(before)) {
		t.Fatalf("the batch was not undone: skipped %v", skipped)
	}
}

func TestUndoSkipsWhatAnotherActorChanged(t *testing.T) {
	base := baseDoc()

	mine := mustApply(t, base, op("layer.transform", `{"layer_id": "`+lid('d')+`", "x": 80, "y": 440}`))
	theirs := mustApply(t, mine.Doc, op("layer.transform", `{"layer_id": "`+lid('d')+`", "x": 90}`))

	guard := GuardFrom([]Touched{{Revision: 7, Actor: "agent", Diff: theirs.Diff}})

	undone, skipped := ApplyPrimitives(theirs.Doc, mine.Inverse, guard)
	if len(skipped) != 1 || skipped[0].Reason != command.SkipFieldChanged || skipped[0].Fields[0] != "x" ||
		skipped[0].ByRevision == nil || *skipped[0].ByRevision != 7 || skipped[0].ByActor != "agent" {
		t.Fatalf("the change of the other actor was not reported: %+v", skipped)
	}

	got, _ := Find(undone.Doc, lid('d'))
	if got.X != 90 || got.Y != 500 {
		t.Fatalf("x stays with the other actor and y goes back: got %d,%d", got.X, got.Y)
	}
}

func TestUndoOfALayerAnotherActorRemoved(t *testing.T) {
	base := baseDoc()

	mine := mustApply(t, base, op("layer.set", `{"layer_id": "`+lid('d')+`", "name": "Renamed"}`))
	theirs := mustApply(t, mine.Doc, op("layer.remove", `{"layer_id": "`+lid('d')+`"}`))

	undone, skipped := ApplyPrimitives(
		theirs.Doc,
		mine.Inverse,
		GuardFrom([]Touched{{Revision: 3, Actor: "a", Diff: theirs.Diff}}),
	)
	if len(skipped) != 1 || skipped[0].Reason != command.SkipObjectMissing {
		t.Fatalf("want one object_missing, got %+v", skipped)
	}

	if !bytes.Equal(Canonical(undone.Doc), Canonical(theirs.Doc)) {
		t.Fatal("a skipped undo must leave the document alone")
	}
}

func TestUndoOfARemovalIntoAGroupThatIsGone(t *testing.T) {
	base := baseDoc()

	mine := mustApply(t, base, op("layer.remove", `{"layer_id": "`+lid('f')+`"}`))
	theirs := mustApply(t, mine.Doc, op("layer.remove", `{"layer_id": "`+lid('e')+`"}`))

	undone, skipped := ApplyPrimitives(theirs.Doc, mine.Inverse, nil)
	if len(skipped) != 1 || skipped[0].Reason != command.SkipInvalidResult {
		t.Fatalf("a layer cannot come back into a group that is gone: %+v", skipped)
	}

	if !bytes.Equal(Canonical(undone.Doc), Canonical(theirs.Doc)) {
		t.Fatal("a skipped undo must leave the document alone")
	}
}

func TestUndoRefusesAPutOverAnExistingLayer(t *testing.T) {
	base := baseDoc()
	removed := mustApply(t, base, op("layer.remove", `{"layer_id": "`+lid('d')+`"}`))

	again := mustApply(t, base, op("layer.set", `{"layer_id": "`+lid('d')+`", "name": "Other"}`))

	_, skipped := ApplyPrimitives(again.Doc, removed.Inverse, nil)
	if len(skipped) != 1 || skipped[0].Reason != command.SkipIDExists {
		t.Fatalf("want one id_exists, got %+v", skipped)
	}
}

func TestConflict(t *testing.T) {
	base := baseDoc()
	theirs := mustApply(t, base, op("layer.transform", `{"layer_id": "`+lid('c')+`", "x": 1}`))
	resized := mustApply(t, base, op("canvas.resize", `{"width": 10, "height": 10}`))
	removed := mustApply(t, base, op("layer.remove", `{"layer_id": "`+lid('e')+`"}`))

	for _, tc := range []struct {
		name     string
		ops      []command.Operation
		changes  []command.Change
		want     string
		wantKind string
	}{
		{"same layer", []command.Operation{op("layer.set", `{"layer_id": "`+lid('c')+`", "name": "x"}`)}, theirs.Diff, lid('c'), KindLayerUpdated},
		{"another layer", []command.Operation{op("layer.set", `{"layer_id": "`+lid('d')+`", "name": "x"}`)}, theirs.Diff, "", ""},
		{"the second operation names it", []command.Operation{
			op("layer.set", `{"layer_id": "`+lid('d')+`", "name": "x"}`),
			op("layer.align", `{"layer_ids": ["`+lid('d')+`"], "to": "`+lid('c')+`", "horizontal": "left"}`),
		}, theirs.Diff, lid('c'), KindLayerUpdated},
		{"a group that was removed", []command.Operation{op("layer.add_fill", `{"color": "#000000", "parent": "`+lid('e')+`"}`)}, removed.Diff, lid('e'), KindLayerRemoved},
		{"the canvas", []command.Operation{op("canvas.resize", `{"width": 20, "height": 20}`)}, resized.Diff, docID, KindDocumentUpdated},
		{"a layer op is not about the canvas", []command.Operation{op("layer.set", `{"layer_id": "`+lid('d')+`", "name": "x"}`)}, resized.Diff, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Conflicts(tc.ops, tc.changes)

			switch {
			case tc.want == "" && got != nil:
				t.Fatalf("no conflict expected, got %+v", got)
			case tc.want != "" && (got == nil || got.ID != tc.want || got.Kind != tc.wantKind):
				t.Fatalf("want a conflict on %s (%s), got %+v", tc.want, tc.wantKind, got)
			}
		})
	}
}

func TestDryRun(t *testing.T) {
	base := baseDoc()
	frozen := Canonical(base)

	res := mustApply(t, base, op("layer.remove", `{"layer_id": "`+lid('d')+`"}`))
	if res.NoOp() || len(res.Diff) == 0 || DescribeDiff(res.Doc, res.Diff) == "nothing changed" {
		t.Fatalf("a removal is a change: %+v", res.Diff)
	}

	if !bytes.Equal(frozen, Canonical(base)) {
		t.Fatal("applying a batch must not change the document it was given")
	}

	same := mustApply(t, base, op("layer.set", `{"layer_id": "`+lid('d')+`", "name": "Bar"}`))
	if !same.NoOp() || DescribeDiff(same.Doc, same.Diff) != "nothing changed" {
		t.Fatalf("a batch that changes nothing is a no-op, got %+v", same.Diff)
	}
}

func TestBatchIsAtomic(t *testing.T) {
	base := baseDoc()
	frozen := Canonical(base)

	_, err := ApplyBatch(base, []command.Operation{
		op("layer.remove", `{"layer_id": "`+lid('d')+`"}`),
		op("layer.remove", `{"layer_id": "`+lid('d')+`"}`),
	}, testEnv())

	e := command.AsError(err)
	if e.Code != command.CodeNotFound || e.OpIndex == nil || *e.OpIndex != 1 {
		t.Fatalf("the second operation fails with its index: %+v", e)
	}

	if !bytes.Equal(frozen, Canonical(base)) {
		t.Fatal("a failed batch must leave the document alone")
	}

	if _, err := ApplyBatch(base, nil, testEnv()); command.AsError(err).Code != command.CodeValidation {
		t.Fatal("an empty batch is refused")
	}

	if _, err := ApplyBatch(
		base,
		[]command.Operation{op("video.cut", `{}`)},
		testEnv(),
	); command.AsError(
		err,
	).Code != command.CodeUnknownCommand {
		t.Fatal("a video operation is unknown to the image registry")
	}
}

func TestBatchIsDeterministic(t *testing.T) {
	ops := []command.Operation{
		op("layer.add_text", `{"text": "Hello"}`),
		op("layer.duplicate", `{"layer_id": "`+lid('d')+`"}`),
		op("layer.group", `{"layer_ids": ["`+lid('c')+`", "`+lid('d')+`"]}`),
	}

	a, err := ApplyBatch(baseDoc(), ops, testEnv(lid('k'), lid('m'), lid('n')))
	if err != nil {
		t.Fatal(err)
	}

	b, err := ApplyBatch(baseDoc(), ops, testEnv(lid('k'), lid('m'), lid('n')))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(Canonical(a.Doc), Canonical(b.Doc)) || !bytes.Equal(pretty(t, a.Diff), pretty(t, b.Diff)) {
		t.Fatal("the same batch gave two results")
	}
}
