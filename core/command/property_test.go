package command

import (
	"bytes"
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// randomOp returns a random operation of the test catalogue that makes sense for doc.
func randomOp(rng *rand.Rand, doc *timeline.Document) Operation {
	items := make([]string, 0, len(doc.Items))
	for id := range doc.Items {
		items = append(items, id)
	}
	slices.Sort(items)
	visual := []string{}
	var lastOrder string
	for id, tr := range doc.Tracks {
		if tr.Kind == timeline.TrackVisual {
			visual = append(visual, id)
		}
		lastOrder = max(lastOrder, tr.Order)
	}
	slices.Sort(visual)
	pickItem := func() string {
		if len(items) == 0 || rng.Intn(10) == 0 {
			return "itm_zzzzzzzzzz"
		}
		return items[rng.Intn(len(items))]
	}
	switch rng.Intn(8) {
	case 0:
		return opJSON("test.name", fmt.Sprintf(`{"name":"Project %d"}`, rng.Intn(1000)))
	case 1:
		return opJSON(
			"test.track_add",
			fmt.Sprintf(
				`{"kind":"visual","order":%q,"name":"T%d"}`,
				timeline.OrderBetween(lastOrder, ""),
				rng.Intn(100),
			),
		)
	case 2, 3:
		if len(visual) == 0 {
			return opJSON("test.noop", `{}`)
		}
		return opJSON("test.item_add", fmt.Sprintf(`{"track_id":%q,"start":"%df","duration":%d,"text":"t%d"}`,
			visual[rng.Intn(len(visual))], rng.Intn(400), 1+rng.Intn(60), rng.Intn(100)))
	case 4, 5:
		return opJSON("test.item_move", fmt.Sprintf(`{"item_id":%q,"start":"%df"}`, pickItem(), rng.Intn(400)))
	case 6:
		return opJSON("test.item_remove", fmt.Sprintf(`{"item_ids":[%q]}`, pickItem()))
	default:
		return opJSON(
			"test.settings",
			fmt.Sprintf(`{"width":%d,"height":%d}`, 16+2*rng.Intn(2000), 16+2*rng.Intn(2000)),
		)
	}
}

// SDD-008 §3 and ROADMAP P1-ENG-001: over 1,000 random batches, applying and then
// the inverse restores the canonical bytes; every result is valid; Apply does not
// mutate its input; a batch equals its operations applied one by one.
func TestPropertyApplyThenInverseIsTheIdentity(t *testing.T) {
	const seed = 20261114
	rng := rand.New(rand.NewSource(seed))
	r := testRegistry(t)
	doc := baseDoc(t)
	succeeded, failed := 0, 0
	for i := range 1000 {
		ids := &seqIDs{n: i * 7}
		ops := make([]Operation, 1+rng.Intn(4))
		for j := range ops {
			ops[j] = randomOp(rng, doc)
		}
		before := timeline.Canonical(doc)
		res, err := r.ApplyBatch(doc, ops, Env{NewIDs: ids.next})
		if !bytes.Equal(timeline.Canonical(doc), before) {
			t.Fatalf("seed %d, batch %d: ApplyBatch mutated its input", seed, i)
		}
		if err != nil {
			failed++
			e := AsError(err)
			if cerr := CheckError(e); cerr != nil || e.Code == CodeInternal && e.Details["violations"] == nil {
				t.Fatalf("seed %d, batch %d: bad error %+v: %v", seed, i, e, cerr)
			}
			continue
		}
		succeeded++
		if vs := timeline.Validate(res.Doc); len(vs) != 0 {
			t.Fatalf("seed %d, batch %d: invalid result: %v", seed, i, vs)
		}
		back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
		if len(skipped) != 0 || !bytes.Equal(timeline.Canonical(back.Doc), before) {
			t.Fatalf(
				"seed %d, batch %d: the inverse did not restore the document (skipped %v)\nops %v",
				seed,
				i,
				skipped,
				ops,
			)
		}
		cur := doc
		oneByOne := &seqIDs{n: i * 7}
		for _, op := range ops {
			step, err := r.Apply(cur, op, Env{NewIDs: oneByOne.next})
			if err != nil {
				t.Fatalf("seed %d, batch %d: one by one failed: %v", seed, i, err)
			}
			cur = step.Doc
		}
		if !bytes.Equal(timeline.Canonical(cur), timeline.Canonical(res.Doc)) {
			t.Fatalf("seed %d, batch %d: the batch differs from its operations one by one", seed, i)
		}
		ids2 := &seqIDs{n: i * 7}
		again, err := r.ApplyBatch(doc, ops, Env{NewIDs: ids2.next})
		if err != nil || !bytes.Equal(timeline.Canonical(again.Doc), timeline.Canonical(res.Doc)) {
			t.Fatalf("seed %d, batch %d: ApplyBatch is not deterministic", seed, i)
		}
		if len(res.Diff) == 0 != bytes.Equal(timeline.Canonical(res.Doc), before) {
			t.Fatalf("seed %d, batch %d: diff and document disagree", seed, i)
		}
		if rng.Intn(4) != 0 { // mostly keep growing the same document
			doc = res.Doc
		}
		if i%40 == 39 { // and start over now and then, to keep the documents small
			doc = baseDoc(t)
		}
	}
	if succeeded < 400 || failed < 50 {
		t.Errorf("the generator is not exercising both paths: %d succeeded, %d failed", succeeded, failed)
	}
}
