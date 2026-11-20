package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// revStep is one revision of a test history: the document after it and the inverse it stored.
type revStep struct {
	doc     *Document
	inverse []command.Primitive
}

// historyEnv makes the ids of the layers a history adds, so that every revision is reproducible.
func historyEnv() Env {
	n := 100

	env := testEnv()
	env.NewIDs = func(string) string {
		n++

		return fid(n)
	}

	return env
}

// randomOp is one operation over the layers the document has now: some are refused by the core, and the
// history simply tries another.
func randomOp(rng *rand.Rand, d *Document) command.Operation {
	var ids []string

	Walk(d, func(l, _ *Layer, _ int) { ids = append(ids, l.ID) })

	pick := func() string { return ids[rng.Intn(len(ids))] }
	args := func(format string, a ...any) json.RawMessage { return json.RawMessage(fmt.Sprintf(format, a...)) }

	if len(ids) == 0 {
		return command.Operation{Op: "layer.add_fill", Args: args(`{"color": "#102030"}`)}
	}

	switch rng.Intn(11) {
	case 0:
		return command.Operation{Op: "layer.add_shape", Args: args(`{"shape": "rect", "w": %d, "h": %d}`,
			10+rng.Intn(200), 10+rng.Intn(200))}
	case 1:
		return command.Operation{Op: "layer.add_text", Args: args(`{"text": "Text %d"}`, rng.Intn(1000))}
	case 2:
		return command.Operation{Op: "layer.add_image", Args: args(`{"asset": %q}`, hashFace)}
	case 3:
		return command.Operation{Op: "layer.set", Args: args(`{"layer_id": %q, "opacity": %d, "name": "N%d"}`,
			pick(), rng.Intn(101), rng.Intn(100))}
	case 4:
		return command.Operation{Op: "layer.transform", Args: args(`{"layer_id": %q, "x": %d, "y": %d}`,
			pick(), rng.Intn(500), rng.Intn(500))}
	case 5:
		return command.Operation{Op: "layer.remove", Args: args(`{"layer_id": %q}`, pick())}
	case 6:
		return command.Operation{Op: "layer.move", Args: args(`{"layer_id": %q, "index": %d}`, pick(), rng.Intn(4))}
	case 7:
		return command.Operation{Op: "layer.duplicate", Args: args(`{"layer_id": %q, "dx": 20, "dy": 20}`, pick())}
	case 8:
		return command.Operation{Op: "canvas.resize", Args: args(`{"width": %d, "height": %d, "mode": "keep"}`,
			600+rng.Intn(900), 400+rng.Intn(600))}
	case 9:
		return command.Operation{Op: "layer.group", Args: args(`{"layer_ids": [%q]}`, pick())}
	default:
		return command.Operation{Op: "layer.set_shape", Args: args(`{"layer_id": %q, "fill": "#%02x%02x%02x"}`,
			pick(), rng.Intn(256), rng.Intn(256), rng.Intn(256))}
	}
}

// randomHistory is a history of revisions: edits, and now and then an undo of the last edit, as the log of an
// image has them. Index 0 is the starting document with no inverse.
func randomHistory(t *testing.T, seed int64, length int) []revStep {
	t.Helper()

	rng := rand.New(rand.NewSource(seed))
	env := historyEnv()
	start := baseDoc()
	ReorderAll(start)

	steps := []revStep{{doc: start}}
	attempts := 0

	for len(steps) < length {
		attempts++
		if attempts > length*60 {
			t.Fatalf("the history stopped at %d revisions of %d", len(steps), length)
		}

		cur := steps[len(steps)-1]

		if rng.Intn(6) == 0 && len(cur.inverse) > 0 {
			res, skipped := ApplyPrimitives(cur.doc, cur.inverse, nil)
			if len(skipped) == 0 && len(res.Diff) > 0 {
				steps = append(steps, revStep{doc: res.Doc, inverse: res.Inverse})
			}

			continue
		}

		res, err := ApplyBatch(cur.doc, []command.Operation{randomOp(rng, cur.doc)}, env)
		if err != nil || res.NoOp() {
			continue
		}

		steps = append(steps, revStep{doc: res.Doc, inverse: res.Inverse})
	}

	return steps
}

