package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"maps"
	"slices"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/crashpoint"
)

// Nested compositions in the service (SDD-001 section 3.5.1, SDD-004 section 3.16): the timelines of one project
// are read from the store for the rules of the core, for rendering and for the invalidation of previews.

// productionGraph is timeline.Graph over the active timelines of the production that holds one timeline. It reads
// them when first asked, so an edit with no composition in it never pays for it. The documents are as stored: the
// edited timeline is the version before the edit.
type productionGraph struct {
	ctx   context.Context //nolint:containedctx // a graph lives for one operation
	repos Repos
	self  string
	docs  map[string]*timeline.Document
	revs  map[string]int64
	done  bool
}

var _ timeline.Graph = (*productionGraph)(nil)

func newProductionGraph(ctx context.Context, repos Repos, self string) *productionGraph {
	return &productionGraph{
		ctx: ctx, repos: repos, self: self, docs: map[string]*timeline.Document{}, revs: map[string]int64{},
	}
}

func (g *productionGraph) load() {
	if g.done {
		return
	}

	g.done = true

	owner, err := g.repos.Productions().ProjectOfTimeline(g.ctx, g.self)
	if err != nil {
		return
	}

	tls, err := g.repos.Productions().ActiveTimelines(g.ctx, owner)
	if err != nil {
		return
	}

	// The usual project has one timeline, which holds no other: nothing to read.
	if len(tls) == 1 && tls[0].TimelineID == g.self {
		return
	}

	for i := range tls {
		row, gerr := g.repos.Projects().Get(g.ctx, tls[i].TimelineID)
		if gerr != nil || row.State == ProjectTrashed {
			continue
		}

		if doc, derr := decodeRow(&row); derr == nil {
			g.docs[tls[i].TimelineID], g.revs[tls[i].TimelineID] = doc, row.Revision
		}
	}
}

// Doc implements timeline.Graph.
func (g *productionGraph) Doc(id string) (*timeline.Document, bool) {
	g.load()
	d, ok := g.docs[id]

	return d, ok
}

// Users implements timeline.Graph.
func (g *productionGraph) Users(id string) []string {
	g.load()

	var out []string

	for _, other := range slices.Sorted(maps.Keys(g.docs)) {
		if other != id && slices.Contains(timeline.CompositionTimelines(g.docs[other]), id) {
			out = append(out, other)
		}
	}

	return out
}

// Closure is every timeline the compositions of doc use, directly or through others, with the revision of each.
// A timeline that is not in the production (missing or in the trash) is not in it.
func (g *productionGraph) Closure(doc *timeline.Document) (docs map[string]*timeline.Document, revs map[string]int64) {
	docs, revs = map[string]*timeline.Document{}, map[string]int64{}

	var walk func(d *timeline.Document, level int)

	walk = func(d *timeline.Document, level int) {
		if level > timeline.MaxCompositionDepth+1 {
			return
		}

		for _, id := range timeline.CompositionTimelines(d) {
			if _, seen := docs[id]; seen {
				continue
			}

			if n, ok := g.Doc(id); ok {
				docs[id], revs[id] = n, g.revs[id]
				walk(n, level+1)
			}
		}
	}

	walk(doc, 0)

	return docs, revs
}

// Users, transitively: the timelines that hold id directly or through others, sorted.
func (g *productionGraph) usersDeep(id string) []string {
	seen := map[string]bool{}

	var walk func(string)

	walk = func(x string) {
		for _, u := range g.Users(x) {
			if !seen[u] {
				seen[u] = true
				walk(u)
			}
		}
	}

	walk(id)

	return slices.Sorted(maps.Keys(seen))
}

// cacheRevision is the revision a cache is keyed by: the revision of the timeline when it holds no composition,
// else a number that changes when the revision of the timeline or of any timeline it uses changes, so a parent's
// frames are never served after an edit of a child (SDD-004 section 3.16). It is never below 1<<62, so it cannot
// be a real revision.
func cacheRevision(rev int64, revs map[string]int64) int64 {
	if len(revs) == 0 {
		return rev
	}

	h := sha256.New()
	_, _ = h.Write([]byte(strconv.FormatInt(rev, 10)))

	for _, id := range slices.Sorted(maps.Keys(revs)) {
		_, _ = h.Write([]byte("|" + id + ":" + strconv.FormatInt(revs[id], 10)))
	}

	sum := h.Sum(nil)

	return int64(binary.BigEndian.Uint64(sum[:8])>>2) | 1<<62
}

// checkNesting is the check that runs after any change of a timeline that holds compositions: the commands
// refuse a loop and a nesting too deep, and this catches the ways around them (undo, redo, a revert).
func checkNesting(g *productionGraph, doc *timeline.Document) *Error {
	if len(timeline.CompositionTimelines(doc)) == 0 {
		return nil
	}

	n, child := timeline.CheckDocument(g, doc)
	if n == nil {
		return nil
	}

	if n.Kind == timeline.NestingCycle {
		return newError(
			command.CodeInvalidState,
			"the change would make timeline %s hold itself through %s.",
			doc.ID,
			child,
		).
			withHint("remove the composition of the loop first").
			withDetail("reason", "composition_cycle").
			withDetail("path", n.Path)
	}

	return newError(command.CodeLimit, "the change would nest compositions %d deep; the limit is %d.",
		n.Depth, timeline.MaxCompositionDepth).
		withHint("flatten a level with composition.expand").
		withDetail("reason", "composition_depth").withDetail("depth", n.Depth).
		withDetail("limit", timeline.MaxCompositionDepth)
}

