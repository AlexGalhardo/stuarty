package service

import (
	"context"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

type fakeProductionRepo struct {
	ProductionRepo
	ids []string
}

func (f fakeProductionRepo) ProjectOfTimeline(_ context.Context, id string) (string, error) {
	if slices.Contains(f.ids, id) {
		return "prj_production", nil
	}

	return "", ErrNotFound
}

func (f fakeProductionRepo) ActiveTimelines(_ context.Context, _ string) ([]TimelineRef, error) {
	out := make([]TimelineRef, len(f.ids))
	for i, id := range f.ids {
		out[i] = TimelineRef{TimelineID: id}
	}

	return out, nil
}

type fakeProjectRepo struct {
	ProjectRepo
	docs map[string]ProjectRow
}

func (f fakeProjectRepo) Get(_ context.Context, id string) (ProjectRow, error) {
	if r, ok := f.docs[id]; ok {
		return r, nil
	}

	return ProjectRow{}, ErrNotFound
}

type fakeRepos struct {
	Repos
	prod fakeProductionRepo
	proj fakeProjectRepo
}

func (f fakeRepos) Productions() ProductionRepo { return f.prod }
func (f fakeRepos) Projects() ProjectRepo       { return f.proj }

const (
	tlA = "prj_a000000001"
	tlB = "prj_b000000001"
	tlC = "prj_c000000001"
)

func rowOf(t *testing.T, id string, rev int64, holds ...string) ProjectRow {
	t.Helper()

	d, err := timeline.Decode([]byte(`{"schema_version":1,"id":"` + id + `","name":"T","settings":{},"assets":{},` +
		`"tracks":{"trk_a000000001":{"id":"trk_a000000001","kind":"visual","order":"a0"}},"items":{}}`))
	if err != nil {
		t.Fatal(err)
	}

	for i, h := range holds {
		it := timeline.NewItem(
			"itm_"+string(rune('a'+i))+"000000001",
			timeline.ItemComposition,
			"trk_a000000001",
			i*10,
			10,
		)
		it.Timeline = h
		d.Items[it.ID] = it
	}

	return ProjectRow{ID: id, Revision: rev, State: ProjectActive, Document: timeline.Canonical(d)}
}

func graphOf(t *testing.T, rows ...ProjectRow) *productionGraph {
	t.Helper()

	r := fakeRepos{proj: fakeProjectRepo{docs: map[string]ProjectRow{}}}

	for i := range rows {
		r.prod.ids = append(r.prod.ids, rows[i].ID)
		r.proj.docs[rows[i].ID] = rows[i]
	}

	return newProductionGraph(context.Background(), r, rows[0].ID)
}

// A parent's cache key changes when the revision of a timeline it uses changes, however deep, and is the plain
// revision when it uses none.
func TestNestedInvalidationCacheRevision(t *testing.T) {
	t.Parallel()

	if got := cacheRevision(7, nil); got != 7 {
		t.Errorf("a timeline with no composition is keyed by its revision, got %d", got)
	}

	a := cacheRevision(7, map[string]int64{tlB: 3, tlC: 1})

	for name, revs := range map[string]map[string]int64{
		"child":      {tlB: 4, tlC: 1},
		"grandchild": {tlB: 3, tlC: 2},
		"fewer":      {tlB: 3},
	} {
		if cacheRevision(7, revs) == a {
			t.Errorf("a change of the %s did not change the key", name)
		}
	}

	if cacheRevision(8, map[string]int64{tlB: 3, tlC: 1}) == a {
		t.Error("a change of the parent did not change the key")
	}

	if cacheRevision(7, map[string]int64{tlC: 1, tlB: 3}) != a {
		t.Error("the key depends on the order of the map")
	}

	if a < 1<<62 {
		t.Errorf("the key %d can be taken for a real revision", a)
	}
}

func TestNestedInvalidationGraph(t *testing.T) {
	t.Parallel()

	// A holds B, B holds C.
	g := graphOf(t, rowOf(t, tlA, 5, tlB), rowOf(t, tlB, 2, tlC), rowOf(t, tlC, 9))

	if got := g.usersDeep(tlC); len(got) != 2 || got[0] != tlA || got[1] != tlB {
		t.Errorf("users of C = %v, want A and B", got)
	}

	docs, revs := g.Closure(mustDoc(t, g, tlA))
	if len(docs) != 2 || revs[tlB] != 2 || revs[tlC] != 9 {
		t.Errorf("closure of A = %v %v, want B at 2 and C at 9", docs, revs)
	}

	// C holding A would close a loop, and so would B holding A.
	if err := checkNesting(g, mustDoc(t, g, tlC)); err != nil {
		t.Errorf("a sound document is refused: %v", err)
	}

	loop := mustDoc(t, g, tlC)
	it := timeline.NewItem("itm_z000000001", timeline.ItemComposition, "trk_a000000001", 0, 10)
	it.Timeline = tlA
	loop.Items[it.ID] = it

	if err := checkNesting(g, loop); err == nil || err.Details["reason"] != "composition_cycle" {
		t.Errorf("a loop through two timelines was not refused: %v", err)
	}
}

func mustDoc(t *testing.T, g *productionGraph, id string) *timeline.Document {
	t.Helper()

	d, ok := g.Doc(id)
	if !ok {
		t.Fatalf("no timeline %s", id)
	}

	return d
}