// checkRevertTo asserts the whole contract of a revert from revision n to revision k.
func checkRevertTo(t *testing.T, steps []revStep, n, k int) {
	t.Helper()

	current, target := steps[n].doc, steps[k].doc

	var inverses [][]command.Primitive
	for i := n; i > k; i-- {
		inverses = append(inverses, steps[i].inverse)
	}

	rebuilt, err := Rebuild(current, inverses)
	if err != nil {
		t.Fatalf("rebuild r%d from r%d: %v", k, n, err)
	}

	if !bytes.Equal(Canonical(rebuilt), Canonical(target)) {
		t.Fatalf("the rebuilt r%d differs from the stored one", k)
	}

	before := Canonical(current)
	res := Revert(current, rebuilt)

	if !bytes.Equal(Canonical(current), before) {
		t.Fatalf("revert to r%d changed its input", k)
	}

	if !bytes.Equal(Canonical(res.Doc), Canonical(target)) {
		t.Fatalf("revert to r%d: the document differs from r%d", k, k)
	}

	if vs := Validate(res.Doc); len(vs) != 0 {
		t.Fatalf("revert to r%d is invalid: %v", k, vs)
	}

	if res.NoOp() != bytes.Equal(before, Canonical(target)) {
		t.Fatalf("revert to r%d: NoOp is %v with %d changes", k, res.NoOp(), len(res.Diff))
	}

	back, skipped := ApplyPrimitives(res.Doc, res.Inverse, nil)
	if len(skipped) != 0 {
		t.Fatalf("undo of the revert to r%d skipped %+v", k, skipped)
	}

	if !bytes.Equal(Canonical(back.Doc), before) {
		t.Fatalf("revert to r%d then undo does not give the document before the revert", k)
	}
}

// TestImageRevertProperty is the property of RF-120 over random histories of an image: for every earlier
// revision, the document rebuilt from the inverses and the one a revert makes have the canonical bytes of that
// revision, so an export of one is the export of the other, and undoing the revert gives back the document the
// revert started from.
func TestImageRevertProperty(t *testing.T) {
	for _, seed := range []int64{20261120, 7, 1999} {
		steps := randomHistory(t, seed, 45)
		last := len(steps) - 1

		for k := range last {
			checkRevertTo(t, steps, last, k)
		}

		rng := rand.New(rand.NewSource(seed))

		for range 40 {
			n := 1 + rng.Intn(last)
			checkRevertTo(t, steps, n, rng.Intn(n))
		}
	}
}

// TestImageRevertForEveryKindOfChange walks one scenario that has each kind of change: a layer added, moved,
// removed, its look changed, the canvas resized and a file added.
func TestImageRevertForEveryKindOfChange(t *testing.T) {
	batches := [][]command.Operation{
		{op("layer.add_shape", `{"shape": "ellipse", "w": 90, "h": 90}`)},
		{op("layer.transform", `{"layer_id": "`+lid('d')+`", "x": 300, "y": 30}`)},
		{op("layer.set", `{"layer_id": "`+lid('c')+`", "opacity": 40}`)},
		{op("layer.remove", `{"layer_id": "`+lid('d')+`"}`)},
		{op("canvas.resize", `{"width": 640, "height": 360, "mode": "scale"}`)},
		{op("layer.add_image", `{"asset": "`+hashFace+`"}`)},
		{op("layer.remove", `{"layer_id": "`+lid('b')+`"}`)},
	}

	env := historyEnv()
	steps := []revStep{{doc: baseDoc()}}
	ReorderAll(steps[0].doc)

	for i, ops := range batches {
		res, err := ApplyBatch(steps[i].doc, ops, env)
		if err != nil {
			t.Fatalf("batch %d: %v", i, err)
		}

		steps = append(steps, revStep{doc: res.Doc, inverse: res.Inverse})
	}

	last := len(steps) - 1

	for n := last; n > 0; n-- {
		for k := range n {
			checkRevertTo(t, steps, n, k)
		}
	}
}

func TestImageRevertToTheCurrentDocumentIsANoOp(t *testing.T) {
	steps := randomHistory(t, 11, 8)
	last := steps[len(steps)-1].doc

	res := Revert(last, Clone(last))
	if !res.NoOp() || len(res.Diff) != 0 || len(res.Inverse) != 0 {
		t.Fatalf("revert to the current document is not a no-op: %+v", res)
	}
}

