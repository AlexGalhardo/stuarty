package command

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// revStep is one revision of a test history: the document after it and the inverse it stored.
type revStep struct {
	doc     *timeline.Document
	inverse []Primitive
}

// history applies the batches in order and returns the revisions: index 0 is the starting
// document with no inverse, index i is the document after batch i.
func history(t *testing.T, start *timeline.Document, batches [][]Operation) []revStep {
	t.Helper()

	r := fullRegistry()
	ids := &seqIDs{}
	out := []revStep{{doc: start}}

	for i, ops := range batches {
		res, err := r.ApplyBatch(out[len(out)-1].doc, ops, Env{NewIDs: ids.next})
		if err != nil {
			t.Fatalf("batch %d: %v", i, err)
		}

		out = append(out, revStep{doc: res.Doc, inverse: res.Inverse})
	}

	return out
}

// checkRevertTo asserts the whole contract of a revert from revision n to revision k.
func checkRevertTo(t *testing.T, steps []revStep, n, k int) {
	t.Helper()

	current, target := steps[n].doc, steps[k].doc

	var inverses [][]Primitive
	for i := n; i > k; i-- {
		inverses = append(inverses, steps[i].inverse)
	}

	rebuilt, err := RebuildAt(current, inverses)
	if err != nil {
		t.Fatalf("rebuild r%d from r%d: %v", k, n, err)
	}

	if !bytes.Equal(timeline.Canonical(rebuilt), timeline.Canonical(target)) {
		t.Fatalf("the rebuilt r%d differs from the stored one", k)
	}

	before := timeline.Canonical(current)
	res := Revert(current, rebuilt)

	if !bytes.Equal(timeline.Canonical(current), before) {
		t.Fatalf("revert to r%d changed its input", k)
	}

	if !bytes.Equal(timeline.Canonical(res.Doc), timeline.Canonical(target)) {
		t.Fatalf("revert to r%d: the document differs from r%d", k, k)
	}

	if vs := timeline.Validate(res.Doc); len(vs) != 0 {
		t.Fatalf("revert to r%d is invalid: %v", k, vs)
	}

	if res.NoOp() != (len(Diff(current, target)) == 0) {
		t.Fatalf("revert to r%d: NoOp is %v with %d changes", k, res.NoOp(), len(Diff(current, target)))
	}

	// The revert undone leaves the document it started from.
	back, skipped := ApplyPrimitives(res.Doc, reversePrimitives(res.Inverse), nil)
	if len(skipped) != 0 {
		t.Fatalf("undo of the revert to r%d skipped %+v", k, skipped)
	}

	if !bytes.Equal(timeline.Canonical(back.Doc), before) {
		t.Fatalf("revert to r%d then undo does not give the document before the revert", k)
	}
}

func scenarioBatches() [][]Operation {
	return [][]Operation{
		{op2("item.move", `{"item_id":"`+itemA+`","start":"2s"}`)},
		{op2("item.trim", `{"item_id":"`+itemA+`","edge":"end","to":"6s"}`)},
		{op2("track.add", `{"kind":"visual","name":"Overlay"}`)},
		{op2("text.set", `{"item_id":"`+itemB+`","text":"Bye"}`)},
		{op2("item.split", `{"item_id":"`+itemA+`","at":"4s"}`)},
		{op2("item.delete", `{"item_ids":["`+itemB+`"]}`)},
		{op2("project.rename", `{"name":"Final cut"}`)},
	}
}

func scenarioStart() *timeline.Document {
	return base().
		clip(itemA, vidTr, 0, 240, 0).
		text(itemB, capTr, 200, 60).
		caption(itemC, 10, 4).d
}

// TestRevertGolden walks a fixed scenario: from the last revision back to every earlier one, the
// document equals the stored one, and the text of the diff is the golden file.
func TestRevertGolden(t *testing.T) {
	steps := history(t, scenarioStart(), scenarioBatches())
	last := len(steps) - 1

	var text strings.Builder

	for k := last; k >= 0; k-- {
		checkRevertTo(t, steps, last, k)

		text.WriteString("revert r" + itoa(last) + " -> r" + itoa(k) + "\n")

		for _, e := range DiffEntries(steps[last].doc, steps[k].doc) {
			text.WriteString("  " + e.Line + "\n")
		}
	}

	path := filepath.Join("testdata", "revert", "scenario.txt")

	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(text.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the golden file (run with -update to write it): %v", err)
	}

	if got := text.String(); got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatalf("the diff text differs from %s:\n%s", path, got)
	}
}

func TestRevertToTheCurrentRevisionIsANoOp(t *testing.T) {
	steps := history(t, scenarioStart(), scenarioBatches())
	last := len(steps) - 1

	res := Revert(steps[last].doc, steps[last].doc)
	if !res.NoOp() || len(res.Diff) != 0 || len(res.Inverse) != 0 {
		t.Fatalf("revert to the current document is not a no-op: %+v", res)
	}
}

// TestRevertAfterUndoSeesTheSameDocument: the diff is by document, so a revision whose document
// was reached again by an undo reverts to nothing.
func TestRevertAfterUndoSeesTheSameDocument(t *testing.T) {
	steps := history(t, scenarioStart(), scenarioBatches()[:2])
	undone, skipped := ApplyPrimitives(steps[2].doc, reversePrimitives(steps[2].inverse), nil)

	if len(skipped) != 0 {
		t.Fatalf("undo skipped %+v", skipped)
	}

	if res := Revert(undone.Doc, steps[1].doc); !res.NoOp() {
		t.Fatalf("the document after the undo equals r1, but the revert changes %+v", res.Diff)
	}
}

func TestRebuildAtRefusesALogThatDoesNotFit(t *testing.T) {
	steps := history(t, scenarioStart(), scenarioBatches())
	last := len(steps) - 1

	// The inverse of r2 applied to the document of r5: r5 removed an object r2 patches.
	if _, err := RebuildAt(
		steps[last].doc,
		[][]Primitive{{patch("items", "itm_zzzzzzzzzz", map[string]any{"start": jn("1")})}},
	); err == nil {
		t.Fatal("a primitive of an object that does not exist was accepted")
	}
}

// TestRevertProperty is the property of RF-120 over random histories: for every earlier revision,
// reverting gives a document equal to it, valid, and undoing the revert gives back the document
// the revert started from. The export of equal documents is equal, so the export at R equals the
// export after a revert to R.
func TestRevertProperty(t *testing.T) {
	const seed = 20261119

	rng := rand.New(rand.NewSource(seed))
	r := fullRegistry()
	ids := &seqIDs{n: 7}

	steps := []revStep{{doc: base().text("itm_x000000001", vidTr, 0, 120).clip("itm_x000000002", vidTr, 200, 150, 30).
		caption("itm_x000000003", 10, 4).audio("itm_x000000004", 0, 300, 0).d}}

	for len(steps) < 60 {
		cur := steps[len(steps)-1].doc
		res, err := r.ApplyBatch(cur, []Operation{realOp(rng, cur)}, Env{NewIDs: ids.next})

		if err != nil || res.NoOp() {
			continue
		}

		steps = append(steps, revStep{doc: res.Doc, inverse: res.Inverse})
	}

	last := len(steps) - 1

	for k := range last {
		checkRevertTo(t, steps, last, k)
	}

	// From the middle too: every pair, so a revert never depends on the head.
	for range 40 {
		n := 1 + rng.Intn(last)
		checkRevertTo(t, steps, n, rng.Intn(n))
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
