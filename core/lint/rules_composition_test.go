package lint

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func compositionOf(tl string, dur int) timeline.Item {
	it := clip("itm_a1a1a1a1a1", 0, dur)
	it.Type, it.Asset, it.Timeline = timeline.ItemComposition, "", tl

	return it
}

func TestCompositionLint(t *testing.T) {
	t.Parallel()

	child := newDoc()
	child.ID = "prj_c1c1c1c1c1"
	put(child, clip("itm_k1k1k1k1k1", 0, 60))

	empty := newDoc()
	empty.ID = "prj_e1e1e1e1e1"

	env := Env{Nested: func(id string) (*timeline.Document, bool) {
		switch id {
		case child.ID:
			return child, true
		case empty.ID:
			return empty, true
		}

		return nil, false
	}}

	host := func(c timeline.Item) *timeline.Document {
		d := newDoc()
		put(d, c)

		return d
	}

	if got := has(Lint(host(compositionOf(child.ID, 60)), env, Full), CodeSourceRange); len(got) != 0 {
		t.Errorf("a composition that fits its timeline: %+v", got)
	}

	missing := has(Lint(host(compositionOf("prj_m1m1m1m1m1", 60)), env, Fast), CodeMediaMissing)
	if len(missing) != 1 || missing[0].Item != "itm_a1a1a1a1a1" {
		t.Errorf("a missing timeline: %+v", missing)
	}

	if got := has(Lint(host(compositionOf(child.ID, 90)), env, Fast), CodeSourceRange); len(got) != 0 {
		t.Errorf("the range check runs in a Fast lint: %+v", got)
	}

	if got := has(Lint(host(compositionOf(child.ID, 90)), env, Full), CodeSourceRange); len(got) != 1 {
		t.Errorf("a composition longer than its timeline: %+v", got)
	}

	// At 2x a composition of 30 frames reads 60 frames of the timeline (epic 53): it fits; of 31 it does not.
	fast := func(dur int, speed timeline.Rational) timeline.Item {
		it := compositionOf(child.ID, dur)
		it.Speed = speed

		return it
	}

	if got := has(Lint(host(fast(30, timeline.Rational{Num: 2, Den: 1})), env, Full), CodeSourceRange); len(got) != 0 {
		t.Errorf("a composition at 2x that fits its timeline: %+v", got)
	}

	if got := has(Lint(host(fast(31, timeline.Rational{Num: 2, Den: 1})), env, Full), CodeSourceRange); len(got) != 1 {
		t.Errorf("a composition at 2x that reads past its timeline: %+v", got)
	}

	if got := has(Lint(host(fast(90, timeline.Rational{Num: 1, Den: 2})), env, Full), CodeSourceRange); len(got) != 0 {
		t.Errorf("a composition at 0.5x that fits its timeline: %+v", got)
	}

	if got := has(Lint(host(compositionOf(empty.ID, 30)), env, Full), CodeSourceRange); len(got) != 1 {
		t.Errorf("an empty timeline: %+v", got)
	}

	if got := has(Lint(host(compositionOf("prj_m1m1m1m1m1", 60)), Env{}, Full), CodeMediaMissing); len(got) != 0 {
		t.Errorf("without Env.Nested the rule must be silent: %+v", got)
	}
}