func TestImageRevertRefusesALogThatDoesNotFit(t *testing.T) {
	steps := randomHistory(t, 11, 8)

	bad := [][]command.Primitive{{{
		Type: command.PrimitivePatch, Collection: CollLayers, ID: "lyr_zzzzzzzzzz", Set: map[string]any{"x": 1},
	}}}

	if _, err := Rebuild(steps[len(steps)-1].doc, bad); err == nil {
		t.Fatal("a primitive of a layer that does not exist was accepted")
	}
}

// TestImageRevertAfterUndoSeesTheSameDocument: the diff is by document, so a revision whose document was
// reached again by an undo reverts to nothing.
func TestImageRevertAfterUndoSeesTheSameDocument(t *testing.T) {
	steps := randomHistory(t, 3, 6)
	cur := steps[len(steps)-1]

	undone, skipped := ApplyPrimitives(cur.doc, cur.inverse, nil)
	if len(skipped) != 0 {
		t.Fatalf("undo skipped %+v", skipped)
	}

	if res := Revert(undone.Doc, steps[len(steps)-2].doc); !res.NoOp() {
		t.Fatalf("the document after the undo equals the previous revision, but the revert changes %+v", res.Diff)
	}
}

// TestImageDiffEntries names each kind of change in the words of the diff text, most significant first.
func TestImageDiffEntries(t *testing.T) {
	a := baseDoc()
	ReorderAll(a)

	res, err := ApplyBatch(a, []command.Operation{
		op("layer.remove", `{"layer_id": "`+lid('d')+`"}`),
		op("layer.set", `{"layer_id": "`+lid('c')+`", "opacity": 60}`),
		op("layer.transform", `{"layer_id": "`+lid('e')+`", "x": 70}`),
		op("layer.add_shape", `{"shape": "rect", "w": 20, "h": 20, "name": "Dot"}`),
		op("canvas.resize", `{"width": 640, "height": 360, "mode": "keep"}`),
	}, historyEnv())
	if err != nil {
		t.Fatal(err)
	}

	entries := DiffEntries(a, res.Doc)

	var got []string

	for _, e := range entries {
		got = append(got, e.Verb+"/"+e.Noun+" "+e.Line)
	}

	text := strings.Join(got, "\n")

	for _, want := range []string{
		`added/layer added shape "Dot"`,
		`removed/layer removed shape "Bar" ` + lid('d'),
		`changed/document changed document: height 720 -> 360, preset "video_thumbnail" -> none, width 1280 -> 640`,
		`changed/layer changed "Title" ` + lid('c') + `: opacity 100 -> 60`,
		`moved/layer moved "Badge" ` + lid('e') + `: x 50 -> 70`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the diff lacks %q:\n%s", want, text)
		}
	}

	weights := make([]int, len(entries))
	for i, e := range entries {
		weights[i] = e.Weight
	}

	if !slices.IsSortedFunc(weights, func(x, y int) int { return y - x }) {
		t.Errorf("the entries are not most significant first: %v", weights)
	}
}

func TestImageDiffNamesAFileThatEntersOrLeaves(t *testing.T) {
	a := baseDoc()
	ReorderAll(a)

	res, err := ApplyBatch(a, []command.Operation{op("layer.remove", `{"layer_id": "`+lid('b')+`"}`)}, historyEnv())
	if err != nil {
		t.Fatal(err)
	}

	var line string

	for _, e := range DiffEntries(a, res.Doc) {
		if e.Noun == NounAsset {
			line = e.Line
		}
	}

	if want := `removed asset ` + hashFace + ` "face.png" image 1600x900`; line != want {
		t.Fatalf("asset line = %q, want %q", line, want)
	}

	if entries := DiffEntries(res.Doc, res.Doc); len(entries) != 0 {
		t.Fatalf("equal documents have %d changes", len(entries))
	}
}

// TestImageDiffIsOfTheDocumentsNotOfTheLog: an edit and its undo leave nothing to say.
func TestImageDiffIsOfTheDocumentsNotOfTheLog(t *testing.T) {
	steps := randomHistory(t, 5, 6)
	cur := steps[len(steps)-1]

	undone, skipped := ApplyPrimitives(cur.doc, cur.inverse, nil)
	if len(skipped) != 0 {
		t.Fatalf("undo skipped %+v", skipped)
	}

	if entries := DiffEntries(steps[len(steps)-2].doc, undone.Doc); len(entries) != 0 {
		t.Fatalf("a document and the one its undo gave have %d changes", len(entries))
	}
}