// checkRestoredNesting is checkNesting for a timeline that comes back from the trash: while it was there, the
// timelines it holds may have grown deeper, and a composition left naming it is in use again (SDD-001 §3.5.1).
func (s *svc) checkRestoredNesting(ctx context.Context, tx Tx, timelineID string) error {
	row, err := tx.Projects().Get(ctx, timelineID)
	if err != nil {
		return asError(err)
	}

	doc, err := decodeRow(&row)
	if err != nil {
		return err
	}

	if nerr := checkNesting(newProductionGraph(ctx, tx, timelineID), doc); nerr != nil {
		return nerr.withPath("item_id")
	}

	return nil
}

// storeSpawned writes the timelines a command made (Result.Spawned) in the transaction of the commit: the row of
// the document, the timeline item of the production and its place in the order.
func (s *svc) storeSpawned(ctx context.Context, tx ProjectTx, actor string, spawned []*timeline.Document) error {
	if len(spawned) == 0 {
		return nil
	}

	c := Caller{Actor: actor}

	owner, err := tx.Productions().ProjectOfTimeline(ctx, tx.Project().ID)
	if err != nil {
		return err
	}

	head, err := tx.Productions().Get(ctx, owner)
	if err != nil {
		return err
	}

	rows, err := tx.Productions().Items(ctx, owner)
	if err != nil {
		return err
	}

	now := s.nowMs()
	ids := idsOf(rows)

	for _, sp := range spawned {
		if v := project.CheckRoom(coreItems(rows), project.Timeline); v != nil {
			return violation(v)
		}

		it := s.newItem(owner, string(project.Timeline), sp.Name, len(rows), c.Actor, now)
		it.Ref = sp.ID

		if err := tx.Projects().Create(ctx, s.timelineRow(sp, c, head.OwnerUserID, now)); err != nil {
			return err
		}

		if err := tx.Productions().CreateItem(ctx, it); err != nil {
			return err
		}

		rows, ids = append(rows, it), append(ids, it.ID)
	}

	return tx.Productions().SetPositions(ctx, owner, ids)
}

// publishAfterCommit publishes, once the transaction has committed, the batch.committed event of ev and then
// nested.updated on the stream of every timeline that holds the timeline of ev, directly or through others
// (SDD-006 section 3.3.1). With no user it publishes the first only.
func (s *svc) publishAfterCommit(tx Tx, g *productionGraph, ev *Event) {
	users := g.usersDeep(ev.ProjectID)
	committed := *ev

	tx.AfterCommit(func() {
		crashpoint.Hit("commit.after_commit")
		s.emit(committed)

		for _, user := range users {
			s.emit(Event{
				Type: "nested.updated", ProjectID: user, TimelineID: committed.ProjectID,
				Revision: committed.Revision, At: committed.At,
			})
		}

		crashpoint.Hit("commit.after_publish")
	})
}

// compositionUsers lists the active timelines of the production that hold a composition of timelineID.
func (s *svc) compositionUsers(ctx context.Context, repos Repos, timelineID string) []map[string]any {
	g := newProductionGraph(ctx, repos, timelineID)

	var out []map[string]any

	for _, id := range g.Users(timelineID) {
		d, _ := g.Doc(id)
		out = append(out, map[string]any{"timeline_id": id, "title": d.Name})
	}

	return out
}

// nestedFor reads the timelines the compositions of doc use, for rendering: the documents by id and the
// revision to key the caches by. With no composition it reads nothing.
func (s *svc) nestedFor(
	ctx context.Context, repos Repos, doc *timeline.Document,
) (docs map[string]*timeline.Document, keyRev func(int64) int64) {
	if len(timeline.CompositionTimelines(doc)) == 0 {
		return nil, func(rev int64) int64 { return rev }
	}

	g := newProductionGraph(ctx, repos, doc.ID)
	docs, revs := g.Closure(doc)

	return docs, func(rev int64) int64 { return cacheRevision(rev, revs) }
}

// nestedSnapshot is the canonical form of the timelines a job keeps, so that an export renders what was asked
// for even when the timelines it uses are edited while the job waits.
func nestedSnapshot(docs map[string]*timeline.Document) map[string]json.RawMessage {
	if len(docs) == 0 {
		return nil
	}

	out := make(map[string]json.RawMessage, len(docs))
	for id, d := range docs {
		out[id] = timeline.Canonical(d)
	}

	return out
}

// decodeNested is nestedSnapshot back.
func decodeNested(in map[string]json.RawMessage) (map[string]*timeline.Document, error) {
	if len(in) == 0 {
		return nil, nil
	}

	out := make(map[string]*timeline.Document, len(in))

	for id, raw := range in {
		d, err := decodeRow(&ProjectRow{Document: raw})
		if err != nil {
			return nil, err
		}

		out[id] = d
	}

	return out, nil
}

// nestedLookup is lint.Env.Nested over the stored timelines.
func nestedLookup(docs map[string]*timeline.Document) func(string) (*timeline.Document, bool) {
	return func(id string) (*timeline.Document, bool) {
		d, ok := docs[id]

		return d, ok
	}
}

// applyChecked applies a batch with the timelines of the production in reach of the commands, and checks the
// nesting rules of the document it made (see checkNesting).
func (s *svc) applyChecked(
	g *productionGraph, doc *timeline.Document, ops []command.Operation, ids *idRecorder,
) (command.BatchResult, error) {
	out, err := s.reg.ApplyBatch(doc, ops, command.Env{NewIDs: ids.next, Timelines: g})
	if err != nil {
		return out, fromCommand(err)
	}

	if nerr := checkNesting(g, out.Doc); nerr != nil {
		return out, nerr
	}

	return out, nil
}
