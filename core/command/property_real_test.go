package command

import (
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// realOp returns a random operation of the first catalogue that mostly makes
// sense for doc; some are wrong on purpose so that the error paths run too.
func realOp(rng *rand.Rand, doc *timeline.Document) Operation {
	pick := func(m []string) string {
		if len(m) == 0 || rng.Intn(15) == 0 {
			return "itm_zzzzzzzzzz"
		}
		return m[rng.Intn(len(m))]
	}
	items := make([]string, 0, len(doc.Items))
	for id := range doc.Items {
		items = append(items, id)
	}
	slices.Sort(items)
	var visual, all []string
	for id := range doc.Tracks {
		all = append(all, id)
		if doc.Tracks[id].Kind == timeline.TrackVisual {
			visual = append(visual, id)
		}
	}
	slices.Sort(all)
	slices.Sort(visual)
	track := func() string {
		if len(visual) == 0 {
			return "trk_zzzzzzzzzz"
		}
		return visual[rng.Intn(len(visual))]
	}
	frames := func(n int) string { return fmt.Sprintf("%df", rng.Intn(n)) }
	word := func() string { return fmt.Sprintf("w%d", rng.Intn(100)) }

	switch rng.Intn(22) {
	case 0:
		return op2("project.rename", fmt.Sprintf(`{"name": "Project %d"}`, rng.Intn(100)))
	case 1:
		return op2("settings.set", fmt.Sprintf(`{"width": %d, "height": %d}`, 16+2*rng.Intn(1000), 16+2*rng.Intn(1000)))
	case 2:
		return op2(
			"track.add",
			fmt.Sprintf(
				`{"kind": %q, "position": %q}`,
				[]string{"visual", "audio"}[rng.Intn(2)],
				[]string{"top", "bottom"}[rng.Intn(2)],
			),
		)
	case 3:
		return op2(
			"track.update",
			fmt.Sprintf(`{"track_id": %q, "name": "T%d", "muted": %t}`, pick(all), rng.Intn(9), rng.Intn(2) == 0),
		)
	case 4:
		return op2(
			"track.move",
			fmt.Sprintf(`{"track_id": %q, %q: %q}`, pick(all), []string{"above", "below"}[rng.Intn(2)], pick(all)),
		)
	case 5:
		return op2("track.remove", fmt.Sprintf(`{"track_id": %q, "with_items": %t}`, pick(all), rng.Intn(2) == 0))
	case 6:
		return op2(
			"track.set_caption_style",
			fmt.Sprintf(
				`{"track_id": %q, "style": {"size": %d, "uppercase": %t}}`,
				track(),
				8+rng.Intn(100),
				rng.Intn(2) == 0,
			),
		)
	case 7, 8:
		return op2("clip.add", fmt.Sprintf(`{"track_id": %q, "asset_id": %q, "start": %q, "duration": "%df"}`,
			track(), []string{vidAsset, imgAsset}[rng.Intn(2)], frames(2000), 1+rng.Intn(300)))
	case 9, 10:
		return op2(
			"text.add",
			fmt.Sprintf(
				`{"track_id": %q, "start": %q, "duration": "%df", "text": %q}`,
				track(),
				frames(2000),
				1+rng.Intn(100),
				word(),
			),
		)
	case 11:
		return op2(
			"text.set",
			fmt.Sprintf(`{"item_id": %q, "text": %q, "style": {"size": %d}}`, pick(items), word(), 8+rng.Intn(100)),
		)
	case 12, 13:
		return op2(
			"caption.add",
			fmt.Sprintf(
				`{"track_id": %q, "start": %q, "text": "a bb ccc dddd", "duration": "%df"}`,
				track(),
				frames(2000),
				4+rng.Intn(60),
			),
		)
	case 14, 15:
		extra := ""
		if rng.Intn(3) == 0 {
			extra = `, "slide": true`
		}

		return op2("item.move", fmt.Sprintf(`{"item_id": %q, "start": %q%s}`, pick(items), frames(2500), extra))
	case 16, 17:
		extra := []string{"", `, "roll": true`, `, "ripple": "track"`}[rng.Intn(3)]

		return op2(
			"item.trim",
			fmt.Sprintf(
				`{"item_id": %q, "edge": %q, "to": %q%s}`,
				pick(items),
				[]string{"start", "end"}[rng.Intn(2)],
				frames(2500),
				extra,
			),
		)
	case 18, 19:
		return op2("item.split", fmt.Sprintf(`{"item_id": %q, "at": %q}`, pick(items), frames(2500)))
	case 20:
		return op2(
			"item.delete",
			fmt.Sprintf(`{"item_ids": [%q], "ripple": %q}`, pick(items), []string{"none", "track", "all"}[rng.Intn(3)]),
		)
	default:
		return op2(
			"asset.add",
			fmt.Sprintf(
				`{"asset": {"id": %q, "hash": "sha256:%064x", "name": "i", "kind": "image", "width": 10, "height": 10}}`,
				fmt.Sprintf("ast_k%09d", rng.Intn(1000)),
				rng.Intn(1000),
			),
		)
	}
}

func op2(name, args string) Operation { return Operation{Op: name, Args: []byte(args)} }

// The property test over the real catalogue (ROADMAP P1-ENG-001, SDD-008 §3): 1,000
// random batches; every success is valid, leaves its input alone, and the inverse
// restores the canonical bytes.
func TestPropertyRealCommands(t *testing.T) {
	const seed = 20261115
	rng := rand.New(rand.NewSource(seed))
	r := fullRegistry()
	r.Debug = true
	defer func() { r.Debug = false }()
	fresh := func() *timeline.Document {
		b := base().text("itm_x000000001", vidTr, 0, 120).clip("itm_x000000002", vidTr, 200, 150, 30).
			caption("itm_x000000003", 10, 4).audio("itm_x000000004", 0, 300, 0)
		return b.d
	}
	doc := fresh()
	counts := map[string]int{}
	succeeded := 0
	for i := range 1000 {
		ops := make([]Operation, 1+rng.Intn(3))
		for j := range ops {
			ops[j] = realOp(rng, doc)
		}
		before := timeline.Canonical(doc)
		ids := &seqIDs{n: i * 13}
		res, err := r.ApplyBatch(doc, ops, Env{NewIDs: ids.next})
		if !bytes.Equal(timeline.Canonical(doc), before) {
			t.Fatalf("seed %d, batch %d: the input document changed", seed, i)
		}
		if err != nil {
			e := AsError(err)
			if cerr := CheckError(e); cerr != nil {
				t.Fatalf("seed %d, batch %d: %v: %+v", seed, i, cerr, e)
			}
			if e.Code == CodeInternal {
				t.Fatalf("seed %d, batch %d: an internal error for %v: %+v", seed, i, ops, e)
			}
			counts[e.Code]++
			continue
		}
		succeeded++
		for _, o := range ops {
			counts["ok "+o.Op]++
		}
		if vs := timeline.Validate(res.Doc); len(vs) != 0 {
			t.Fatalf("seed %d, batch %d: invalid result %v\nops %v", seed, i, vs, ops)
		}
		fullBefore, fullAfter := timeline.Raw(doc), timeline.Raw(res.Doc)
		if !reflect.DeepEqual(res.Diff, diffOf(timeline.Full(doc), timeline.Full(res.Doc))) ||
			!reflect.DeepEqual(res.Inverse, inverseOf(fullBefore, fullAfter)) {
			t.Fatalf("seed %d, batch %d: the lazy diff differs from the full diff; ops %v", seed, i, ops)
		}
		back, skipped := ApplyPrimitives(res.Doc, reversed(res.Inverse), nil)
		if len(skipped) != 0 || !bytes.Equal(timeline.Canonical(back.Doc), before) {
			t.Fatalf(
				"seed %d, batch %d: the inverse did not restore the document (skipped %+v)\nops %v",
				seed,
				i,
				skipped,
				ops,
			)
		}
		doc = res.Doc
		if len(doc.Items) > 60 || i%50 == 49 {
			doc = fresh()
		}
	}
	if succeeded < 300 {
		t.Errorf("only %d of 1000 batches succeeded; counts %v", succeeded, counts)
	}
	for _, name := range []string{
		"item.move", "item.trim", "item.split", "item.delete", "clip.add", "text.add", "caption.add",
		"track.add", "track.move", "track.remove", "text.set",
	} {
		if counts["ok "+name] == 0 {
			t.Errorf("%s never succeeded in the random batches", name)
		}
	}
}
